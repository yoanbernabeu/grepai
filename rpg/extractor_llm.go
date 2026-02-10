package rpg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMExtractorConfig configures the LLM feature extractor.
type LLMExtractorConfig struct {
	Provider string // "openai" compatible
	Model    string
	Endpoint string
	APIKey   string
	Timeout  time.Duration
}

// LLMExtractor generates feature labels using an LLM API.
// Falls back to LocalExtractor on error.
type LLMExtractor struct {
	cfg      LLMExtractorConfig
	client   *http.Client
	fallback *LocalExtractor
	ctx      context.Context // parent context for cancellation propagation
}

// NewLLMExtractor creates an LLM-based feature extractor with local fallback.
func NewLLMExtractor(cfg LLMExtractorConfig) *LLMExtractor {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	return &LLMExtractor{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.Timeout},
		fallback: NewLocalExtractor(),
	}
}

func (e *LLMExtractor) Mode() string { return "llm" }

// WithContext returns a copy of the extractor that uses the given context for LLM calls.
func (e *LLMExtractor) WithContext(ctx context.Context) *LLMExtractor {
	cp := *e
	cp.ctx = ctx
	return &cp
}

// ExtractFeature calls the LLM to generate a semantic feature label.
// Falls back to local extraction on any error.
func (e *LLMExtractor) ExtractFeature(symbolName, signature, receiver, comment string) string {
	parent := e.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, e.cfg.Timeout)
	defer cancel()

	label, err := e.callLLM(ctx, symbolName, signature, receiver, comment)
	if err != nil {
		// Fallback to local extractor
		return e.fallback.ExtractFeature(symbolName, signature, receiver, comment)
	}
	return label
}

// callLLM makes an OpenAI-compatible chat completion API call.
func (e *LLMExtractor) callLLM(ctx context.Context, symbolName, signature, receiver, comment string) (string, error) {
	// Build prompt
	prompt := buildFeaturePrompt(symbolName, signature, receiver, comment)

	// Build request body (OpenAI chat completion format)
	reqBody := map[string]any{
		"model": e.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a code analysis assistant. Given a function/method signature, output a concise verb-object feature label in kebab-case (e.g., 'handle-request', 'validate-token', 'parse-config'). Output ONLY the label, nothing else."},
			{"role": "user", "content": prompt},
		},
		"max_tokens":  30,
		"temperature": 0,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimRight(e.cfg.Endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Parse OpenAI response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	label := strings.TrimSpace(result.Choices[0].Message.Content)
	label = sanitizeLabel(label)
	if label == "" {
		return "", fmt.Errorf("empty label from LLM")
	}

	return label, nil
}

// buildFeaturePrompt constructs the prompt for the LLM based on symbol metadata.
func buildFeaturePrompt(symbolName, signature, receiver, comment string) string {
	var sb strings.Builder
	sb.WriteString("Function: " + symbolName + "\n")
	if signature != "" {
		sb.WriteString("Signature: " + signature + "\n")
	}
	if receiver != "" {
		sb.WriteString("Receiver: " + receiver + "\n")
	}
	if comment != "" {
		sb.WriteString("Comment: " + comment + "\n")
	}
	sb.WriteString("\nOutput a kebab-case verb-object label:")
	return sb.String()
}

// sanitizeLabel cleans up the LLM output to ensure it's a valid feature label.
func sanitizeLabel(label string) string {
	// Remove quotes, backticks, trailing punctuation
	label = strings.Trim(label, "\"'`\n\r\t .")
	// Take only the first line
	if idx := strings.IndexAny(label, "\n\r"); idx >= 0 {
		label = label[:idx]
	}
	// Lowercase
	label = strings.ToLower(label)
	// Replace spaces with hyphens
	label = strings.ReplaceAll(label, " ", "-")
	// Cap length
	if len(label) > 50 {
		label = label[:50]
	}
	return label
}
