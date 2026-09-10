package indexer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/store"
)

type fileScanDecision struct {
	file             *FileInfo
	countAsSkipped   bool
	verifiedPath     string
	verified         *VerifiedFile
	missingAfterWalk bool
	excluded         bool
}

func (idx *Indexer) scanMetadataForReconciliation(ctx context.Context) ([]FileMeta, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := checkScanRoot(idx.root); err != nil {
		return nil, nil, fmt.Errorf("scan root unavailable: %w", err)
	}
	return idx.scanner.ScanMetadata()
}

func (idx *Indexer) loadExistingDocumentMetadata(ctx context.Context) (map[string]store.DocumentMetadata, error) {
	metadata, err := store.LoadDocumentMetadata(ctx, idx.store)
	if err != nil {
		return nil, err
	}
	documents := make(map[string]store.DocumentMetadata, len(metadata))
	for _, meta := range metadata {
		documents[meta.Path] = meta
	}
	return documents, nil
}

func (idx *Indexer) decideFileScanFromMeta(ctx context.Context, fileMeta FileMeta, existing *store.DocumentMetadata) (fileScanDecision, error) {
	if err := ctx.Err(); err != nil {
		return fileScanDecision{}, err
	}
	if idx.allowMetadataFastSkip && existing != nil && existing.HasChunks && existing.Hash != "" && existing.HasExactModTime {
		fresh, err := idx.scanner.StatFile(fileMeta.Path)
		if err != nil {
			log.Printf("Failed to stat %s: %v", fileMeta.Path, err)
			return fileScanDecision{countAsSkipped: true, missingAfterWalk: errors.Is(err, fs.ErrNotExist)}, nil
		}
		if fresh == nil {
			return idx.nilSnapshotDecision(fileMeta.Path), nil
		}
		if hasExactTimestamp(fresh.ObservedModTime) && existing.ModTime.Equal(fresh.ObservedModTime) {
			verified := VerifiedFile{Hash: existing.Hash, Size: fresh.Size, ModTime: fresh.ObservedModTime}
			return fileScanDecision{countAsSkipped: true, verifiedPath: fileMeta.Path, verified: &verified}, nil
		}
	}
	file, err := idx.scanner.ScanFile(fileMeta.Path)
	if err != nil {
		log.Printf("Failed to scan %s: %v", fileMeta.Path, err)
		return fileScanDecision{countAsSkipped: true, missingAfterWalk: errors.Is(err, fs.ErrNotExist)}, nil
	}
	if file == nil {
		return idx.nilSnapshotDecision(fileMeta.Path), nil
	}
	if existing != nil && existing.Hash == file.Hash && existing.HasChunks {
		return idx.refreshMatchingDocument(ctx, file, existing)
	}
	return fileScanDecision{file: file}, nil
}

func verifiedFileDecision(file *FileInfo) fileScanDecision {
	verified := VerifiedFile{Hash: file.Hash, Size: file.Size, ModTime: file.ObservedModTime}
	return fileScanDecision{verifiedPath: file.Path, verified: &verified}
}

func (idx *Indexer) nilSnapshotDecision(path string) fileScanDecision {
	reason, err := idx.scanner.ExistingPathExclusion(path)
	return fileScanDecision{
		countAsSkipped:   true,
		missingAfterWalk: errors.Is(err, fs.ErrNotExist),
		excluded:         err == nil && reason != "",
	}
}

func (idx *Indexer) refreshMatchingDocument(ctx context.Context, file *FileInfo, existing *store.DocumentMetadata) (fileScanDecision, error) {
	refresher, ok := idx.store.(store.DocumentModTimeRefresher)
	if !ok || !hasExactTimestamp(file.ObservedModTime) {
		return verifiedFileDecision(file), nil
	}
	updated, err := refresher.RefreshDocumentModTime(ctx, file.Path, existing.Hash, file.ObservedModTime)
	if errors.Is(err, store.ErrRefreshUnsupported) {
		return verifiedFileDecision(file), nil
	}
	if err != nil {
		return fileScanDecision{}, fmt.Errorf("refresh document timestamp: %w", err)
	}
	if updated {
		return verifiedFileDecision(file), nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		doc, err := idx.store.GetDocument(ctx, file.Path)
		if err != nil {
			return fileScanDecision{}, fmt.Errorf("reload document after timestamp conflict: %w", err)
		}
		fresh, err := idx.scanner.ScanFile(file.Path)
		if err != nil {
			return fileScanDecision{countAsSkipped: true, missingAfterWalk: errors.Is(err, fs.ErrNotExist)}, nil
		}
		if fresh == nil {
			return idx.nilSnapshotDecision(file.Path), nil
		}
		file = fresh
		if doc == nil || len(doc.ChunkIDs) == 0 || doc.Hash != fresh.Hash {
			return fileScanDecision{file: fresh}, nil
		}
		updated, err = refresher.RefreshDocumentModTime(ctx, fresh.Path, doc.Hash, fresh.ObservedModTime)
		if errors.Is(err, store.ErrRefreshUnsupported) {
			return verifiedFileDecision(fresh), nil
		}
		if err != nil {
			return fileScanDecision{}, fmt.Errorf("refresh document timestamp after conflict: %w", err)
		}
		if updated {
			return verifiedFileDecision(fresh), nil
		}
	}
	return fileScanDecision{}, fmt.Errorf("refresh document timestamp: concurrent updates did not stabilize")
}

func (idx *Indexer) removeMissingFilesForScan(ctx context.Context, candidates map[string]store.DocumentMetadata, scanned []FileMeta, forcedRemovals map[string]string) (int, removalReconciliation, error) {
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	witnesses := FindCaseRenameWitnesses(idx.root, paths, scanned)
	for path := range candidates {
		if _, forced := forcedRemovals[path]; forced || witnesses[path] != "" {
			continue
		}
		reason, err := idx.scanner.ExistingPathExclusion(path)
		if err == nil && reason != "" {
			forcedRemovals[path] = string(reason)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("Warning: cannot classify %s (%v); keeping its index entry", path, err)
		}
	}
	return idx.removeCandidatesWithRevalidation(ctx, candidates, forcedRemovals, witnesses)
}

type lstatFunc func(string) (os.FileInfo, error)

func (idx *Indexer) removeMissingFilesWith(ctx context.Context, candidates map[string]store.DocumentMetadata, lstat lstatFunc) (int, error) {
	return idx.removeMissingFilesWithWitnesses(ctx, candidates, nil, lstat)
}

func (idx *Indexer) removeMissingFilesWithWitnesses(ctx context.Context, candidates map[string]store.DocumentMetadata, caseRenames map[string]string, lstat lstatFunc) (int, error) {
	if err := checkScanRoot(idx.root); err != nil {
		return 0, fmt.Errorf("scan root unavailable while reconciling removals: %w", err)
	}
	removed := 0
	for path := range candidates {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		_, caseRenamed := caseRenames[path]
		if _, err := lstat(filepath.Join(idx.root, path)); err == nil && !caseRenamed {
			continue
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) && !caseRenamed {
			log.Printf("Warning: cannot verify %s (%v); keeping its index entry", path, err)
			continue
		}
		if err := checkScanRoot(idx.root); err != nil {
			return removed, fmt.Errorf("scan root lost before removing %s: %w", path, err)
		}
		if err := idx.RemoveFile(ctx, path); err != nil {
			log.Printf("Failed to remove %s: %v", path, err)
			continue
		}
		removed++
	}
	return removed, nil
}

func checkScanRoot(root string) error {
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	_, err = dir.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
