package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

// GenerateFromEnv generates a .grepai/config.yaml from GREPAI_* environment
// variables. It is idempotent: if the config file already exists, it does
// nothing. This replaces the shell-based docker-entrypoint.sh logic.
func GenerateFromEnv(projectRoot string) error {
	if Exists(projectRoot) {
		log.Printf("Config already exists at %s, skipping generation", GetConfigPath(projectRoot))
		return nil
	}

	provider := envOrDefault("GREPAI_PROVIDER", DefaultEmbedderProvider)

	// Start from provider defaults
	cfg := DefaultConfig()
	cfg.Embedder = DefaultEmbedderForProvider(provider)
	cfg.Store = DefaultStoreForBackend(envOrDefault("GREPAI_BACKEND", "gob"))

	// Override embedder fields from env vars
	if v := os.Getenv("GREPAI_MODEL"); v != "" {
		cfg.Embedder.Model = v
	}
	if v := os.Getenv("GREPAI_ENDPOINT"); v != "" {
		cfg.Embedder.Endpoint = v
	}
	if v := os.Getenv("GREPAI_API_KEY"); v != "" {
		cfg.Embedder.APIKey = v
	}
	if v := os.Getenv("GREPAI_DIMENSIONS"); v != "" {
		dim, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid GREPAI_DIMENSIONS %q: %w", v, err)
		}
		cfg.Embedder.Dimensions = &dim
	}
	if v := os.Getenv("GREPAI_PARALLELISM"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid GREPAI_PARALLELISM %q: %w", v, err)
		}
		cfg.Embedder.Parallelism = p
	}

	// Override store fields from env vars
	if v := os.Getenv("GREPAI_POSTGRES_DSN"); v != "" {
		cfg.Store.Postgres.DSN = v
	}
	if v := os.Getenv("GREPAI_QDRANT_ENDPOINT"); v != "" {
		cfg.Store.Qdrant.Endpoint = v
	}
	if v := os.Getenv("GREPAI_QDRANT_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid GREPAI_QDRANT_PORT %q: %w", v, err)
		}
		cfg.Store.Qdrant.Port = p
	}
	if v := os.Getenv("GREPAI_QDRANT_COLLECTION"); v != "" {
		cfg.Store.Qdrant.Collection = v
	}
	if v := os.Getenv("GREPAI_QDRANT_API_KEY"); v != "" {
		cfg.Store.Qdrant.APIKey = v
	}
	if v := os.Getenv("GREPAI_QDRANT_USE_TLS"); v != "" {
		cfg.Store.Qdrant.UseTLS = v == "true" || v == "1"
	}

	// Override chunking fields from env vars
	if v := os.Getenv("GREPAI_CHUNKING_SIZE"); v != "" {
		s, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid GREPAI_CHUNKING_SIZE %q: %w", v, err)
		}
		cfg.Chunking.Size = s
	}
	if v := os.Getenv("GREPAI_CHUNKING_OVERLAP"); v != "" {
		o, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid GREPAI_CHUNKING_OVERLAP %q: %w", v, err)
		}
		cfg.Chunking.Overlap = o
	}

	if err := cfg.Save(projectRoot); err != nil {
		return fmt.Errorf("failed to save generated config: %w", err)
	}

	log.Printf("Config generated at %s (provider: %s, backend: %s)",
		GetConfigPath(projectRoot), cfg.Embedder.Provider, cfg.Store.Backend)
	return nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
