package cli

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/watcher"
)

// A model change empties the project's index so every file is re-embedded;
// an index without metadata is adopted without a rebuild (#139).
func TestResetIndexIfEmbedderChanged(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t) // main.go indexed
	runtime := &projectIndexRuntime{
		projectRoot:   h.projectRoot,
		vectorStore:   h.vecStore,
		indexMetaPath: config.GetIndexMetaPath(h.projectRoot),
		embedderMeta:  config.IndexMeta{Provider: "ollama", Model: "nomic-embed-text", Dimensions: 768},
	}

	if err := runtime.resetIndexIfEmbedderChanged(ctx); err != nil {
		t.Fatal(err)
	}
	if doc, _ := h.vecStore.GetDocument(ctx, "main.go"); doc == nil {
		t.Fatal("an index without metadata must be kept")
	}
	if !runtime.saveIndexMeta {
		t.Fatal("metadata must be written for an adopted index")
	}
	if err := config.SaveIndexMeta(runtime.indexMetaPath, runtime.embedderMeta); err != nil {
		t.Fatal(err)
	}

	runtime.saveIndexMeta = false
	runtime.embedderMeta.Model = "mxbai-embed-large"
	if err := runtime.resetIndexIfEmbedderChanged(ctx); err != nil {
		t.Fatal(err)
	}
	if doc, _ := h.vecStore.GetDocument(ctx, "main.go"); doc != nil {
		t.Fatal("documents must be removed after a model change")
	}
	if !runtime.saveIndexMeta {
		t.Fatal("new metadata must be written after the rebuild")
	}

	// The next event re-indexes the file normally.
	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventModify, Path: "main.go"})
	if doc, _ := h.vecStore.GetDocument(ctx, "main.go"); doc == nil {
		t.Fatal("file not re-indexed after the rebuild")
	}
}
