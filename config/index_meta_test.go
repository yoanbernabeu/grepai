package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCheckIndexMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), IndexMetaFileName)
	cfg := EmbedderConfig{Provider: "ollama", Model: "nomic-embed-text"}

	// Indexes built before the metadata file existed are accepted.
	if err := CheckIndexMeta(path, cfg); err != nil {
		t.Fatalf("missing metadata must not fail: %v", err)
	}

	if err := SaveIndexMeta(path, EmbedderIndexMeta(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := CheckIndexMeta(path, cfg); err != nil {
		t.Fatalf("same embedder must pass: %v", err)
	}

	// The endpoint is not part of the identity.
	moved := cfg
	moved.Endpoint = "http://gpu-box:11434"
	if err := CheckIndexMeta(path, moved); err != nil {
		t.Fatalf("an endpoint change must not invalidate the index: %v", err)
	}

	other := cfg
	other.Model = "mxbai-embed-large"
	var changed *ErrEmbedderChanged
	if err := CheckIndexMeta(path, other); !errors.As(err, &changed) {
		t.Fatalf("a model change must be reported, got %v", err)
	}
}
