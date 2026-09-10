package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
)

const remainingRecoverySource = "package fixture\n\nfunc Stable() int { return 23 }\n"

type remainingCountingEmbedder struct{ calls int }

func (e *remainingCountingEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	return []float32{1, 2, 3}, nil
}

func (e *remainingCountingEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	e.calls += len(texts)
	vectors := make([][]float32, len(texts))
	for i := range vectors {
		vectors[i] = []float32{1, 2, 3}
	}
	return vectors, nil
}

func (*remainingCountingEmbedder) Dimensions() int { return 3 }
func (*remainingCountingEmbedder) Close() error    { return nil }

type remainingRecoveryStore struct {
	*store.GOBStore
	refreshes int
}

func (s *remainingRecoveryStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	s.refreshes++
	return s.GOBStore.RefreshDocumentModTime(ctx, path, expectedHash, modTime)
}

type remainingRecoveryCase struct {
	stats     *indexer.IndexStats
	doc       *store.Document
	fresh     *indexer.FileInfo
	embedded  int
	refreshes int
}

func runRemainingRecoveryCase(t *testing.T, linked, chunkless bool, mutate func(string, time.Time) error) remainingRecoveryCase {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	relative := filepath.Join("linked", "stable.go")
	var absolute string
	if linked {
		target := t.TempDir()
		absolute = filepath.Join(target, "stable.go")
		if err := os.WriteFile(absolute, []byte(remainingRecoverySource), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.Mkdir(filepath.Join(root, "linked"), 0o700); err != nil {
			t.Fatal(err)
		}
		absolute = filepath.Join(root, relative)
		if err := os.WriteFile(absolute, []byte(remainingRecoverySource), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	seedSnapshot, err := scanner.ScanFile(relative)
	if err != nil || seedSnapshot == nil {
		t.Fatalf("seed snapshot=%v err=%v", seedSnapshot, err)
	}
	backend := &remainingRecoveryStore{GOBStore: store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "project", projectPath: root}
	chunker := indexer.NewChunker(512, 50)
	chunkIDs := []string(nil)
	if !chunkless {
		chunks := chunker.ChunkWithContext(relative, seedSnapshot.Content)
		if len(chunks) != 1 {
			t.Fatalf("seed chunks=%d", len(chunks))
		}
		seed := store.Chunk{ID: chunks[0].ID, FilePath: relative, Content: chunks[0].Content, Hash: chunks[0].Hash, ContentHash: chunks[0].ContentHash, Vector: []float32{9, 8, 7}}
		if err := prefixed.SaveChunks(ctx, []store.Chunk{seed}); err != nil {
			t.Fatal(err)
		}
		chunkIDs = []string{prefixed.getPrefix() + "/" + filepath.ToSlash(seed.ID)}
	}
	if err := prefixed.SaveDocument(ctx, store.Document{Path: relative, Hash: seedSnapshot.Hash, ModTime: seedSnapshot.ObservedModTime, HasExactModTime: true, ChunkIDs: chunkIDs}); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		if err := mutate(absolute, seedSnapshot.ObservedModTime); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := scanner.ScanFile(relative)
	if err != nil || fresh == nil {
		t.Fatalf("fresh snapshot=%v err=%v", fresh, err)
	}
	embedder := &remainingCountingEmbedder{}
	idx := indexer.NewIndexer(root, prefixed, embedder, chunker, scanner, time.Unix(1, 0))
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := prefixed.GetDocument(ctx, relative)
	if err != nil || doc == nil {
		t.Fatalf("document=%v err=%v", doc, err)
	}
	return remainingRecoveryCase{stats: stats, doc: doc, fresh: fresh, embedded: embedder.calls, refreshes: backend.refreshes}
}

func TestRemainingUnchangedPathUsesMetadataWithoutEmbedding(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "normal"
		if linked {
			name = "linked-directory"
		}
		t.Run(name, func(t *testing.T) {
			result := runRemainingRecoveryCase(t, linked, false, nil)
			if result.embedded != 0 || result.refreshes != 0 || result.stats.FilesIndexed != 0 || len(result.stats.ScannedFiles) != 1 {
				t.Fatalf("embedded=%d refreshes=%d indexed=%d scanned=%v", result.embedded, result.refreshes, result.stats.FilesIndexed, result.stats.ScannedFiles)
			}
			if _, ok := result.stats.VerifiedUnchangedFiles[result.fresh.Path]; !ok {
				t.Fatalf("verified=%v", result.stats.VerifiedUnchangedFiles)
			}
		})
	}
}

func TestRemainingChangedAndChunklessPathsStillEmbed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		chunkless bool
		mutate    func(string, time.Time) error
	}{
		{name: "changed", mutate: func(path string, _ time.Time) error {
			return os.WriteFile(path, []byte("package fixture\n\nfunc Changed() int { return 42 }\n"), 0o600)
		}},
		{name: "chunkless", chunkless: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runRemainingRecoveryCase(t, true, tc.chunkless, tc.mutate)
			if result.embedded == 0 || result.stats.FilesIndexed != 1 || result.doc.Hash != result.fresh.Hash || len(result.doc.ChunkIDs) == 0 {
				t.Fatalf("embedded=%d indexed=%d doc=%+v freshHash=%s", result.embedded, result.stats.FilesIndexed, result.doc, result.fresh.Hash)
			}
		})
	}
}

func TestRemainingHashMatchWithNewMtimeSkipsEmbedding(t *testing.T) {
	result := runRemainingRecoveryCase(t, true, false, func(path string, prior time.Time) error {
		return os.Chtimes(path, prior.Add(time.Second), prior.Add(time.Second))
	})
	if result.embedded != 0 || result.refreshes != 1 || result.stats.FilesIndexed != 0 || !result.doc.ModTime.Equal(result.fresh.ObservedModTime) || !result.doc.HasExactModTime {
		t.Fatalf("embedded=%d refreshes=%d indexed=%d doc=%+v freshMtime=%v", result.embedded, result.refreshes, result.stats.FilesIndexed, result.doc, result.fresh.ObservedModTime)
	}
}
