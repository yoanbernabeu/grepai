package trace

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestGOBFileFingerprintsIncludeZeroSymbolFilesAndDetach(t *testing.T) {
	ctx := context.Background()
	st := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := st.SaveFileWithSignature(ctx, "empty.go", "hash", "v1", nil, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadFileFingerprints(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot["empty.go"]
	if !ok || !got.HasContentHash || !got.HasExtractorVersion || got.ContentHash != "hash" || got.ExtractorVersion != "v1" {
		t.Fatalf("fingerprint=%+v present=%v", got, ok)
	}
	if err := st.DeleteFile(ctx, "empty.go"); err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot["empty.go"]; !ok {
		t.Fatal("snapshot changed after store mutation")
	}
}

func TestLoadFileFingerprintsUnsupportedAndSourceError(t *testing.T) {
	bare := &bareFingerprintStore{SymbolStore: NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))}
	if got, err := LoadFileFingerprints(context.Background(), bare); got != nil || !errors.Is(err, ErrFileFingerprintsUnsupported) {
		t.Fatalf("snapshot=%v err=%v", got, err)
	}
	want := errors.New("snapshot failed")
	failing := &failingFingerprintStore{SymbolStore: bare, err: want}
	if _, err := LoadFileFingerprints(context.Background(), failing); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestGOBSignatureSavePublishesOneCoherentSnapshot(t *testing.T) {
	ctx := context.Background()
	st := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	path := "same.go"
	oldSymbol := Symbol{Name: "Old", File: path, Line: 1}
	if err := st.SaveFileWithSignature(ctx, path, "hash-old", "version-old", []Symbol{oldSymbol}, nil); err != nil {
		t.Fatal(err)
	}

	// Pause a same-path replacement after its hash and symbols have changed but
	// before its extractor version is assigned. The snapshot must remain
	// blocked because every part of the tuple is published under one lock.
	entered := make(chan struct{})
	release := make(chan struct{})
	locked := make(chan bool, 1)
	writerDone := make(chan error, 1)
	newVersion := "version-new"
	newSymbol := Symbol{Name: "New", File: path, Line: 2}
	go func() {
		writerDone <- st.saveFile(ctx, path, "hash-new", &newVersion, []Symbol{newSymbol}, nil, func() {
			if st.mu.TryRLock() {
				st.mu.RUnlock()
				locked <- false
			} else {
				locked <- true
			}
			close(entered)
			<-release
		})
	}()
	<-entered
	if !<-locked {
		close(release)
		<-writerDone
		t.Fatal("fingerprint tuple was partially published without the store lock")
	}

	type snapshotResult struct {
		values map[string]FileFingerprint
		err    error
	}
	snapshotDone := make(chan snapshotResult, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		values, err := st.ListFileFingerprints(ctx)
		snapshotDone <- snapshotResult{values: values, err: err}
	}()
	<-started
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	result := <-snapshotDone
	if result.err != nil {
		t.Fatal(result.err)
	}
	fingerprint := result.values[path]
	if fingerprint.ContentHash != "hash-new" || fingerprint.ExtractorVersion != newVersion {
		t.Fatalf("mixed fingerprint tuple: %+v", fingerprint)
	}
	symbols, err := st.GetSymbolsForFile(ctx, path)
	if err != nil || len(symbols) != 1 || symbols[0].Name != "New" {
		t.Fatalf("symbols=%v err=%v", symbols, err)
	}
}

func TestConcurrentPublicSignatureSavesKeepHashVersionPaired(t *testing.T) {
	ctx := context.Background()
	st := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := st.SaveFileWithSignature(ctx, "same.go", "hash-a", "version-a", nil, nil); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var writers sync.WaitGroup
	for _, name := range []string{"a", "b"} {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			for range 1000 {
				if err := st.SaveFileWithSignature(ctx, "same.go", "hash-"+name, "version-"+name, nil, nil); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	close(start)
	var mixed *FileFingerprint
	for range 2000 {
		values, err := st.ListFileFingerprints(ctx)
		if err != nil {
			t.Error(err)
			break
		}
		value := values["same.go"]
		if (value.ContentHash == "hash-a" && value.ExtractorVersion != "version-a") ||
			(value.ContentHash == "hash-b" && value.ExtractorVersion != "version-b") {
			mixed = &value
			break
		}
	}
	writers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if mixed != nil {
		t.Fatalf("public SaveFileWithSignature published a mixed tuple: %+v", *mixed)
	}
}

func TestGOBLegacySaveFingerprintSemanticsRemainUnchanged(t *testing.T) {
	ctx := context.Background()
	st := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := st.SaveFileWithSignature(ctx, "a.go", "hash-one", "version-one", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFileWithContentHash(ctx, "a.go", "hash-two", nil, nil); err != nil {
		t.Fatal(err)
	}
	if hash, ok := st.GetFileContentHash("a.go"); !ok || hash != "hash-two" {
		t.Fatalf("content hash=%q present=%v", hash, ok)
	}
	if version, ok := st.GetFileExtractorVersion("a.go"); !ok || version != "version-one" {
		t.Fatalf("extractor version=%q present=%v", version, ok)
	}
	if err := st.SaveFile(ctx, "a.go", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.GetFileContentHash("a.go"); ok {
		t.Fatal("legacy SaveFile retained content hash")
	}
	if version, ok := st.GetFileExtractorVersion("a.go"); !ok || version != "version-one" {
		t.Fatalf("legacy SaveFile changed extractor version=%q present=%v", version, ok)
	}
	if err := st.SaveFileWithSignature(ctx, "a.go", "", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.GetFileContentHash("a.go"); ok {
		t.Fatal("empty signature retained content hash")
	}
	if _, ok := st.GetFileExtractorVersion("a.go"); ok {
		t.Fatal("empty signature retained extractor version")
	}
}

type bareFingerprintStore struct{ SymbolStore }
type failingFingerprintStore struct {
	SymbolStore
	err error
}

func (s *failingFingerprintStore) ListFileFingerprints(context.Context) (map[string]FileFingerprint, error) {
	return nil, s.err
}
