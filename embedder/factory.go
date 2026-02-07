package embedder

import (
	"fmt"

	"github.com/yoanbernabeu/grepai/config"
)

// NewFromConfig creates an Embedder based on the provided configuration.
// This factory function centralizes provider initialization and eliminates
// code duplication across CLI commands and MCP server.
func NewFromConfig(cfg *config.Config) (Embedder, error) {
	switch cfg.Embedder.Provider {
	case "ollama":
		opts := []OllamaOption{
			WithOllamaEndpoint(cfg.Embedder.Endpoint),
			WithOllamaModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, WithOllamaDimensions(*cfg.Embedder.Dimensions))
		}
		return NewOllamaEmbedder(opts...), nil

	case "openai":
		opts := []OpenAIOption{
			WithOpenAIModel(cfg.Embedder.Model),
			WithOpenAIKey(cfg.Embedder.APIKey),
			WithOpenAIEndpoint(cfg.Embedder.Endpoint),
			WithOpenAIParallelism(cfg.Embedder.Parallelism),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, WithOpenAIDimensions(*cfg.Embedder.Dimensions))
		}
		return NewOpenAIEmbedder(opts...)

	case "lmstudio":
		opts := []LMStudioOption{
			WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			WithLMStudioModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, WithLMStudioDimensions(*cfg.Embedder.Dimensions))
		}
		return NewLMStudioEmbedder(opts...), nil

	case "synthetic":
		opts := []SyntheticOption{
			WithSyntheticModel(cfg.Embedder.Model),
			WithSyntheticKey(cfg.Embedder.APIKey),
			WithSyntheticEndpoint(cfg.Embedder.Endpoint),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, WithSyntheticDimensions(*cfg.Embedder.Dimensions))
		}
		return NewSyntheticEmbedder(opts...)

	case "openrouter":
		opts := []OpenRouterOption{
			WithOpenRouterModel(cfg.Embedder.Model),
			WithOpenRouterKey(cfg.Embedder.APIKey),
			WithOpenRouterEndpoint(cfg.Embedder.Endpoint),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, WithOpenRouterDimensions(*cfg.Embedder.Dimensions))
		}
		return NewOpenRouterEmbedder(opts...)

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Embedder.Provider)
	}
}

// NewFromWorkspaceConfig creates an Embedder from workspace configuration.
// This is a convenience wrapper for workspace-specific embedder creation.
func NewFromWorkspaceConfig(ws *config.Workspace) (Embedder, error) {
	// Convert workspace embedder config to regular config
	cfg := &config.Config{
		Embedder: ws.Embedder,
	}
	return NewFromConfig(cfg)
}
