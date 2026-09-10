package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
)

func removeOfflineSymbolFiles(ctx context.Context, scanner *indexer.Scanner, symbolStore trace.SymbolStore, snapshot map[string]trace.FileFingerprint, scanned []indexer.FileMeta, excluded []string) error {
	result, err := removeOfflineSymbolFilesForScan(ctx, scanner, symbolStore, snapshot, scanned, excluded)
	if err != nil {
		return err
	}
	return consumeSymbolRetirements(ctx, symbolStore, result.retiredAliases)
}

func removeOfflineSymbolFilesForScan(ctx context.Context, scanner *indexer.Scanner, symbolStore trace.SymbolStore, snapshot map[string]trace.FileFingerprint, scanned []indexer.FileMeta, excluded []string) (symbolReconciliation, error) {
	return removeOfflineSymbolFilesForScanWithSeams(ctx, scanner, symbolStore, snapshot, scanned, excluded, indexer.FindCaseRenameWitnesses, scanner.InspectExistingPath)
}

type caseRenameFinder func(string, []string, []indexer.FileMeta) map[string]string
type existingPathInspector func(string) (*indexer.FileInfo, indexer.PathExclusionReason, error)
type symbolReconciliation struct {
	reeligible     []indexer.FileInfo
	retiredAliases []indexer.RetiredAlias
}

func removeOfflineSymbolFilesWithCaseRenames(ctx context.Context, scanner *indexer.Scanner, symbolStore trace.SymbolStore, snapshot map[string]trace.FileFingerprint, scanned []indexer.FileMeta, excluded []string, findCaseRenames caseRenameFinder) error {
	result, err := removeOfflineSymbolFilesForScanWithSeams(ctx, scanner, symbolStore, snapshot, scanned, excluded, findCaseRenames, scanner.InspectExistingPath)
	if err != nil {
		return err
	}
	return consumeSymbolRetirements(ctx, symbolStore, result.retiredAliases)
}

func removeOfflineSymbolFilesForScanWithSeams(ctx context.Context, scanner *indexer.Scanner, symbolStore trace.SymbolStore, snapshot map[string]trace.FileFingerprint, scanned []indexer.FileMeta, excluded []string, findCaseRenames caseRenameFinder, inspect existingPathInspector) (symbolReconciliation, error) {
	if snapshot == nil {
		return symbolReconciliation{}, nil
	}
	root := scanner.Root()
	if _, err := os.Stat(root); err != nil {
		return symbolReconciliation{}, fmt.Errorf("symbol cleanup root unavailable: %w", err)
	}
	seen := make(map[string]struct{}, len(scanned))
	for _, file := range scanned {
		seen[file.Path] = struct{}{}
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, path := range excluded {
		excludedSet[path] = struct{}{}
	}
	indexedPaths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		indexedPaths = append(indexedPaths, path)
	}
	caseRenames := findCaseRenames(root, indexedPaths, scanned)
	var candidates []string
	for path := range snapshot {
		_, intentionallyExcluded := excludedSet[path]
		if _, ok := seen[path]; ok && !intentionallyExcluded {
			continue
		}
		_, caseRenamed := caseRenames[path]
		_, statErr := os.Lstat(filepath.Join(root, path))
		if statErr == nil || os.IsNotExist(statErr) || caseRenamed || intentionallyExcluded {
			candidates = append(candidates, path)
		}
	}
	sort.Strings(candidates)
	result := symbolReconciliation{}
	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		caseWitness, caseRenamed := caseRenames[path]
		_, statErr := os.Lstat(filepath.Join(root, path))
		forced := caseRenamed
		if caseRenamed {
			current, err := indexer.RevalidateCaseRenameWitness(root, path, caseWitness)
			if err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("revalidate symbol case rename %s to %s: %w", path, caseWitness, err)
			}
			if !current {
				file, reason, inspectErr := inspect(path)
				if inspectErr != nil && !os.IsNotExist(inspectErr) {
					return result, fmt.Errorf("inspect reversed symbol case rename %s: %w", path, inspectErr)
				}
				caseRenamed = false
				forced = reason != ""
				if file != nil && reason == "" {
					if err := validateReeligiblePath(file, path); err != nil {
						return result, err
					}
					retireAlias, retireErr := indexer.CanRetireCaseAlias(root, path, caseWitness)
					if retireErr != nil {
						return result, fmt.Errorf("verify temporary symbol case alias %s: %w", caseWitness, retireErr)
					}
					if retireAlias {
						if err := verifyInitialScanRoot(root); err != nil {
							return result, fmt.Errorf("symbol cleanup root lost before reconciling reversed case rename %s: %w", path, err)
						}
						result.retiredAliases = append(result.retiredAliases, indexer.RetiredAlias{Path: caseWitness, CanonicalPath: path})
					}
					result.reeligible = append(result.reeligible, *file)
					continue
				}
				if forced {
					retireAlias, retireErr := indexer.CanRetireCaseAlias(root, path, caseWitness)
					if retireErr != nil {
						return result, fmt.Errorf("verify temporary excluded symbol case alias %s: %w", caseWitness, retireErr)
					}
					if retireAlias {
						if err := verifyInitialScanRoot(root); err != nil {
							return result, fmt.Errorf("symbol cleanup root lost before removing excluded case alias %s: %w", caseWitness, err)
						}
						result.retiredAliases = append(result.retiredAliases, indexer.RetiredAlias{Path: caseWitness, CanonicalPath: path})
					}
				}
			}
		}
		if statErr == nil && !caseRenamed {
			file, reason, inspectErr := inspect(path)
			if inspectErr != nil {
				continue
			}
			if reason == "" && file != nil {
				if err := validateReeligiblePath(file, path); err != nil {
					return result, err
				}
				result.reeligible = append(result.reeligible, *file)
				continue
			}
			forced = reason != ""
		}
		if (statErr == nil || !os.IsNotExist(statErr)) && !forced {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			return result, fmt.Errorf("symbol cleanup root lost before removing %s: %w", path, err)
		}
		if err := symbolStore.DeleteFile(ctx, path); err != nil {
			return result, fmt.Errorf("delete offline symbol file %s: %w", path, err)
		}
	}
	return result, nil
}

func consumeSymbolRetirements(ctx context.Context, symbolStore trace.SymbolStore, aliases []indexer.RetiredAlias) error {
	for _, alias := range aliases {
		if err := symbolStore.DeleteFile(ctx, alias.Path); err != nil {
			return fmt.Errorf("delete retired symbol case alias %s: %w", alias.Path, err)
		}
	}
	return nil
}
