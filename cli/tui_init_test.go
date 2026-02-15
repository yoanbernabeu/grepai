package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestInitWizardNumberInput(t *testing.T) {
	m := newInitUIModel("/tmp", nil, nil, nil, false)
	m.step = initStepProvider
	// Default index 0 (ollama)
	if m.providerIdx != 0 {
		t.Fatalf("initial providerIdx = %d, want 0", m.providerIdx)
	}

	// Press '2' (index 1) -> should select lmstudio?
	// Currently implementation doesn't support it, so this test documents the "bug" (missing feature).
	// usage: 1-based index from user perspective?
	// options: ollama, lmstudio, openai
	// 1 -> ollama (idx 0)
	// 2 -> lmstudio (idx 1)
	// 3 -> openai (idx 2)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = next.(initUIModel)

	// If the feature is missing, this will fail if we assert it works.
	// Validating that it currently fails (or implementing the check).
	if m.providerIdx != 1 {
		t.Fatalf("providerIdx after pressing '2' = %d, want 1", m.providerIdx)
	}
}
