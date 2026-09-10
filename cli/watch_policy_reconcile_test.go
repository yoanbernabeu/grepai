package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

func TestHandleFileEventPolicyReconcileForgetsNewlyIgnoredExistingFile(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	path := filepath.Join("src", "indexed.go")
	content := "package src\nfunc PolicyIndexed() { PolicyTarget() }\n"
	indexDirectoryHarnessFile(t, h, path, content)
	ignore, err := indexer.NewIgnoreMatcher(h.projectRoot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.scanner = indexer.NewScanner(h.projectRoot, ignore)
	if err := os.WriteFile(filepath.Join(h.projectRoot, ".grepaiignore"), []byte("src/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ignore.Refresh(); err != nil {
		t.Fatal(err)
	}

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventReconcile, Path: ".", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, path); err != nil || doc != nil {
		t.Fatalf("newly ignored document = %#v, err=%v", doc, err)
	}
	if symbols, err := h.symbolStore.LookupSymbol(ctx, "PolicyIndexed"); err != nil || len(symbols) != 0 {
		t.Fatalf("newly ignored symbols = %#v, err=%v", symbols, err)
	}
	if refs, err := h.symbolStore.LookupCallers(ctx, "PolicyTarget"); err != nil || len(refs) != 0 {
		t.Fatalf("newly ignored references = %#v, err=%v", refs, err)
	}
	if got, err := os.ReadFile(filepath.Join(h.projectRoot, path)); err != nil || string(got) != content {
		t.Fatalf("physical file changed: content=%q err=%v", got, err)
	}
	if doc, err := h.vecStore.GetDocument(ctx, "main.go"); err != nil || doc == nil {
		t.Fatalf("unaffected root document = %#v, err=%v", doc, err)
	}
}

func TestHandleFileEventPhysicalRootRemovalEventDoesNotPurgeIndex(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventRename, Path: ".", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, "main.go"); err != nil || doc == nil {
		t.Fatalf("physical root event purged document: %#v, err=%v", doc, err)
	}
	if symbols, err := h.symbolStore.LookupSymbol(ctx, "original"); err != nil || len(symbols) == 0 {
		t.Fatalf("physical root event purged symbols: %#v, err=%v", symbols, err)
	}
}

func TestHandleFileEventPolicyReconcileHonorsSubtreeScope(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	scopedPath := filepath.Join("src", "ignored.go")
	outsidePath := filepath.Join("other", "kept.go")
	indexDirectoryHarnessFile(t, h, scopedPath, "package src\nfunc ScopedPolicySymbol() {}\n")
	indexDirectoryHarnessFile(t, h, outsidePath, "package other\nfunc OutsidePolicySymbol() {}\n")
	ignore, err := indexer.NewIgnoreMatcher(h.projectRoot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.scanner = indexer.NewScanner(h.projectRoot, ignore)
	if err := os.WriteFile(filepath.Join(h.projectRoot, "src", ".gitignore"), []byte("ignored.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ignore.RefreshSubtree("src"); err != nil {
		t.Fatal(err)
	}

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventReconcile, Path: "src", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, scopedPath); err != nil || doc != nil {
		t.Fatalf("scoped document = %#v, err=%v", doc, err)
	}
	if doc, err := h.vecStore.GetDocument(ctx, outsidePath); err != nil || doc == nil {
		t.Fatalf("outside document = %#v, err=%v", doc, err)
	}
	if symbols, err := h.symbolStore.LookupSymbol(ctx, "OutsidePolicySymbol"); err != nil || len(symbols) == 0 {
		t.Fatalf("outside symbols = %#v, err=%v", symbols, err)
	}
}

func TestPolicyReconcilePreservesIndexWhenRootUnavailable(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	movedRoot := h.projectRoot + "-moved"
	if err := os.Rename(h.projectRoot, movedRoot); err != nil {
		t.Fatal(err)
	}

	dispatched := false
	err := reconcilePolicyDirectory(ctx, h.projectRoot, ".", h.scanner, h.vecStore, h.symbolStore, func(directoryAction) {
		dispatched = true
	})
	if err == nil || dispatched {
		t.Fatalf("err=%v dispatched=%v, want root error without actions", err, dispatched)
	}
	if doc, getErr := h.vecStore.GetDocument(ctx, "main.go"); getErr != nil || doc == nil {
		t.Fatalf("document not preserved: %#v, err=%v", doc, getErr)
	}
	if symbols, lookupErr := h.symbolStore.LookupSymbol(ctx, "original"); lookupErr != nil || len(symbols) == 0 {
		t.Fatalf("symbols not preserved: %#v, err=%v", symbols, lookupErr)
	}
}

func TestHandleFileEventPolicyRootDoesNotReportPhantomRemoval(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	var deltas []watchStatsDelta

	handleFileEvent(
		ctx, h.idx, h.scanner, trace.NewRegexExtractor(), h.symbolStore,
		nil, h.vecStore, []string{".go"}, h.projectRoot, h.cfg,
		&h.lastWrite, nil, watcher.FileEvent{Type: watcher.EventReconcile, Path: ".", IsDir: true}, nil,
		func(_ string, delta watchStatsDelta) { deltas = append(deltas, delta) },
	)

	for _, delta := range deltas {
		if delta.FilesRemoved != 0 {
			t.Fatalf("root policy reconciliation reported phantom removal stats: %#v", deltas)
		}
	}
}

func TestHandleFileEventPolicyRootDiscoversNewlyUnignoredFile(t *testing.T) {
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
	path := filepath.Join("hidden", "new.go")
	absPath := filepath.Join(h.projectRoot, path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte("package hidden\nfunc NewlyVisible() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if doc, err := h.vecStore.GetDocument(ctx, path); err != nil || doc != nil {
		t.Fatalf("setup document = %#v, err=%v", doc, err)
	}
	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	if err := ignore.Refresh(); err != nil {
		t.Fatal(err)
	}

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventReconcile, Path: ".", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, path); err != nil || doc == nil {
		t.Fatalf("newly visible document = %#v, err=%v", doc, err)
	}
	if symbols, err := h.symbolStore.LookupSymbol(ctx, "NewlyVisible"); err != nil || len(symbols) == 0 {
		t.Fatalf("newly visible symbols = %#v, err=%v", symbols, err)
	}
}

func TestHandleFileEventPolicySubtreeDiscoversOnlyScopedUnignoredFiles(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)
	if err := os.MkdirAll(filepath.Join(h.projectRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	ignorePath := filepath.Join(h.projectRoot, "src", ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("*.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(h.projectRoot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.scanner = indexer.NewScanner(h.projectRoot, ignore)
	scopedPath := filepath.Join("src", "new.go")
	outsidePath := filepath.Join("outside", "not-in-scope.go")
	for path, content := range map[string]string{
		scopedPath:  "package src\nfunc ScopedNew() {}\n",
		outsidePath: "package outside\nfunc OutsideNew() {}\n",
	} {
		absPath := filepath.Join(h.projectRoot, path)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	if err := ignore.RefreshSubtree("src"); err != nil {
		t.Fatal(err)
	}

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventReconcile, Path: "src", IsDir: true})

	if doc, err := h.vecStore.GetDocument(ctx, scopedPath); err != nil || doc == nil {
		t.Fatalf("scoped document = %#v, err=%v", doc, err)
	}
	if doc, err := h.vecStore.GetDocument(ctx, outsidePath); err != nil || doc != nil {
		t.Fatalf("out-of-scope document = %#v, err=%v", doc, err)
	}
}
