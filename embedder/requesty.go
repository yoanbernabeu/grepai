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
	defaultRequestyEndpoint = "https://router.requesty.ai/v1"
	defaultRequestyModel    = "openai/text-embedding-3-small"
	requestyDimensions      = 1536
)

// RequestyEmbedder implements the Embedder interface for the Requesty API.
// Note: Parallelism was intentionally removed to keep the implementation simple.
// Requesty processes batches efficiently as-is. If parallel processing is needed
// in the future, consider implementing the BatchEmbedder interface similar to
// OpenAIEmbedder with AdaptiveRateLimiter and worker pools.
type RequestyEmbedder struct {
	endpoint   string
	model      string
	apiKey     string
	dimensions *int
	client     *http.Client
}

type requestyEmbedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

type requestyEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type requestyErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type RequestyOption func(*RequestyEmbedder)

func WithRequestyEndpoint(endpoint string) RequestyOption {
	return func(e *RequestyEmbedder) {
		e.endpoint = endpoint
	}
}

func WithRequestyModel(model string) RequestyOption {
	return func(e *RequestyEmbedder) {
		e.model = model
	}
}

func WithRequestyKey(key string) RequestyOption {
	return func(e *RequestyEmbedder) {
		e.apiKey = key
	}
}

func WithRequestyDimensions(dimensions int) RequestyOption {
	return func(e *RequestyEmbedder) {
		e.dimensions = &dimensions
	}
}

func NewRequestyEmbedder(opts ...RequestyOption) (*RequestyEmbedder, error) {
	e := &RequestyEmbedder{
		endpoint:   defaultRequestyEndpoint,
		model:      defaultRequestyModel,
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
		e.apiKey = os.Getenv("REQUESTY_API_KEY")
	}

	if e.apiKey == "" {
		e.apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("requesty API key not set (use REQUESTY_API_KEY or OPENAI_API_KEY environment variable)")
	}

	return e, nil
}

func (e *RequestyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *RequestyEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := requestyEmbedRequest{
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
		return nil, fmt.Errorf("failed to send request to Requesty: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp requestyErrorResponse
		msg := string(body)
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		return nil, fmt.Errorf("requesty API error (status %d): %s", resp.StatusCode, msg)
	}

	var result requestyEmbedResponse
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

func (e *RequestyEmbedder) Dimensions() int {
	if e.dimensions == nil {
		return requestyDimensions
	}
	return *e.dimensions
}

func (e *RequestyEmbedder) Close() error {
	return nil
}

// Ping checks if Requesty API is reachable
func (e *RequestyEmbedder) Ping(ctx context.Context) error {
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
		return fmt.Errorf("failed to reach Requesty at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("requesty returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
