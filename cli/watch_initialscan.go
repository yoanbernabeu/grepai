package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/framework"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
)

type legacyFileFingerprints interface {
	GetFileContentHash(string) (string, bool)
	GetFileExtractorVersion(string) (string, bool)
}

type signatureSaver interface {
	SaveFileWithSignature(context.Context, string, string, string, []trace.Symbol, []trace.Reference) error
}

type initialSymbolFingerprints struct {
	snapshot map[string]trace.FileFingerprint
	legacy   legacyFileFingerprints
}

func loadInitialSymbolFingerprints(ctx context.Context, symbolStore trace.SymbolStore) (initialSymbolFingerprints, error) {
	snapshot, err := trace.LoadFileFingerprints(ctx, symbolStore)
	if err == nil {
		return initialSymbolFingerprints{snapshot: snapshot}, nil
	}
	if !errors.Is(err, trace.ErrFileFingerprintsUnsupported) {
		return initialSymbolFingerprints{}, err
	}
	legacy, ok := symbolStore.(legacyFileFingerprints)
	if !ok {
		return initialSymbolFingerprints{}, fmt.Errorf("symbol store does not support file fingerprints")
	}
	return initialSymbolFingerprints{legacy: legacy}, nil
}

func (f initialSymbolFingerprints) values(path string) (hash, version string, hashOK, versionOK bool) {
	if f.legacy != nil {
		hash, hashOK = f.legacy.GetFileContentHash(path)
		version, versionOK = f.legacy.GetFileExtractorVersion(path)
		return
	}
	value, ok := f.snapshot[path]
	if !ok {
		return "", "", false, false
	}
	return value.ContentHash, value.ExtractorVersion, value.HasContentHash, value.HasExtractorVersion
}

