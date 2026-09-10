package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type recoveryCASStore struct {
	*store.GOBStore
	once       sync.Once
	path       string
	relative   string
	newContent string
	refreshes  int
}

type neverRefreshRecoveryStore struct {
	*store.GOBStore
	refreshes int
}

func (s *neverRefreshRecoveryStore) RefreshDocumentModTime(context.Context, string, string, time.Time) (bool, error) {
	s.refreshes++
	return false, nil
}

func (s *recoveryCASStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	s.refreshes++
	mutated := false
	var mutationErr error
	s.once.Do(func() {
		mutated = true
		mutationErr = os.WriteFile(s.path, []byte(s.newContent), 0o644)
		if mutationErr != nil {
			return
		}
		info, err := os.Stat(s.path)
		if err != nil {
			mutationErr = err
			return
		}
		hash := sha256.Sum256([]byte(s.newContent))
		if err := s.DeleteByFile(ctx, s.relative); err != nil {
			mutationErr = err
			return
		}
		mutationErr = s.SaveChunks(ctx, []store.Chunk{{ID: "new-chunk", FilePath: s.relative, Vector: []float32{4, 5, 6}}})
		if mutationErr != nil {
			return
		}
		mutationErr = s.SaveDocument(ctx, store.Document{Path: s.relative, Hash: hex.EncodeToString(hash[:]), ModTime: info.ModTime(), ChunkIDs: []string{"new-chunk"}})
	})
	if mutationErr != nil || mutated {
		return false, mutationErr
	}
	return s.GOBStore.RefreshDocumentModTime(ctx, path, expectedHash, modTime)
}

func TestRemainingRecoveryCASConflictPublishesLatestVerification(t *testing.T) {
	ctx, root, absolute, scanner, base, initial := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	newContent := "package newer\n"
	if err := base.SaveDocument(ctx, store.Document{Path: relative, Hash: initial.Hash, ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	st := &recoveryCASStore{GOBStore: base, path: absolute, relative: relative, newContent: newContent}
	embedder := newMockEmbedder()
	idx := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now())
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := scanner.ScanFile(relative)
	if err != nil || latest == nil {
		t.Fatalf("latest=%v err=%v", latest, err)
	}
	verified, ok := stats.VerifiedUnchangedFiles[relative]
	if !ok {
		t.Fatalf("verified=%v", stats.VerifiedUnchangedFiles)
	}
	if verified.Hash != latest.Hash || verified.Size != latest.Size || !verified.ModTime.Equal(latest.ObservedModTime) {
		t.Fatalf("verified=%+v latest=%+v", verified, latest)
	}
	if len(stats.ScannedFiles) != 1 || stats.ScannedFiles[0].Path != relative || stats.ScannedFiles[0].Size != latest.Size || !stats.ScannedFiles[0].ObservedModTime.Equal(latest.ObservedModTime) {
		t.Fatalf("scanned=%v latest=%+v", stats.ScannedFiles, latest)
	}
	if stats.FilesIndexed != 0 || embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
	if st.refreshes != 2 {
		t.Fatalf("refresh attempts=%d, want 2", st.refreshes)
	}
	doc, err := st.GetDocument(ctx, relative)
	if err != nil || doc == nil || doc.Hash != latest.Hash {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
}

func TestRemainingRecoveryReturnsErrorAfterTwoUnstableRefreshes(t *testing.T) {
	ctx, root, _, scanner, base, initial := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	if err := base.SaveDocument(ctx, store.Document{Path: relative, Hash: initial.Hash, ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	st := &neverRefreshRecoveryStore{GOBStore: base}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Now())
	if _, err := idx.IndexAll(ctx); err == nil {
		t.Fatal("unstable recovered timestamp returned success")
	}
	if st.refreshes != 2 {
		t.Fatalf("refresh attempts=%d, want 2", st.refreshes)
	}
}
