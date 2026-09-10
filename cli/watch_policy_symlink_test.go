package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/watcher"
)

func TestHandleFileEventPolicyDiscoverySkipsFileSymlinkOutsideProject(t *testing.T) {
	skipIfWindows(t)
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	ignorePath := filepath.Join(h.projectRoot, ".grepaiignore")
	if err := os.WriteFile(ignorePath, []byte("hidden/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(h.projectRoot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.scanner = indexer.NewScanner(h.projectRoot, ignore)
	hidden := filepath.Join(h.projectRoot, "hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	regularPath := filepath.Join("hidden", "new.go")
	if err := os.WriteFile(filepath.Join(h.projectRoot, regularPath), []byte("package hidden\nfunc SafeNew() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	targetPath := filepath.Join(outDir, "secret.go")
	targetContent := []byte("package outside\nfunc SecretOutside() {}\n")
	if err := os.WriteFile(targetPath, targetContent, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join("hidden", "link.go")
	if err := os.Symlink(targetPath, filepath.Join(h.projectRoot, linkPath)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	if err := ignore.Refresh(); err != nil {
		t.Fatal(err)
	}
	embedCalls, embedBatchCalls := h.emb.embedCalls, h.emb.embedBatchCalls

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventReconcile, Path: "hidden", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, regularPath); err != nil || doc == nil {
		t.Fatalf("regular discovered document = %#v, err=%v", doc, err)
	}
	if doc, err := h.vecStore.GetDocument(ctx, linkPath); err != nil || doc != nil {
		t.Fatalf("file symlink document = %#v, err=%v", doc, err)
	}
	if symbols, err := h.symbolStore.LookupSymbol(ctx, "SecretOutside"); err != nil || len(symbols) != 0 {
		t.Fatalf("outside target symbols = %#v, err=%v", symbols, err)
	}
	if h.emb.embedCalls != embedCalls || h.emb.embedBatchCalls != embedBatchCalls+1 {
		t.Fatalf("embedding calls %d/%d -> %d/%d, want only regular file embedded", embedCalls, embedBatchCalls, h.emb.embedCalls, h.emb.embedBatchCalls)
	}
	after, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil || string(got) != string(targetContent) || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("outside target changed: content=%q before=%v after=%v err=%v", got, before, after, err)
	}
}
