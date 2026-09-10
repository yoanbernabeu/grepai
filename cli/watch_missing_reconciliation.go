package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
)

func removeFileMissingDuringSymbolScan(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, path string) (bool, error) {
	_, err := os.Lstat(filepath.Join(scanner.Root(), path))
	if err == nil {
		return false, fmt.Errorf("missing symbol path %s reappeared before cleanup", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("recheck missing symbol path %s: %w", path, err)
	}
	if err := verifyInitialScanRoot(scanner.Root()); err != nil {
		return false, fmt.Errorf("symbol scan root unavailable before removing %s: %w", path, err)
	}
	if err := idx.RemoveFile(ctx, path); err != nil {
		return false, fmt.Errorf("remove missing vector file %s: %w", path, err)
	}
	if err := symbolStore.DeleteFile(ctx, path); err != nil {
		return false, fmt.Errorf("remove missing symbol file %s: %w", path, err)
	}
	return true, nil
}

func removeExcludedDuringSymbolScan(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, path string, reason indexer.PathExclusionReason) error {
	if err := verifyInitialScanRoot(scanner.Root()); err != nil {
		return fmt.Errorf("symbol scan root unavailable before removing excluded %s: %w", path, err)
	}
	if err := idx.RemoveFile(ctx, path); err != nil {
		return fmt.Errorf("remove excluded vector file %s: %w", path, err)
	}
	if err := symbolStore.DeleteFile(ctx, path); err != nil {
		return fmt.Errorf("remove excluded symbol file %s (%s): %w", path, reason, err)
	}
	return nil
}

type emptySymbolScanResolution struct {
	file      *indexer.FileInfo
	removed   bool
	chunks    int
	uncertain error
}

func reconcileEmptySymbolScan(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, path string) (emptySymbolScanResolution, error) {
	return reconcileEmptySymbolScanWithInspect(ctx, idx, scanner, symbolStore, path, scanner.InspectExistingPath)
}

func reconcileEmptySymbolScanWithInspect(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, path string, inspect existingPathInspector) (emptySymbolScanResolution, error) {
	fresh, reason, err := inspect(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return emptySymbolScanResolution{uncertain: err}, nil
		}
		removed, removeErr := removeFileMissingDuringSymbolScan(ctx, idx, scanner, symbolStore, path)
		return emptySymbolScanResolution{removed: removed}, removeErr
	}
	if reason != "" {
		if err := removeExcludedDuringSymbolScan(ctx, idx, scanner, symbolStore, path, reason); err != nil {
			return emptySymbolScanResolution{}, err
		}
		return emptySymbolScanResolution{removed: true}, nil
	}
	if fresh == nil {
		return emptySymbolScanResolution{}, fmt.Errorf("symbol scan state changed for %s without a stable snapshot", path)
	}
	if err := validateReeligiblePath(fresh, path); err != nil {
		return emptySymbolScanResolution{}, err
	}
	chunks, err := idx.IndexFile(ctx, *fresh)
	if err != nil {
		return emptySymbolScanResolution{}, fmt.Errorf("repair vector index for %s after symbol scan change: %w", path, err)
	}
	return emptySymbolScanResolution{file: fresh, chunks: chunks}, nil
}

func verifyInitialScanRoot(root string) error {
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

func withoutFilePaths(files []indexer.FileMeta, removed []string) []indexer.FileMeta {
	if len(removed) == 0 {
		return files
	}
	removedSet := make(map[string]struct{}, len(removed))
	for _, path := range removed {
		removedSet[path] = struct{}{}
	}
	filtered := files[:0]
	for _, file := range files {
		if _, ok := removedSet[file.Path]; !ok {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func withoutPath(paths []string, removed string) []string {
	filtered := paths[:0]
	for _, path := range paths {
		if path != removed {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func consumeRetiredAliases(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, fingerprints map[string]trace.FileFingerprint, stats *indexer.IndexStats, aliases []indexer.RetiredAlias) error {
	return consumeRetiredAliasesWithInspect(ctx, idx, scanner, symbolStore, fingerprints, stats, aliases, scanner.InspectExistingPath)
}

func consumeRetiredAliasesWithInspect(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, symbolStore trace.SymbolStore, fingerprints map[string]trace.FileFingerprint, stats *indexer.IndexStats, aliases []indexer.RetiredAlias, inspect existingPathInspector) error {
	for _, alias := range aliases {
		retire, err := indexer.CanRetireCaseAlias(scanner.Root(), alias.CanonicalPath, alias.Path)
		if err != nil {
			return fmt.Errorf("revalidate retired case alias %s: %w", alias.Path, err)
		}
		if !retire {
			file, reason, inspectErr := inspect(alias.Path)
			if inspectErr != nil {
				return fmt.Errorf("inspect reappeared case alias %s: %w", alias.Path, inspectErr)
			}
			if reason != "" {
				return fmt.Errorf("retired case alias %s changed to excluded state %q", alias.Path, reason)
			}
			if file == nil {
				return fmt.Errorf("retired case alias %s changed state without a stable snapshot", alias.Path)
			}
			if err := validateReeligiblePath(file, alias.Path); err != nil {
				return err
			}
			chunks, err := idx.IndexFile(ctx, *file)
			if err != nil {
				return fmt.Errorf("index reappeared case alias %s: %w", alias.Path, err)
			}
			stats.FilesIndexed++
			stats.ChunksCreated += chunks
			stats.ScannedFiles = appendFileMetaIfMissing(stats.ScannedFiles, indexer.FileMeta{Path: file.Path, Size: file.Size, ModTime: file.ModTime, ObservedModTime: file.ObservedModTime})
			delete(stats.VerifiedUnchangedFiles, file.Path)
			continue
		}
		if err := verifyInitialScanRoot(scanner.Root()); err != nil {
			return fmt.Errorf("root unavailable before consuming retired alias %s: %w", alias.Path, err)
		}
		if err := idx.RemoveFile(ctx, alias.Path); err != nil {
			return fmt.Errorf("remove retired vector case alias %s: %w", alias.Path, err)
		}
		if err := symbolStore.DeleteFile(ctx, alias.Path); err != nil {
			return fmt.Errorf("remove retired symbol case alias %s: %w", alias.Path, err)
		}
		delete(fingerprints, alias.Path)
		stats.ScannedFiles = withoutFilePaths(stats.ScannedFiles, []string{alias.Path})
		stats.ExcludedFiles = withoutPath(stats.ExcludedFiles, alias.Path)
	}
	return nil
}

func validateReeligiblePath(file *indexer.FileInfo, indexedPath string) error {
	expected := filepath.FromSlash(indexedPath)
	if file.Path != expected {
		return fmt.Errorf("indexed path %q changed to unexpected spelling %q during re-eligibility", expected, file.Path)
	}
	return nil
}

func appendFileMetaIfMissing(files []indexer.FileMeta, candidate indexer.FileMeta) []indexer.FileMeta {
	for _, file := range files {
		if file.Path == candidate.Path {
			return files
		}
	}
	return append(files, candidate)
}

func replaceFileMeta(files []indexer.FileMeta, candidate indexer.FileMeta) []indexer.FileMeta {
	for i := range files {
		if files[i].Path == candidate.Path {
			files[i] = candidate
			return files
		}
	}
	return append(files, candidate)
}
