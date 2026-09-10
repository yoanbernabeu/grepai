package trace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGOBSymbolStoreListIndexedFilesIncludesZeroSymbolEntries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(path)
	if err := store.SaveFileWithSignature(ctx, "z/empty.go", "hash", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(ctx, "a/symbol.go", []Symbol{{Name: "Symbol", File: "a/symbol.go"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatal(err)
	}
	store = NewGOBSymbolStore(path)
	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a/symbol.go", "z/empty.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

func TestGOBSymbolStoreListIndexedFilesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	files, err := store.ListIndexedFiles(ctx)
	if !errors.Is(err, context.Canceled) || files != nil {
		t.Fatalf("files=%v err=%v", files, err)
	}
}