func runInitialScan(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore trace.SymbolStore, tracedLanguages []string, lastIndexTime time.Time, background bool, onScan func(int, int, string), onEmbed func(indexer.BatchProgressInfo), processors ...*framework.ProcessorRegistry) (*indexer.IndexStats, error) {
	fingerprints, err := loadInitialSymbolFingerprints(ctx, symbolStore)
	if err != nil {
		return nil, err
	}
	announceInitialScan(background)
	stats, err := indexInitialFiles(ctx, idx, background, onScan, onEmbed)
	if err != nil {
		return nil, fmt.Errorf("initial indexing failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	announceInitialScanComplete(stats, background)
	if err := consumeRetiredAliases(ctx, idx, scanner, symbolStore, fingerprints.snapshot, stats, stats.RetiredAliases); err != nil {
		return nil, err
	}
	stats.RetiredAliases = nil
	symbolReconciliation, err := removeOfflineSymbolFilesForScan(ctx, scanner, symbolStore, fingerprints.snapshot, stats.ScannedFiles, stats.ExcludedFiles)
	if err != nil {
		return nil, err
	}
	if err := consumeRetiredAliases(ctx, idx, scanner, symbolStore, fingerprints.snapshot, stats, symbolReconciliation.retiredAliases); err != nil {
		return nil, err
	}
	for _, file := range symbolReconciliation.reeligible {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunks, err := idx.IndexFile(ctx, file)
		if err != nil {
			return nil, fmt.Errorf("index re-eligible file %s: %w", file.Path, err)
		}
		stats.FilesIndexed++
		stats.ChunksCreated += chunks
		stats.ScannedFiles = append(stats.ScannedFiles, indexer.FileMeta{Path: file.Path, Size: file.Size, ModTime: file.ModTime, ObservedModTime: file.ObservedModTime})
		stats.ExcludedFiles = withoutPath(stats.ExcludedFiles, file.Path)
	}
	if background {
		log.Println("Building symbol index...")
	} else {
		fmt.Println("Building symbol index...")
	}
	count, symbolChanges, err := indexInitialSymbols(ctx, idx, scanner, extractor, symbolStore, fingerprints, stats.ScannedFiles, stats.VerifiedUnchangedFiles, tracedLanguages, lastIndexTime, processors...)
	if err != nil {
		return nil, err
	}
	stats.ScannedFiles = withoutFilePaths(stats.ScannedFiles, symbolChanges.removed)
	stats.FilesRemoved += len(symbolChanges.removed)
	stats.FilesIndexed += len(symbolChanges.reindexed)
	stats.ChunksCreated += symbolChanges.chunksCreated
	for _, file := range symbolChanges.reindexed {
		stats.ScannedFiles = replaceFileMeta(stats.ScannedFiles, indexer.FileMeta{Path: file.Path, Size: file.Size, ModTime: file.ModTime, ObservedModTime: file.ObservedModTime})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := symbolStore.Persist(ctx); err != nil {
		return nil, fmt.Errorf("persist symbol index: %w", err)
	}
	if background {
		log.Printf("Symbol index built: %d symbols extracted", count)
	} else {
		fmt.Printf("Symbol index built: %d symbols extracted\n", count)
	}
	return stats, nil
}

func indexInitialFiles(ctx context.Context, idx *indexer.Indexer, background bool, onScan func(int, int, string), onEmbed func(indexer.BatchProgressInfo)) (*indexer.IndexStats, error) {
	scanProgress := func(info indexer.ProgressInfo) {
		if onScan != nil {
			onScan(info.Current, info.Total, info.CurrentFile)
		} else if !background {
			printProgress(info.Current, info.Total, info.CurrentFile)
		}
	}
	embedProgress := func(info indexer.BatchProgressInfo) {
		if onEmbed != nil {
			onEmbed(info)
		} else if !background {
			printBatchProgress(info)
		}
	}
	stats, err := idx.IndexAllWithBatchProgress(ctx, scanProgress, embedProgress)
	if !background {
		watchProgressOutput.clear()
		fmt.Println()
	}
	return stats, err
}

type symbolScanChanges struct {
	removed       []string
	reindexed     []indexer.FileInfo
	chunksCreated int
}

func indexInitialSymbols(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore trace.SymbolStore, fingerprints initialSymbolFingerprints, files []indexer.FileMeta, verified map[string]indexer.VerifiedFile, languages []string, lastIndexTime time.Time, processors ...*framework.ProcessorRegistry) (int, symbolScanChanges, error) {
	_ = lastIndexTime // Deprecated: per-file exact observations govern correctness.
	count := 0
	changes := symbolScanChanges{}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return 0, changes, err
		}
		if !isTracedLanguage(strings.ToLower(filepath.Ext(file.Path)), languages) {
			continue
		}
		hash, version, hashOK, versionOK := fingerprints.values(file.Path)
		if known, ok := verified[file.Path]; ok && hashOK && versionOK && hash == known.Hash && version == extractor.Version() {
			fresh, statErr := scanner.StatFile(file.Path)
			if statErr == nil && fresh != nil && fresh.Size == known.Size && fresh.ObservedModTime.Equal(known.ModTime) {
				continue
			}
		}
		info, err := scanner.ScanFile(file.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				removed, removeErr := removeFileMissingDuringSymbolScan(ctx, idx, scanner, symbolStore, file.Path)
				if removeErr != nil {
					return count, changes, removeErr
				}
				if removed {
					changes.removed = append(changes.removed, file.Path)
				}
				continue
			}
			log.Printf("Warning: failed to scan %s for symbols: %v", file.Path, err)
			continue
		}
		if info == nil {
			resolution, err := reconcileEmptySymbolScan(ctx, idx, scanner, symbolStore, file.Path)
			if err != nil {
				return count, changes, err
			}
			if resolution.uncertain != nil {
				log.Printf("Warning: failed to classify %s after empty symbol scan: %v", file.Path, resolution.uncertain)
				continue
			}
			if resolution.removed {
				changes.removed = append(changes.removed, file.Path)
				continue
			}
			changes.reindexed = append(changes.reindexed, *resolution.file)
			changes.chunksCreated += resolution.chunks
			delete(verified, file.Path)
			info = resolution.file
		}
		hash, version, hashOK, versionOK = fingerprints.values(info.Path)
		if hashOK && versionOK && hash == info.Hash && version == extractor.Version() {
			continue
		}
		symbols, refs, err := extractSymbolsWithFramework(ctx, extractor, info.Path, info.Content, processors...)
		if err != nil {
			if ctx.Err() != nil {
				return count, changes, ctx.Err()
			}
			log.Printf("Warning: failed to extract symbols from %s: %v", info.Path, err)
			continue
		}
		if err := ctx.Err(); err != nil {
			return count, changes, err
		}
		if saver, ok := symbolStore.(signatureSaver); ok {
			err = saver.SaveFileWithSignature(ctx, info.Path, info.Hash, extractor.Version(), symbols, refs)
		} else {
			err = symbolStore.SaveFile(ctx, info.Path, symbols, refs)
		}
		if err != nil {
			return count, changes, fmt.Errorf("save symbols for %s: %w", info.Path, err)
		}
		count += len(symbols)
	}
	if err := ctx.Err(); err != nil {
		return count, changes, err
	}
	return count, changes, nil
}

func announceInitialScan(background bool) {
	if background {
		log.Println("Performing initial scan...")
	} else {
		fmt.Println("\nPerforming initial scan...")
	}
}

func announceInitialScanComplete(stats *indexer.IndexStats, background bool) {
	format := "Initial scan complete: %d files indexed, %d chunks created, %d files removed, %d skipped (took %s)"
	args := []any{stats.FilesIndexed, stats.ChunksCreated, stats.FilesRemoved, stats.FilesSkipped, stats.Duration.Round(time.Millisecond)}
	if background {
		log.Printf(format, args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}
