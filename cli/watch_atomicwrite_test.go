package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

// atomicWriteHarness wires the pieces handleFileEvent needs and seeds an
// already-indexed main.go, mirroring a watcher that has completed its
// initial scan.
type atomicWriteHarness struct {
	projectRoot string
	srcPath     string
	idx         *indexer.Indexer
	scanner     *indexer.Scanner
	vecStore    *store.GOBStore
	symbolStore *trace.GOBSymbolStore
	emb         *countingEmbedder
	cfg         *config.Config
	lastWrite   time.Time
}

func newAtomicWriteHarness(t *testing.T) *atomicWriteHarness {
	t.Helper()
	ctx := context.Background()
	projectRoot := t.TempDir()

	srcPath := filepath.Join(projectRoot, "main.go")
	if err := os.WriteFile(srcPath, []byte("package main\n\nfunc original() {}\n"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	emb := &countingEmbedder{}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Time{})

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	t.Cleanup(func() { symbolStore.Close() })

	h := &atomicWriteHarness{
		projectRoot: projectRoot,
		srcPath:     srcPath,
		idx:         idx,
		scanner:     scanner,
		vecStore:    vecStore,
		symbolStore: symbolStore,
		emb:         emb,
		cfg:         config.DefaultConfig(),
	}

	// Index it once, the way a completed initial scan would leave things.
	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventCreate, Path: "main.go"})
	if doc, err := vecStore.GetDocument(ctx, "main.go"); err != nil || doc == nil {
		t.Fatalf("setup: expected main.go to be indexed, got doc=%v err=%v", doc, err)
	}
	return h
}

func (h *atomicWriteHarness) dispatch(ctx context.Context, ev watcher.FileEvent) {
	handleFileEvent(
		ctx, h.idx, h.scanner, trace.NewRegexExtractor(), h.symbolStore,
		nil, h.vecStore, []string{".go"}, h.projectRoot, h.cfg,
		&h.lastWrite, nil, ev, nil, nil,
	)
}

// atomicOverwrite reproduces what an editor or coding agent does when it
// saves: write the new content to a sibling temp file, then rename it over
// the target. On the destination path this surfaces as RENAME/REMOVE.
func (h *atomicWriteHarness) atomicOverwrite(t *testing.T, content string) {
	t.Helper()
	tmp := h.srcPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := os.Rename(tmp, h.srcPath); err != nil {
		t.Fatalf("failed to rename over target: %v", err)
	}
}

// TestHandleFileEvent_AtomicWriteKeepsFileIndexed covers issue #225: an
// atomic write emits RENAME/REMOVE on a path whose file still exists. The
// file must stay in the index and pick up the new content, instead of being
// silently dropped until the next manual save or watcher restart.
func TestHandleFileEvent_AtomicWriteKeepsFileIndexed(t *testing.T) {
	for _, evType := range []watcher.EventType{watcher.EventRename, watcher.EventDelete} {
		t.Run(evType.String(), func(t *testing.T) {
			ctx := context.Background()
			h := newAtomicWriteHarness(t)

			h.atomicOverwrite(t, "package main\n\nfunc afterAtomicWrite() {}\n")
			h.dispatch(ctx, watcher.FileEvent{Type: evType, Path: "main.go"})

			doc, err := h.vecStore.GetDocument(ctx, "main.go")
			if err != nil {
				t.Fatalf("GetDocument failed: %v", err)
			}
			if doc == nil {
				t.Fatal("file was dropped from the index after an atomic write (#225)")
			}
			if len(doc.ChunkIDs) == 0 {
				t.Error("file kept its document but lost all chunks")
			}

			// The new content must be what got indexed, not the old one.
			syms, err := h.symbolStore.LookupSymbol(ctx, "afterAtomicWrite")
			if err != nil {
				t.Fatalf("LookupSymbol failed: %v", err)
			}
			if len(syms) == 0 {
				t.Error("expected the post-rename content to be re-extracted")
			}
			stale, err := h.symbolStore.LookupSymbol(ctx, "original")
			if err != nil {
				t.Fatalf("LookupSymbol failed: %v", err)
			}
			if len(stale) != 0 {
				t.Errorf("expected pre-rename symbols to be replaced, found %d", len(stale))
			}
		})
	}
}

// TestHandleFileEvent_RealDeleteStillRemoves locks in the behavior the fix
// must not regress: when the file is genuinely gone from disk, the delete
// path still runs and the document leaves the index.
func TestHandleFileEvent_RealDeleteStillRemoves(t *testing.T) {
	for _, evType := range []watcher.EventType{watcher.EventRename, watcher.EventDelete} {
		t.Run(evType.String(), func(t *testing.T) {
			ctx := context.Background()
			h := newAtomicWriteHarness(t)

			if err := os.Remove(h.srcPath); err != nil {
				t.Fatalf("failed to remove source file: %v", err)
			}
			h.dispatch(ctx, watcher.FileEvent{Type: evType, Path: "main.go"})

			doc, err := h.vecStore.GetDocument(ctx, "main.go")
			if err != nil {
				t.Fatalf("GetDocument failed: %v", err)
			}
			if doc != nil {
				t.Error("a genuinely deleted file must still be removed from the index")
			}
			syms, err := h.symbolStore.LookupSymbol(ctx, "original")
			if err != nil {
				t.Fatalf("LookupSymbol failed: %v", err)
			}
			if len(syms) != 0 {
				t.Errorf("expected symbols of a deleted file to be dropped, found %d", len(syms))
			}
		})
	}
}

// TestHandleFileEvent_DirectoryRenameStillRemoves guards the case where the
// path resolves to something that is not a regular file: it must not be
// mistaken for a live file and must follow the delete path.
func TestHandleFileEvent_DirectoryRenameStillRemoves(t *testing.T) {
	ctx := context.Background()
	h := newAtomicWriteHarness(t)

	if err := os.Remove(h.srcPath); err != nil {
		t.Fatalf("failed to remove source file: %v", err)
	}
	if err := os.MkdirAll(h.srcPath, 0755); err != nil {
		t.Fatalf("failed to create directory in its place: %v", err)
	}

	h.dispatch(ctx, watcher.FileEvent{Type: watcher.EventRename, Path: "main.go"})

	doc, err := h.vecStore.GetDocument(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc != nil {
		t.Error("a path replaced by a directory must be removed from the index")
	}
}
