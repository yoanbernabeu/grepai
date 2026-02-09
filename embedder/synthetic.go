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
	defaultSyntheticEndpoint = "https://api.synthetic.new"
	defaultSyntheticPath     = "/openai/v1"
	defaultSyntheticModel    = "hf:nomic-ai/nomic-embed-text-v1.5"
	syntheticEmbedDimensions = 768
)

type SyntheticEmbedder struct {
	endpoint   string
	model      string
	apiKey     string
	dimensions int
	client     *http.Client
}

type syntheticEmbedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

type syntheticEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model,omitempty"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type syntheticErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type SyntheticOption func(*SyntheticEmbedder)

func WithSyntheticEndpoint(endpoint string) SyntheticOption {
	return func(e *SyntheticEmbedder) {
		e.endpoint = endpoint
	}
}

func WithSyntheticModel(model string) SyntheticOption {
	return func(e *SyntheticEmbedder) {
		e.model = model
	}
}

func WithSyntheticKey(key string) SyntheticOption {
	return func(e *SyntheticEmbedder) {
		e.apiKey = key
	}
}

func WithSyntheticDimensions(dimensions int) SyntheticOption {
	return func(e *SyntheticEmbedder) {
		e.dimensions = dimensions
	}
}

func NewSyntheticEmbedder(opts ...SyntheticOption) (*SyntheticEmbedder, error) {
	e := &SyntheticEmbedder{
		endpoint:   defaultSyntheticEndpoint + defaultSyntheticPath,
		model:      defaultSyntheticModel,
		dimensions: syntheticEmbedDimensions,
		client: &http.Client{
			Timeout: 90 * time.Second, // Longer timeout for synthetic API
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	// Try to get API key from environment if not set
	if e.apiKey == "" {
		e.apiKey = os.Getenv("SYNTHETIC_API_KEY")
	}

	if e.apiKey == "" {
		e.apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("synthetic API key not set (use SYNTHETIC_API_KEY or OPENAI_API_KEY environment variable)")
	}

	return e, nil
}

func (e *SyntheticEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *SyntheticEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := syntheticEmbedRequest{
		Model:      e.model,
		Input:      texts,
		Dimensions: &e.dimensions,
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

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Synthetic: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp syntheticErrorResponse
		msg := string(body)
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		return nil, fmt.Errorf("synthetic API error (status %d): %s", resp.StatusCode, msg)
	}

	var result syntheticEmbedResponse
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

func (e *SyntheticEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *SyntheticEmbedder) Close() error {
	return nil
}

// Ping checks if Synthetic API is reachable
func (e *SyntheticEmbedder) Ping(ctx context.Context) error {
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

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach Synthetic at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("synthetic returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
