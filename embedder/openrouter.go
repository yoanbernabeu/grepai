package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultOpenRouterEndpoint = "https://openrouter.ai/api/v1"
	defaultOpenRouterModel    = "openai/text-embedding-3-small"
	openRouterDimensions      = 1536
)

// OpenRouterEmbedder implements the Embedder interface for the OpenRouter API.
// Note: Parallelism was intentionally removed to keep the implementation simple.
// OpenRouter processes batches efficiently as-is. If parallel processing is needed
// in the future, consider implementing the BatchEmbedder interface similar to
// OpenAIEmbedder with AdaptiveRateLimiter and worker pools.
type OpenRouterEmbedder struct {
	endpoint   string
	model      string
	apiKey     string
	dimensions *int
	client     *http.Client
}

type openRouterEmbedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

type openRouterEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type openRouterErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type OpenRouterOption func(*OpenRouterEmbedder)

func WithOpenRouterEndpoint(endpoint string) OpenRouterOption {
	return func(e *OpenRouterEmbedder) {
		e.endpoint = endpoint
	}
}

func WithOpenRouterModel(model string) OpenRouterOption {
	return func(e *OpenRouterEmbedder) {
		e.model = model
	}
}

func WithOpenRouterKey(key string) OpenRouterOption {
	return func(e *OpenRouterEmbedder) {
		e.apiKey = key
	}
}

func WithOpenRouterDimensions(dimensions int) OpenRouterOption {
	return func(e *OpenRouterEmbedder) {
		e.dimensions = &dimensions
	}
}

func NewOpenRouterEmbedder(opts ...OpenRouterOption) (*OpenRouterEmbedder, error) {
	e := &OpenRouterEmbedder{
		endpoint:   defaultOpenRouterEndpoint,
		model:      defaultOpenRouterModel,
		dimensions: nil, // nil = let the model use its native dimensions
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	// Try to get API key from environment if not set
	if e.apiKey == "" {
		e.apiKey = os.Getenv("OPENROUTER_API_KEY")
	}

	if e.apiKey == "" {
		e.apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("openrouter API key not set (use OPENROUTER_API_KEY or OPENAI_API_KEY environment variable)")
	}

	return e, nil
}

func (e *OpenRouterEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *OpenRouterEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openRouterEmbedRequest{
		Model:      e.model,
		Input:      texts,
		Dimensions: e.dimensions,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))
	req.Header.Set("HTTP-Referer", "grepai")
	req.Header.Set("X-Title", "grepai")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openRouterErrorResponse
		msg := string(body)
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		return nil, fmt.Errorf("openrouter API error (status %d): %s", resp.StatusCode, msg)
	}

	var result openRouterEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	// Sort by index to maintain order
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

func (e *OpenRouterEmbedder) Dimensions() int {
	if e.dimensions == nil {
		return openRouterDimensions
	}
	return *e.dimensions
}

func (e *OpenRouterEmbedder) Close() error {
	return nil
}

// Ping checks if OpenRouter API is reachable
func (e *OpenRouterEmbedder) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	pingReq := map[string]string{
		"model": e.model,
		"input": "test",
	}
	jsonData, err := json.Marshal(pingReq)
	if err != nil {
		return fmt.Errorf("failed to marshal ping request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))
	req.Header.Set("HTTP-Referer", "grepai")
	req.Header.Set("X-Title", "grepai")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach OpenRouter at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openrouter returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
