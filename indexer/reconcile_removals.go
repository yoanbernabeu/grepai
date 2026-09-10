package indexer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/store"
)

func (idx *Indexer) removeCandidatesWithRevalidation(ctx context.Context, candidates map[string]store.DocumentMetadata, exclusions, caseRenames map[string]string) (int, removalReconciliation, error) {
	if err := checkScanRoot(idx.root); err != nil {
		return 0, removalReconciliation{}, fmt.Errorf("scan root unavailable while reconciling removals: %w", err)
	}
	removed := 0
	result := removalReconciliation{}
	for path := range candidates {
		if err := ctx.Err(); err != nil {
			return removed, result, err
		}
		_, excluded := exclusions[path]
		caseWitness, caseRenamed := caseRenames[path]
		_, statErr := os.Lstat(filepath.Join(idx.root, path))
		if caseRenamed {
			current, err := RevalidateCaseRenameWitness(idx.root, path, caseWitness)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return removed, result, fmt.Errorf("revalidate case rename %s to %s: %w", path, caseWitness, err)
			}
			if !current {
				file, reason, inspectErr := idx.scanner.InspectExistingPath(path)
				if inspectErr != nil {
					if !errors.Is(inspectErr, fs.ErrNotExist) {
						return removed, result, fmt.Errorf("inspect reversed case rename %s: %w", path, inspectErr)
					}
					if err := idx.confirmInspectionMissing(path); err != nil {
						return removed, result, err
					}
					retireAlias, retireErr := CanRetireCaseAlias(idx.root, path, caseWitness)
					if retireErr != nil {
						return removed, result, fmt.Errorf("verify alias after missing inspection %s: %w", caseWitness, retireErr)
					}
					if retireAlias {
						if err := checkScanRoot(idx.root); err != nil {
							return removed, result, fmt.Errorf("scan root lost before removing alias after missing inspection %s: %w", caseWitness, err)
						}
						if err := idx.RemoveFile(ctx, caseWitness); err != nil {
							return removed, result, fmt.Errorf("remove alias after missing inspection %s: %w", caseWitness, err)
						}
						removed++
						result.retiredAliases = append(result.retiredAliases, RetiredAlias{Path: caseWitness, CanonicalPath: path})
					}
					statErr = os.ErrNotExist
				}
				caseRenamed = false
				if file != nil && reason == "" {
					if err := validateReeligibleFilePath(file, path); err != nil {
						return removed, result, err
					}
					retireAlias, retireErr := CanRetireCaseAlias(idx.root, path, caseWitness)
					if retireErr != nil {
						return removed, result, fmt.Errorf("verify temporary case alias %s: %w", caseWitness, retireErr)
					}
					if retireAlias {
						if err := checkScanRoot(idx.root); err != nil {
							return removed, result, fmt.Errorf("scan root lost before reconciling reversed case rename %s: %w", path, err)
						}
						if err := idx.RemoveFile(ctx, caseWitness); err != nil {
							return removed, result, fmt.Errorf("remove temporary case alias %s: %w", caseWitness, err)
						}
						removed++
						result.retiredAliases = append(result.retiredAliases, RetiredAlias{Path: caseWitness, CanonicalPath: path})
					}
					result.reeligible = append(result.reeligible, *file)
					continue
				}
				excluded = reason != ""
				if excluded {
					retireAlias, retireErr := CanRetireCaseAlias(idx.root, path, caseWitness)
					if retireErr != nil {
						return removed, result, fmt.Errorf("verify temporary excluded case alias %s: %w", caseWitness, retireErr)
					}
					if retireAlias {
						if err := checkScanRoot(idx.root); err != nil {
							return removed, result, fmt.Errorf("scan root lost before removing excluded case alias %s: %w", caseWitness, err)
						}
						if err := idx.RemoveFile(ctx, caseWitness); err != nil {
							return removed, result, fmt.Errorf("remove temporary excluded case alias %s: %w", caseWitness, err)
						}
						removed++
						result.retiredAliases = append(result.retiredAliases, RetiredAlias{Path: caseWitness, CanonicalPath: path})
					}
				}
			}
		}
		if statErr == nil && excluded && !caseRenamed {
			file, reason, err := idx.scanner.InspectExistingPath(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					if confirmErr := idx.confirmInspectionMissing(path); confirmErr != nil {
						return removed, result, confirmErr
					}
					statErr = os.ErrNotExist
				} else {
					log.Printf("Warning: cannot revalidate %s (%v); keeping its index entry", path, err)
					continue
				}
			}
			if reason == "" && file != nil {
				if err := validateReeligibleFilePath(file, path); err != nil {
					return removed, result, err
				}
				result.reeligible = append(result.reeligible, *file)
				continue
			}
			excluded = reason != ""
		}
		if statErr == nil && !excluded && !caseRenamed {
			file, reason, err := idx.scanner.InspectExistingPath(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					if confirmErr := idx.confirmInspectionMissing(path); confirmErr != nil {
						return removed, result, confirmErr
					}
					statErr = os.ErrNotExist
				} else {
					log.Printf("Warning: cannot verify remaining candidate %s (%v); keeping its index entry", path, err)
					continue
				}
			} else if reason == "" {
				if file == nil {
					return removed, result, fmt.Errorf("remaining candidate %s has no stable classification", path)
				}
				if err := validateReeligibleFilePath(file, path); err != nil {
					return removed, result, err
				}
				decision, err := idx.reconcileRecoveredCandidate(ctx, file, candidates[path])
				if err != nil {
					return removed, result, fmt.Errorf("reconcile remaining candidate %s: %w", path, err)
				}
				if decision.file != nil {
					if err := validateReeligibleFilePath(decision.file, path); err != nil {
						return removed, result, err
					}
					result.reeligible = append(result.reeligible, *decision.file)
					continue
				}
				if decision.verified != nil {
					expectedPath := filepath.FromSlash(path)
					if decision.verifiedPath != expectedPath {
						return removed, result, fmt.Errorf("verified path %q changed to unexpected spelling %q during recovery", expectedPath, decision.verifiedPath)
					}
					result.reused = append(result.reused, recoveredVerification{path: decision.verifiedPath, verified: *decision.verified})
					continue
				}
				if decision.missingAfterWalk {
					if confirmErr := idx.confirmInspectionMissing(path); confirmErr != nil {
						return removed, result, confirmErr
					}
					statErr = os.ErrNotExist
				} else if !decision.excluded {
					return removed, result, fmt.Errorf("remaining candidate %s produced no reconciliation decision", path)
				}
			}
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			log.Printf("Warning: cannot verify %s (%v); keeping its index entry", path, statErr)
			continue
		}
		if err := checkScanRoot(idx.root); err != nil {
			return removed, result, fmt.Errorf("scan root lost before removing %s: %w", path, err)
		}
		if err := idx.RemoveFile(ctx, path); err != nil {
			log.Printf("Failed to remove %s: %v", path, err)
			continue
		}
		removed++
	}
	return removed, result, nil
}

func (idx *Indexer) confirmInspectionMissing(path string) error {
	if _, err := os.Lstat(filepath.Join(idx.root, path)); err == nil {
		return fmt.Errorf("path %s reappeared after missing inspection", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("recheck missing path %s: %w", path, err)
	}
	if err := checkScanRoot(idx.root); err != nil {
		return fmt.Errorf("scan root unavailable after missing inspection of %s: %w", path, err)
	}
	return nil
}
