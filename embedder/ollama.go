package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultOllamaEndpoint = "http://localhost:11434"
	defaultOllamaModel    = "nomic-embed-text"
	nomicEmbedDimensions  = 768
)

type OllamaEmbedder struct {
	endpoint   string
	model      string
	dimensions int
	client     *http.Client
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

type OllamaOption func(*OllamaEmbedder)

func WithOllamaEndpoint(endpoint string) OllamaOption {
	return func(e *OllamaEmbedder) {
		e.endpoint = endpoint
	}
}

func WithOllamaModel(model string) OllamaOption {
	return func(e *OllamaEmbedder) {
		e.model = model
	}
}
func WithOllamaDimensions(dimensions int) OllamaOption {
	return func(e *OllamaEmbedder) {
		e.dimensions = dimensions
	}
}

func NewOllamaEmbedder(opts ...OllamaOption) *OllamaEmbedder {
	e := &OllamaEmbedder{
		endpoint:   defaultOllamaEndpoint,
		model:      defaultOllamaModel,
		dimensions: nomicEmbedDimensions,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model:  e.model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("Ollama returned empty embedding")
	}

	return result.Embedding, nil
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))

	for i, text := range texts {
		embedding, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}

	return results, nil
}

func (e *OllamaEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *OllamaEmbedder) Close() error {
	return nil
}

// Ping checks if Ollama is reachable
func (e *OllamaEmbedder) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/tags", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach Ollama at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	return nil
}
