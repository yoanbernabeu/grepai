package trace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGOBSymbolStore_ContentHashLifecycle(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	symbols := []Symbol{
		{
			Name:     "main",
			Kind:     KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}
	refs := []Reference{
		{
			SymbolName: "main",
			File:       "main.go",
			Line:       1,
			CallerName: "<top-level>",
		},
	}

	if err := store.SaveFileWithContentHash(ctx, "main.go", "hash-1", symbols, refs); err != nil {
		t.Fatalf("SaveFileWithContentHash failed: %v", err)
	}

	hash, ok := store.GetFileContentHash("main.go")
	if !ok || hash != "hash-1" {
		t.Fatalf("expected hash-1 in memory, got ok=%v hash=%q", ok, hash)
	}

	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hash, ok = reloaded.GetFileContentHash("main.go")
	if !ok || hash != "hash-1" {
		t.Fatalf("expected hash-1 after reload, got ok=%v hash=%q", ok, hash)
	}
	if !reloaded.IsFileIndexed("main.go") {
		t.Fatal("expected file to be marked indexed")
	}

	if err := reloaded.DeleteFile(ctx, "main.go"); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	if _, ok := reloaded.GetFileContentHash("main.go"); ok {
		t.Fatal("expected content hash to be removed on delete")
	}
	if reloaded.IsFileIndexed("main.go") {
		t.Fatal("expected file index marker to be removed on delete")
	}
}

func TestGOBSymbolStore_SaveFileClearsHashForBackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	if err := store.SaveFileWithContentHash(ctx, "main.go", "hash-1", nil, nil); err != nil {
		t.Fatalf("SaveFileWithContentHash failed: %v", err)
	}

	if err := store.SaveFile(ctx, "main.go", nil, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	if _, ok := store.GetFileContentHash("main.go"); ok {
		t.Fatal("expected SaveFile without hash to clear stored hash")
	}
}
