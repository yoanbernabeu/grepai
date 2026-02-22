package rpg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

// ExtractFeature calls the LLM to generate a semantic feature label.
// Falls back to local extraction on any error.
func (e *LLMExtractor) ExtractFeature(ctx context.Context, symbolName, signature, receiver, comment string) string {
	features := e.ExtractAtomicFeatures(ctx, symbolName, signature, receiver, comment)
	if len(features) == 0 {
		return e.fallback.ExtractFeature(ctx, symbolName, signature, receiver, comment)
	}
	return primaryFromAtomicFeature(features[0])
}

// ExtractAtomicFeatures calls the LLM to generate atomic semantic features.
// Falls back to local extraction on any error.
func (e *LLMExtractor) ExtractAtomicFeatures(ctx context.Context, symbolName, signature, receiver, comment string) []string {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	prompt := buildAtomicFeaturePrompt(symbolName, signature, receiver, comment)
	systemPrompt := "You are a senior software analyst. Return 1 to 5 atomic semantic features as lowercase verb-object phrases. Output ONLY a JSON array of strings."

	response, err := e.callCompletion(ctx, systemPrompt, prompt)
	if err != nil {
		return e.fallback.ExtractAtomicFeatures(ctx, symbolName, signature, receiver, comment)
	}

	features := parseAtomicFeatureResponse(response)
	if len(features) == 0 {
		return e.fallback.ExtractAtomicFeatures(ctx, symbolName, signature, receiver, comment)
	}
	return features
}

// GenerateSummary calls the LLM to generate a high-level summary.
func (e *LLMExtractor) GenerateSummary(ctx context.Context, name, contextStr string) (string, error) {
	systemPrompt := "You are a code analysis assistant. Summarize the provided code context (Area/Category/Subcategory) into a concise, high-level description of its responsibility. Output ONLY the summary."
	userPrompt := fmt.Sprintf("Name: %s\nContext: %s\n\nGenerate a summary:", name, contextStr)
	return e.callCompletion(ctx, systemPrompt, userPrompt)
}

// callCompletion makes an OpenAI-compatible chat completion API call.
func (e *LLMExtractor) callCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Build request body (OpenAI chat completion format)
	reqBody := map[string]any{
		"model": e.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"max_tokens":  100, // Increased for summaries
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
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
	if label == "" {
		return "", fmt.Errorf("empty label from LLM")
	}

	return label, nil
}

// buildAtomicFeaturePrompt constructs the prompt for LLM feature extraction.
func buildAtomicFeaturePrompt(symbolName, signature, receiver, comment string) string {
	var sb strings.Builder
	sb.WriteString("Function: " + symbolName + "\n")
	if signature != "" {
		sb.WriteString("Signature: " + signature + "\n")
	}
	if receiver != "" {
		sb.WriteString("Receiver: " + receiver + "\n")
	}
	if comment != "" {
		sb.WriteString("Docstring: " + comment + "\n")
	}
	sb.WriteString("\nRules:\n")
	sb.WriteString("- Use lowercase english.\n")
	sb.WriteString("- Each item is one atomic verb-object phrase.\n")
	sb.WriteString("- Avoid implementation details, frameworks, and control flow language.\n")
	sb.WriteString("- Do not include receiver/type names unless semantically required.\n")
	sb.WriteString("\nReturn JSON array only, for example: [\"validate token\", \"load config\"]")
	return sb.String()
}

func parseAtomicFeatureResponse(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Strip optional markdown fences.
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			lines = lines[1 : len(lines)-1]
			raw = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return dedupeAtomicFeatures(arr, 5)
	}

	// Fallback: split by common separators and bullet prefixes.
	re := regexp.MustCompile(`[\n,;]+`)
	parts := re.Split(raw, -1)
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "-"))
		if part == "" {
			continue
		}
		features = append(features, part)
	}
	return dedupeAtomicFeatures(features, 5)
}
