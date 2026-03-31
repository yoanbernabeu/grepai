package config

import "testing"

func TestMultiModelDefaultsFalse(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Store.MultiModel {
		t.Errorf("expected MultiModel to default to false, got true")
	}
}

func TestEmbedModelTag_ReturnsProviderSlashModel(t *testing.T) {
	cfg := &Config{
		Embedder: EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
		},
	}

	tag := cfg.EmbedModelTag()
	expected := "ollama/nomic-embed-text"
	if tag != expected {
		t.Errorf("expected %q, got %q", expected, tag)
	}
}

func TestEmbedModelTag_EmptyProviderReturnsEmpty(t *testing.T) {
	cfg := &Config{
		Embedder: EmbedderConfig{
			Provider: "",
			Model:    "nomic-embed-text",
		},
	}

	tag := cfg.EmbedModelTag()
	if tag != "" {
		t.Errorf("expected empty tag when provider is empty, got %q", tag)
	}
}

func TestEmbedModelTag_EmptyModelReturnsEmpty(t *testing.T) {
	cfg := &Config{
		Embedder: EmbedderConfig{
			Provider: "ollama",
			Model:    "",
		},
	}

	tag := cfg.EmbedModelTag()
	if tag != "" {
		t.Errorf("expected empty tag when model is empty, got %q", tag)
	}
}
