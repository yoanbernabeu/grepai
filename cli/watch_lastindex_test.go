package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

func contentSHA256(tb testing.TB, path string) string {
	tb.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("failed to read %s: %v", path, err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// TestRefreshLastIndexTimeAfterScan_RepairsZeroValue ensures a successful
// scan with a missing last_index_time writes the field so the next restart
// can use the mtime fast-path gate.
func TestRefreshLastIndexTimeAfterScan_RepairsZeroValue(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	if !cfg.Watch.LastIndexTime.IsZero() {
		t.Fatalf("expected default LastIndexTime to be zero, got %v", cfg.Watch.LastIndexTime)
	}

	scanStarted := time.Now().Add(-5 * time.Second).Truncate(time.Second)
	refreshLastIndexTimeAfterScan(cfg, projectRoot, &indexer.IndexStats{}, scanStarted)

	if !cfg.Watch.LastIndexTime.Equal(scanStarted) {
		t.Fatalf("expected LastIndexTime to equal scan start %v, got %v", scanStarted, cfg.Watch.LastIndexTime)
	}

	// The stamp is only written to disk after the index is saved (#311).
	if _, err := os.Stat(config.GetConfigPath(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("refresh must not write the config before the index is persisted (stat err: %v)", err)
	}
}

// TestRefreshLastIndexTimeAfterScan_KeepsNonZeroWhenNothingIndexed ensures an
// up-to-date restart does not advance the stamp when nothing was reindexed.
func TestRefreshLastIndexTimeAfterScan_KeepsNonZeroWhenNothingIndexed(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	existing := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	cfg.Watch.LastIndexTime = existing
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	refreshLastIndexTimeAfterScan(cfg, projectRoot, &indexer.IndexStats{}, time.Now())

	if !cfg.Watch.LastIndexTime.Equal(existing) {
		t.Fatalf("expected LastIndexTime to remain %v, got %v", existing, cfg.Watch.LastIndexTime)
	}
}

// TestRefreshLastIndexTimeAfterScan_UpdatesWhenFilesIndexed ensures indexing
// work refreshes the stamp to the scan-start time (not the scan-end time).
func TestRefreshLastIndexTimeAfterScan_UpdatesWhenFilesIndexed(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	existing := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	cfg.Watch.LastIndexTime = existing

	scanStarted := time.Now().Add(-30 * time.Second).Truncate(time.Second)
	refreshLastIndexTimeAfterScan(cfg, projectRoot, &indexer.IndexStats{FilesIndexed: 1}, scanStarted)

	if !cfg.Watch.LastIndexTime.Equal(scanStarted) {
		t.Fatalf("expected LastIndexTime to be scan start %v, got %v", scanStarted, cfg.Watch.LastIndexTime)
	}
}

// TestPersistLastIndexTimeOnShutdown_DoesNotClobberOtherFields ensures the
// shutdown flush writes only watch.last_index_time when a config already
// exists on disk.
func TestPersistLastIndexTimeOnShutdown_DoesNotClobberOtherFields(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Chunking.Size = 256
	cfg.Watch.LastIndexTime = time.Time{}
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Simulate a long-running watch session that advanced the stamp in memory
	// while the user edited chunking.size on disk.
	memCfg := config.DefaultConfig()
	memCfg.Chunking.Size = 999
	memCfg.Watch.LastIndexTime = time.Now().Add(-time.Minute).Truncate(time.Second)

	onDiskBefore, err := config.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if onDiskBefore.Chunking.Size != 256 {
		t.Fatalf("precondition: expected disk chunking size 256, got %d", onDiskBefore.Chunking.Size)
	}

	persistLastIndexTime(projectRoot, memCfg, memCfg.Watch.LastIndexTime)

	onDisk, err := config.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if !onDisk.Watch.LastIndexTime.Equal(memCfg.Watch.LastIndexTime) {
		t.Fatalf("expected stamp %v on disk, got %v", memCfg.Watch.LastIndexTime, onDisk.Watch.LastIndexTime)
	}
	if onDisk.Chunking.Size != 256 {
		t.Fatalf("expected disk chunking size to remain 256, got %d (in-memory clobber)", onDisk.Chunking.Size)
	}
}

// TestPersistLastIndexTimeOnShutdown_WritesConfigWhenMissing ensures a project
// without an on-disk config still gets last_index_time written on shutdown.
func TestPersistLastIndexTimeOnShutdown_WritesConfigWhenMissing(t *testing.T) {
	projectRoot := t.TempDir()
	memCfg := config.DefaultConfig()
	stamp := time.Now().Add(-time.Minute).Truncate(time.Second)
	memCfg.Watch.LastIndexTime = stamp

	persistLastIndexTime(projectRoot, memCfg, stamp)

	loaded, err := config.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !loaded.Watch.LastIndexTime.Equal(stamp) {
		t.Fatalf("expected stamp %v, got %v", stamp, loaded.Watch.LastIndexTime)
	}
}

// TestHandleFileEvent_AdvancesLastIndexTimeInMemoryWhenThrottled ensures a
// successful index always advances the in-memory stamp even when the disk
// write is inside the 30s throttle window.
func TestHandleFileEvent_AdvancesLastIndexTimeOnlyInMemory(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	srcPath := filepath.Join(projectRoot, "main.go")
	srcContent := "package main\n\nfunc real() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
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
	if err := symbolStore.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	defer symbolStore.Close()

	cfg := config.DefaultConfig()
	handleFileEvent(
		ctx,
		idx,
		scanner,
		trace.NewRegexExtractor(),
		symbolStore,
		nil,
		nil,
		[]string{".go"},
		projectRoot,
		cfg,
		nil,
		watcher.FileEvent{Type: watcher.EventCreate, Path: "main.go"},
		nil,
		nil,
	)

	if cfg.Watch.LastIndexTime.IsZero() {
		t.Fatal("expected in-memory last_index_time to advance on successful index")
	}
	// Written to disk only after the next successful index persist (#311).
	if _, err := os.Stat(config.GetConfigPath(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("file event must not write the config before the index is persisted (stat err: %v)", err)
	}
}

func TestIndexTimestampPreservesUserIgnore(t *testing.T) {
	for _, phase := range []string{"initial scan", "file event"} {
		t.Run(phase, func(t *testing.T) {
			h := newAtomicWriteHarness(t)
			// The project config exists, as after grepai init.
			if err := h.cfg.Save(h.projectRoot); err != nil {
				t.Fatal(err)
			}
			onDisk, err := config.Load(h.projectRoot)
			if err != nil {
				t.Fatal(err)
			}
			onDisk.Ignore = []string{"_archive"}
			if err := onDisk.Save(h.projectRoot); err != nil {
				t.Fatal(err)
			}

			if phase == "initial scan" {
				refreshLastIndexTimeAfterScan(h.cfg, h.projectRoot, &indexer.IndexStats{FilesIndexed: 1}, time.Now())
			} else {
				if err := os.WriteFile(h.srcPath, []byte("package main\n\nfunc updated() {}\n"), 0644); err != nil {
					t.Fatal(err)
				}
				h.dispatch(context.Background(), watcher.FileEvent{Type: watcher.EventModify, Path: "main.go"})
			}
			// The stamp is written once the index is saved.
			persistLastIndexTime(h.projectRoot, h.cfg, h.cfg.Watch.LastIndexTime)

			saved, err := config.Load(h.projectRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(saved.Ignore) != 1 || saved.Ignore[0] != "_archive" {
				t.Errorf("user's ignore edits were overwritten: %v", saved.Ignore)
			}
			if !saved.Watch.LastIndexTime.Equal(h.cfg.Watch.LastIndexTime) {
				t.Error("last_index_time was not persisted")
			}
		})
	}
}

func TestRunInitialScan_ReusesHashedFilesForSymbols(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	srcPath := filepath.Join(projectRoot, "main.go")
	srcContent := "package main\n\nfunc real() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
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
	// Zero lastIndexTime: vector phase must hash files, but content matches
	// the seeded document so nothing is re-embedded.
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Time{})

	fileHash := contentSHA256(t, srcPath)
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:     "main.go",
		Hash:     fileHash,
		ChunkIDs: []string{"c1"},
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	if err := symbolStore.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	defer symbolStore.Close()

	extractor := trace.NewRegexExtractor()
	// Seed symbols with a *different* content hash so extraction is required
	// even though the vector document hash matches the file.
	if err := symbolStore.SaveFileWithSignature(ctx, "main.go", "stale-symbol-hash", extractor.Version(), []trace.Symbol{
		{Name: "sentinel", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	callsBefore := scanner.ScanFileCallCount()
	stats, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}
	callsAfter := scanner.ScanFileCallCount()

	// Vector decision phase reads main.go once; the symbol phase must reuse
	// HashedFiles and not scan the same path again.
	if callsAfter-callsBefore != 1 {
		t.Fatalf("expected exactly 1 ScanFile during startup (vector phase only), got %d", callsAfter-callsBefore)
	}

	if emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("expected no embedding calls for unchanged content, got embed=%d embedBatch=%d", emb.embedCalls, emb.embedBatchCalls)
	}

	if stats.HashedFiles != nil {
		t.Fatal("expected HashedFiles to be cleared after the symbol phase")
	}

	realSymbols, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("failed to lookup real symbol: %v", err)
	}
	if len(realSymbols) == 0 {
		t.Fatal("expected real symbols to be extracted from reused HashedFiles content")
	}
}

// failingPersistStore makes Persist fail to simulate a crash or a timeout
// during the index save.
type failingPersistStore struct{ *store.GOBStore }

func (f failingPersistStore) Persist(context.Context) error { return errors.New("disk full") }

// last_index_time must not move past an index that was not saved, or the next
// restart would mtime-skip files whose new content was lost (#311).
func TestPersistProjectPeriodically_KeepsStampWhenIndexNotSaved(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	before := time.Now().Add(-time.Hour).Truncate(time.Second)
	cfg.Watch.LastIndexTime = before
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()

	cfg.Watch.LastIndexTime = time.Now()
	failing := failingPersistStore{store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))}
	if err := persistProjectPeriodically(ctx, newWatchMutationFence(), failing, symbolStore, nil, projectRoot, cfg); err != nil {
		t.Fatal(err)
	}
	onDisk, err := config.Load(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !onDisk.Watch.LastIndexTime.Equal(before) {
		t.Fatalf("stamp advanced to %v although the index was not saved", onDisk.Watch.LastIndexTime)
	}

	ok := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	if err := persistProjectPeriodically(ctx, newWatchMutationFence(), ok, symbolStore, nil, projectRoot, cfg); err != nil {
		t.Fatal(err)
	}
	if onDisk, _ = config.Load(projectRoot); !onDisk.Watch.LastIndexTime.Equal(cfg.Watch.LastIndexTime.Truncate(time.Second)) && !onDisk.Watch.LastIndexTime.Equal(cfg.Watch.LastIndexTime) {
		t.Fatalf("stamp not written after a successful save: got %v want %v", onDisk.Watch.LastIndexTime, cfg.Watch.LastIndexTime)
	}
}
