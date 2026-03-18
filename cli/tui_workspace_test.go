package cli

import "testing"

func TestBuildWorkspaceFromSelectionMatchesFlagsBuilder(t *testing.T) {
	ws := buildWorkspaceFromSelection("demo", 1, 0) // qdrant + ollama

	ref, err := buildWorkspaceFromFlags("demo", "qdrant", "ollama", "", "", "", "", 0, "", true)
	if err != nil {
		t.Fatalf("buildWorkspaceFromFlags failed: %v", err)
	}

	if ws.Store.Backend != ref.Store.Backend {
		t.Fatalf("backend = %s, want %s", ws.Store.Backend, ref.Store.Backend)
	}
	if ws.Embedder.Provider != ref.Embedder.Provider {
		t.Fatalf("provider = %s, want %s", ws.Embedder.Provider, ref.Embedder.Provider)
	}
	if ws.Store.Qdrant.Port != ref.Store.Qdrant.Port {
		t.Fatalf("qdrant port = %d, want %d", ws.Store.Qdrant.Port, ref.Store.Qdrant.Port)
	}
}

func TestBuildWorkspaceFromSelection_LlamaCPP(t *testing.T) {
	ws := buildWorkspaceFromSelection("demo", 1, 1)

	if ws.Embedder.Provider != "llamacpp" {
		t.Fatalf("provider = %s, want llamacpp", ws.Embedder.Provider)
	}
	if ws.Embedder.Model != "bge-small-en-v1.5-q8_0" {
		t.Fatalf("model = %s, want bge-small-en-v1.5-q8_0", ws.Embedder.Model)
	}
}
