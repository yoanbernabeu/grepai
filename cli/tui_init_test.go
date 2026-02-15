package cli

import (
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestInitWizardBuildConfig(t *testing.T) {
	m := newInitUIModel("/tmp/project", config.DefaultConfig(), nil, nil, false)
	m.providerIdx = optionIndex(initProviderOptions, "openai")
	m.backendIdx = optionIndex(initBackendOptions, "qdrant")

	cfg, err := m.buildConfig()
	if err != nil {
		t.Fatalf("buildConfig failed: %v", err)
	}

	if cfg.Embedder.Provider != "openai" {
		t.Fatalf("provider = %s, want openai", cfg.Embedder.Provider)
	}
	if cfg.Store.Backend != "qdrant" {
		t.Fatalf("backend = %s, want qdrant", cfg.Store.Backend)
	}
	if cfg.Store.Qdrant.Port != 6334 {
		t.Fatalf("qdrant port = %d, want 6334", cfg.Store.Qdrant.Port)
	}
}
