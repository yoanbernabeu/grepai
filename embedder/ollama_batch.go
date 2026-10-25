package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ollamaBatchSize bounds request latency while still amortizing per-request overhead.
const (
	ollamaBatchSize        = 64
	maxOllamaErrorBodySize = 64 << 10
)

type ollamaBatchEmbedRequest struct {
	Model    string   `json:"model"`
	Input    []string `json:"input"`
	Truncate bool     `json:"truncate"`
}

type ollamaBatchEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type ollamaHTTPError struct {
	statusCode int
	body       string
}

func (e *ollamaHTTPError) Error() string {
	return fmt.Sprintf("Ollama returned status %d: %s", e.statusCode, e.body)
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return make([][]float32, 0), nil
	}
	if e.legacyBatchAPI.Load() {
		return e.embedLegacyBatch(ctx, texts)
	}
	results := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += ollamaBatchSize {
		end := min(start+ollamaBatchSize, len(texts))
		embeddings, err := e.embedModernBatch(ctx, texts[start:end])
		if err != nil {
			var httpError *ollamaHTTPError
			if errors.As(err, &httpError) && shouldUseLegacyBatchAPI(httpError) {
				e.legacyBatchAPI.Store(true)
				return e.embedLegacyBatch(ctx, texts)
			}
			if errors.As(err, &httpError) && isOllamaContextLengthError(httpError.body) {
				probeError := e.probeContextLengthError(ctx, texts[start:end])
				if contextError := AsContextLengthError(probeError); contextError != nil {
					contextError.ChunkIndex += start
					return nil, contextError
				}
				if probeError != nil {
					return nil, probeError
				}
			}
			return nil, err
		}
		results = append(results, embeddings...)
	}
	return results, nil
}

func (e *OllamaEmbedder) embedModernBatch(ctx context.Context, texts []string) ([][]float32, error) {
	requestBody := ollamaBatchEmbedRequest{
		Model:    e.model,
		Input:    texts,
		Truncate: false,
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embed", e.endpoint)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := e.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaErrorBodySize))
		if err != nil {
			return nil, fmt.Errorf("failed to read Ollama error response: %w", err)
		}
		return nil, &ollamaHTTPError{statusCode: response.StatusCode, body: string(body)}
	}

	var result ollamaBatchEmbedResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}
	for i, embedding := range result.Embeddings {
		if len(embedding) == 0 {
			return nil, fmt.Errorf("Ollama returned empty embedding for text %d", i)
		}
	}

	return result.Embeddings, nil
}

func (e *OllamaEmbedder) embedLegacyBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := e.Embed(ctx, text)
		if err != nil {
			if contextError := AsContextLengthError(err); contextError != nil {
				contextError.ChunkIndex = i
				return nil, contextError
			}
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}
	return results, nil
}

func (e *OllamaEmbedder) probeContextLengthError(ctx context.Context, texts []string) error {
	for i, text := range texts {
		_, err := e.embedModernBatch(ctx, []string{text})
		if err == nil {
			continue
		}
		var httpError *ollamaHTTPError
		if errors.As(err, &httpError) && isOllamaContextLengthError(httpError.body) {
			return NewContextLengthError(i, len(text)/4, 0, httpError.body)
		}
		return fmt.Errorf("failed to probe text %d after context length error: %w", i, err)
	}
	return nil
}

func isOllamaContextLengthError(body string) bool {
	message := strings.ToLower(body)
	return strings.Contains(message, "context length") ||
		strings.Contains(message, "context window") ||
		strings.Contains(message, "too many tokens") ||
		strings.Contains(message, "prompt is too long")
}

func shouldUseLegacyBatchAPI(httpError *ollamaHTTPError) bool {
	if httpError.statusCode == http.StatusMethodNotAllowed {
		return true
	}
	if httpError.statusCode != http.StatusNotFound {
		return false
	}
	var errorResponse struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(httpError.body), &errorResponse) == nil {
		switch strings.ToLower(strings.TrimSpace(errorResponse.Error)) {
		case "", "not found", "404 page not found", "route not found", "endpoint not found":
			return true
		default:
			return false
		}
	}
	return true
}
