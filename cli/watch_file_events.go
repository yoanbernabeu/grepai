package cli

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/framework"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

func handleFileEvent(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore *trace.GOBSymbolStore, rpgEncoder *rpg.RPGEncoder, vectorStore store.VectorStore, enabledLanguages []string, projectRoot string, cfg *config.Config, lastConfigWrite *time.Time, rpgManager *rpgRealtimeManager, event watcher.FileEvent, onActivity watchActivityObserver, onStats watchStatsObserver, processors ...*framework.ProcessorRegistry) {
	forgetIndex := func(path string, eventType watcher.EventType) {
		var oldChunkCount, oldSymbolCount int
		fileExisted := false
		if vectorStore != nil {
			if doc, err := vectorStore.GetDocument(ctx, path); err == nil && doc != nil {
				oldChunkCount = len(doc.ChunkIDs)
				fileExisted = true
			}
		}
		if symbolStore != nil {
			if syms, err := symbolStore.GetSymbolsForFile(ctx, path); err == nil {
				oldSymbolCount = len(syms)
			}
		}
		if onActivity != nil {
			onActivity("removing", path)
			defer onActivity("steady", "")
		}
		start := time.Now()
		if err := idx.RemoveFile(ctx, path); err != nil {
			log.Printf("Failed to remove %s from index: %v", path, err)
			return
		}
		if err := symbolStore.DeleteFile(ctx, path); err != nil {
			log.Printf("Failed to remove symbols for %s: %v", path, err)
		}
		if onStats != nil {
			delta := watchStatsDelta{ChunksRemoved: oldChunkCount, SymbolsLost: oldSymbolCount}
			if fileExisted {
				delta.FilesRemoved = 1
			}
			if delta.FilesRemoved > 0 || delta.ChunksRemoved > 0 || delta.SymbolsLost > 0 {
				onStats(projectRoot, delta)
			}
		}
		if rpgEncoder != nil {
			if err := rpgEncoder.HandleFileEvent(ctx, "delete", path, nil); err != nil {
				log.Printf("Warning: failed to update RPG for deleted %s: %v", path, err)
			} else if rpgManager != nil {
				rpgManager.MarkFileDirty(path)
				dirtyCount, _, _, _ := rpgManager.Snapshot()
				log.Printf("rpg_event_applied_ms=%d file=%s event=%s rpg_dirty_files_count=%d", time.Since(start).Milliseconds(), path, eventType.String(), dirtyCount)
			}
		}
		log.Printf("Removed %s from index", path)
	}

	if event.IsDir {
		dispatch := func(action directoryAction) {
			err := applyDirectoryAction(action, scanner, func(path string) {
				forgetIndex(path, watcher.EventDelete)
			}, func(fileEvent watcher.FileEvent) {
				handleFileEvent(ctx, idx, scanner, extractor, symbolStore, rpgEncoder, vectorStore, enabledLanguages, projectRoot, cfg, lastConfigWrite, rpgManager, fileEvent, onActivity, onStats, processors...)
			})
			if err != nil {
				log.Printf("Preserving %s after failed reconciliation scan: %v", action.event.Path, err)
			}
		}
		var err error
		if event.Type == watcher.EventReconcile {
			err = reconcilePolicyDirectory(ctx, projectRoot, event.Path, scanner, vectorStore, symbolStore, dispatch)
		} else {
			err = reconcileDeletedDirectory(ctx, projectRoot, event.Path, vectorStore, symbolStore, dispatch)
		}
		if err != nil {
			log.Printf("Failed to reconcile directory %s: %v", event.Path, err)
		}
		return
	}

	// An atomic write -- write to a temp file, then rename it over the target
	// -- surfaces on the destination path as RENAME/REMOVE with no follow-up
	// CREATE or WRITE. Editors and coding agents (Claude Code, Cursor) save
	// this way, so taking the event at face value would drop a file that is
	// still on disk from the index, silently, until the next manual save or
	// watcher restart. Re-qualify the event whenever the path still resolves
	// to a regular file; a genuine delete leaves nothing to stat.
	eventType := event.Type
	if eventType == watcher.EventDelete || eventType == watcher.EventRename {
		qualifiedType, err := requalifyRemovedFile(projectRoot, event, os.Lstat)
		if errors.Is(err, errWatchPathReplaced) {
			log.Printf("Preserving %s after removal event: path has a non-regular replacement", event.Path)
			return
		}
		if err != nil {
			log.Printf("Preserving %s after failed deletion check: %v", event.Path, err)
			return
		}
		if qualifiedType == watcher.EventModify {
			log.Printf("Treating %s of %s as a modification: file is still on disk (atomic write)", eventType.String(), event.Path)
		}
		eventType = qualifiedType
	}
	if eventType == watcher.EventDelete || eventType == watcher.EventRename {
		forgetIndex(event.Path, eventType)
		return
	}

	if onActivity != nil {
		op := "processing"
		if eventType == watcher.EventDelete {
			op = "removing"
		}
		onActivity(op, event.Path)
		defer onActivity("steady", "")
	}

	// Capture previous state for stats delta
	var oldChunkCount int
	var oldSymbolCount int
	var fileExisted bool

	if vectorStore != nil {
		if doc, err := vectorStore.GetDocument(ctx, event.Path); err == nil && doc != nil {
			oldChunkCount = len(doc.ChunkIDs)
			fileExisted = true
		}
	}
	if symbolStore != nil {
		if syms, err := symbolStore.GetSymbolsForFile(ctx, event.Path); err == nil {
			oldSymbolCount = len(syms)
		}
	}

	switch eventType {
	case watcher.EventCreate, watcher.EventModify:
		start := time.Now()
		fileInfo, err := scanner.ScanFile(event.Path)
		if err != nil {
			log.Printf("Failed to scan %s: %v", event.Path, err)
			return
		}
		if fileInfo == nil {
			return // File was skipped (binary, too large, etc.)
		}

		needsReindex, err := idx.NeedsReindex(ctx, fileInfo.Path, fileInfo.Hash)
		if err != nil {
			log.Printf("Failed to check reindex status for %s: %v", event.Path, err)
			return
		}
		if !needsReindex {
			log.Printf("Skipped unchanged %s", event.Path)
			return
		}

		chunks, err := idx.IndexFile(ctx, *fileInfo)
		if err != nil {
			log.Printf("Failed to index %s: %v", event.Path, err)
			return
		}
		log.Printf("Indexed %s (%d chunks)", event.Path, chunks)

		// Report stats (files/chunks)
		if onStats != nil {
			delta := watchStatsDelta{
				ChunksCreated: chunks,
				ChunksRemoved: oldChunkCount,
			}
			if !fileExisted {
				delta.FilesIndexed = 1
			}
			onStats(projectRoot, delta)
		}

		// Update last_index_time with throttling (only write if 30 seconds have passed)
		now := time.Now()
		if now.Sub(*lastConfigWrite) >= configWriteThrottle {
			cfg.Watch.LastIndexTime = now
			if err := cfg.Save(projectRoot); err != nil {
				log.Printf("Warning: failed to save config: %v", err)
			}
			*lastConfigWrite = now
		}

		// Extract symbols if language is supported
		ext := strings.ToLower(filepath.Ext(event.Path))
		if isTracedLanguage(ext, enabledLanguages) {
			symbols, refs, err := extractSymbolsWithFramework(ctx, extractor, fileInfo.Path, fileInfo.Content, processors...)
			if err != nil {
				log.Printf("Failed to extract symbols from %s: %v", event.Path, err)
			} else if err := symbolStore.SaveFileWithSignature(ctx, fileInfo.Path, fileInfo.Hash, extractor.Version(), symbols, refs); err != nil {
				log.Printf("Failed to save symbols for %s: %v", event.Path, err)
			} else {
				log.Printf("Extracted %d symbols from %s", len(symbols), event.Path)

				if onStats != nil {
					onStats(projectRoot, watchStatsDelta{
						SymbolsFound: len(symbols),
						SymbolsLost:  oldSymbolCount,
					})
				}

				// Update RPG graph.
				if rpgEncoder != nil {
					rpgEventType := "create"
					if eventType == watcher.EventModify {
						rpgEventType = "modify"
					}
					if err := rpgEncoder.HandleFileEvent(ctx, rpgEventType, fileInfo.Path, symbols); err != nil {
						log.Printf("Warning: failed to update RPG for %s: %v", event.Path, err)
					}
					if vectorStore != nil {
						if chunks, err := vectorStore.GetChunksForFile(ctx, fileInfo.Path); err == nil {
							if err := rpgEncoder.LinkChunksForFile(ctx, fileInfo.Path, chunks); err != nil {
								log.Printf("Warning: failed to link RPG chunks for %s: %v", event.Path, err)
							}
						}
					}
					if rpgManager != nil {
						rpgManager.MarkFileDirty(fileInfo.Path)
						dirtyCount, _, _, _ := rpgManager.Snapshot()
						log.Printf("rpg_event_applied_ms=%d file=%s event=%s rpg_dirty_files_count=%d",
							time.Since(start).Milliseconds(),
							fileInfo.Path,
							eventType.String(),
							dirtyCount,
						)
					}
				}
			}
		}

	}
}
