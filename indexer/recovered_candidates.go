package indexer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/store"
)

type removalReconciliation struct {
	reeligible     []FileInfo
	reused         []recoveredVerification
	retiredAliases []RetiredAlias
}

type recoveredVerification struct {
	path     string
	verified VerifiedFile
}

func (idx *Indexer) applyRemovalReconciliation(ctx context.Context, stats *IndexStats, result removalReconciliation) error {
	for _, alias := range result.retiredAliases {
		stats.ScannedFiles = removeFileMetaPath(stats.ScannedFiles, alias.Path)
		stats.ExcludedFiles = removeStringPath(stats.ExcludedFiles, alias.Path)
		stats.RetiredAliases = append(stats.RetiredAliases, alias)
	}
	for _, file := range result.reeligible {
		chunks, err := idx.IndexFile(ctx, file)
		if err != nil {
			return fmt.Errorf("index re-eligible file %s: %w", file.Path, err)
		}
		stats.FilesIndexed++
		stats.ChunksCreated += chunks
		stats.ScannedFiles = append(stats.ScannedFiles, FileMeta{Path: file.Path, Size: file.Size, ModTime: file.ModTime, ObservedModTime: file.ObservedModTime})
		stats.ExcludedFiles = removeStringPath(stats.ExcludedFiles, file.Path)
	}
	for _, recovered := range result.reused {
		verified := recovered.verified
		stats.ScannedFiles = append(stats.ScannedFiles, FileMeta{Path: recovered.path, Size: verified.Size, ModTime: verified.ModTime.Unix(), ObservedModTime: verified.ModTime})
		stats.VerifiedUnchangedFiles[recovered.path] = verified
		stats.ExcludedFiles = removeStringPath(stats.ExcludedFiles, recovered.path)
	}
	return nil
}

func (idx *Indexer) reconcileRecoveredCandidate(ctx context.Context, file *FileInfo, metadata store.DocumentMetadata) (fileScanDecision, error) {
	if metadata.HasChunks && metadata.Hash != "" && metadata.Hash == file.Hash {
		source, ok := idx.store.(store.CompleteDocumentSource)
		if !ok {
			return idx.inspectRecoveredForIndex(file.Path)
		}
		current, err := source.GetCompleteDocument(ctx, file.Path)
		if errors.Is(err, store.ErrCompleteDocumentUnsupported) {
			return idx.inspectRecoveredForIndex(file.Path)
		}
		if err != nil {
			return fileScanDecision{}, fmt.Errorf("load complete recovered document: %w", err)
		}
		if current != nil && current.Hash == file.Hash {
			return idx.refreshRecoveredComplete(ctx, source, file, current)
		}
		return idx.reconcileInvalidRecoveredRecord(ctx, source, file.Path)
	}
	return fileScanDecision{file: file}, nil
}

func (idx *Indexer) inspectRecoveredForIndex(path string) (fileScanDecision, error) {
	fresh, reason, err := idx.scanner.InspectExistingPath(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileScanDecision{countAsSkipped: true, missingAfterWalk: true}, nil
		}
		return fileScanDecision{}, err
	}
	if reason != "" {
		return fileScanDecision{countAsSkipped: true, excluded: true}, nil
	}
	if fresh == nil {
		return fileScanDecision{}, fmt.Errorf("recovered path %s has no stable inspection result", path)
	}
	if err := validateReeligibleFilePath(fresh, path); err != nil {
		return fileScanDecision{}, err
	}
	return fileScanDecision{file: fresh}, nil
}

func (idx *Indexer) reconcileInvalidRecoveredRecord(ctx context.Context, source store.CompleteDocumentSource, path string) (fileScanDecision, error) {
	decision, err := idx.inspectRecoveredForIndex(path)
	if err != nil || decision.file == nil {
		return decision, err
	}
	fresh := decision.file
	current, err := source.GetCompleteDocument(ctx, path)
	if errors.Is(err, store.ErrCompleteDocumentUnsupported) {
		return decision, nil
	}
	if err != nil {
		return fileScanDecision{}, fmt.Errorf("re-observe complete recovered document: %w", err)
	}
	if current == nil || current.Hash != fresh.Hash {
		return decision, nil
	}
	return idx.refreshRecoveredComplete(ctx, source, fresh, current)
}

func (idx *Indexer) refreshRecoveredComplete(ctx context.Context, source store.CompleteDocumentSource, file *FileInfo, doc *store.Document) (fileScanDecision, error) {
	for range 2 {
		if doc == nil || doc.Hash != file.Hash {
			return fileScanDecision{file: file}, nil
		}
		if doc.HasExactModTime && doc.ModTime.Equal(file.ObservedModTime) {
			return verifiedFileDecision(file), nil
		}
		refresher, ok := idx.store.(store.DocumentModTimeRefresher)
		if !ok || !hasExactTimestamp(file.ObservedModTime) {
			return verifiedFileDecision(file), nil
		}
		_, err := refresher.RefreshDocumentModTime(ctx, file.Path, doc.Hash, file.ObservedModTime)
		if errors.Is(err, store.ErrRefreshUnsupported) {
			return verifiedFileDecision(file), nil
		}
		if err != nil {
			return fileScanDecision{}, fmt.Errorf("refresh recovered document timestamp: %w", err)
		}
		decision, err := idx.inspectRecoveredForIndex(file.Path)
		if err != nil || decision.file == nil {
			return decision, err
		}
		file = decision.file
		doc, err = source.GetCompleteDocument(ctx, file.Path)
		if errors.Is(err, store.ErrCompleteDocumentUnsupported) {
			return fileScanDecision{file: file}, nil
		}
		if err != nil {
			return fileScanDecision{}, fmt.Errorf("revalidate complete recovered document: %w", err)
		}
	}
	if doc != nil && doc.Hash == file.Hash && doc.HasExactModTime && doc.ModTime.Equal(file.ObservedModTime) {
		return verifiedFileDecision(file), nil
	}
	return fileScanDecision{}, fmt.Errorf("recovered document %s did not stabilize", file.Path)
}

func validateReeligibleFilePath(file *FileInfo, indexedPath string) error {
	expected := filepath.FromSlash(indexedPath)
	if file.Path != expected {
		return fmt.Errorf("indexed path %q changed to unexpected spelling %q during re-eligibility", expected, file.Path)
	}
	return nil
}

func removeFileMetaPath(files []FileMeta, target string) []FileMeta {
	filtered := files[:0]
	for _, file := range files {
		if file.Path != target {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func removeStringPath(paths []string, target string) []string {
	filtered := paths[:0]
	for _, path := range paths {
		if path != target {
			filtered = append(filtered, path)
		}
	}
	return filtered
}
