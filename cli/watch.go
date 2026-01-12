package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Start the real-time file watcher daemon",
	Long: `Start a background process that monitors file changes and maintains the index.

The watcher will:
- Perform an initial scan comparing disk state with existing index
- Remove obsolete entries and index new files
- Monitor filesystem events (create, modify, delete, rename)
- Apply debouncing (500ms) to batch rapid changes
- Handle atomic updates to avoid duplicate vectors`,
	RunE: runWatch,
}

func runWatch(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Find project root
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("Starting grepai watch in %s\n", projectRoot)
	fmt.Printf("Provider: %s (%s)\n", cfg.Embedder.Provider, cfg.Embedder.Model)
	fmt.Printf("Backend: %s\n", cfg.Store.Backend)

	// Initialize embedder
	var emb embedder.Embedder
	switch cfg.Embedder.Provider {
	case "ollama":
		ollamaEmb := embedder.NewOllamaEmbedder(
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
			embedder.WithOllamaDimensions(cfg.Embedder.Dimensions),
		)
		// Test connection
		if err := ollamaEmb.Ping(ctx); err != nil {
			return fmt.Errorf("cannot connect to Ollama: %w\nMake sure Ollama is running and has the %s model", err, cfg.Embedder.Model)
		}
		emb = ollamaEmb
	case "openai":
		var err error
		emb, err = embedder.NewOpenAIEmbedder(
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOpenAIDimensions(cfg.Embedder.Dimensions),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		lmstudioEmb := embedder.NewLMStudioEmbedder(
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
			embedder.WithLMStudioDimensions(cfg.Embedder.Dimensions),
		)
		if err := lmstudioEmb.Ping(ctx); err != nil {
			return fmt.Errorf("cannot connect to LM Studio: %w\nMake sure LM Studio is running with the %s model loaded", err, cfg.Embedder.Model)
		}
		emb = lmstudioEmb
	default:
		return fmt.Errorf("unknown embedding provider: %s", cfg.Embedder.Provider)
	}
	defer emb.Close()

	// Initialize store
	var st store.VectorStore
	switch cfg.Store.Backend {
	case "gob":
		indexPath := config.GetIndexPath(projectRoot)
		gobStore := store.NewGOBStore(indexPath)
		if err := gobStore.Load(ctx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}
		st = gobStore
	case "postgres":
		var err error
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.Dimensions)
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
	default:
		return fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
	defer st.Close()

	// Initialize ignore matcher
	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, cfg.Ignore)
	if err != nil {
		return fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}

	// Initialize scanner
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)

	// Initialize chunker
	chunker := indexer.NewChunker(cfg.Chunking.Size, cfg.Chunking.Overlap)

	// Initialize indexer
	idx := indexer.NewIndexer(projectRoot, st, emb, chunker, scanner)

	// Initialize symbol store and extractor
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index: %v", err)
	}
	defer symbolStore.Close()

	extractor := trace.NewRegexExtractor()

	// Use default trace languages if not configured
	tracedLanguages := cfg.Trace.EnabledLanguages
	if len(tracedLanguages) == 0 {
		tracedLanguages = []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php"}
	}

	// Initial scan with progress
	fmt.Println("\nPerforming initial scan...")
	stats, err := idx.IndexAllWithProgress(ctx, func(info indexer.ProgressInfo) {
		printProgress(info.Current, info.Total, info.CurrentFile)
	})
	// Clear progress line
	fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
	if err != nil {
		return fmt.Errorf("initial indexing failed: %w", err)
	}

	fmt.Printf("Initial scan complete: %d files indexed, %d chunks created, %d files removed, %d skipped (took %s)\n",
		stats.FilesIndexed, stats.ChunksCreated, stats.FilesRemoved, stats.FilesSkipped, stats.Duration.Round(time.Millisecond))

	// Save index after initial scan
	if err := st.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist index: %v", err)
	}

	// Index symbols for traced languages
	fmt.Println("Building symbol index...")
	symbolCount := 0
	files, _, _ := scanner.Scan()
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Path))
		if !isTracedLanguage(ext, tracedLanguages) {
			continue
		}
		symbols, refs, err := extractor.ExtractAll(ctx, file.Path, file.Content)
		if err != nil {
			log.Printf("Warning: failed to extract symbols from %s: %v", file.Path, err)
			continue
		}
		if err := symbolStore.SaveFile(ctx, file.Path, symbols, refs); err != nil {
			log.Printf("Warning: failed to save symbols for %s: %v", file.Path, err)
		}
		symbolCount += len(symbols)
	}
	if err := symbolStore.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist symbol index: %v", err)
	}
	fmt.Printf("Symbol index built: %d symbols extracted\n", symbolCount)

	// Initialize watcher
	w, err := watcher.NewWatcher(projectRoot, ignoreMatcher, cfg.Watch.DebounceMs)
	if err != nil {
		return fmt.Errorf("failed to initialize watcher: %w", err)
	}
	defer w.Close()

	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	fmt.Println("\nWatching for changes... (Press Ctrl+C to stop)")

	// Periodic persist ticker
	persistTicker := time.NewTicker(30 * time.Second)
	defer persistTicker.Stop()

	// Event loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index on shutdown: %v", err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index on shutdown: %v", err)
			}
			return nil

		case <-persistTicker.C:
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index: %v", err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index: %v", err)
			}

		case event := <-w.Events():
			handleFileEvent(ctx, idx, scanner, extractor, symbolStore, tracedLanguages, event)
		}
	}
}

func handleFileEvent(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore *trace.GOBSymbolStore, enabledLanguages []string, event watcher.FileEvent) {
	log.Printf("[%s] %s", event.Type, event.Path)

	switch event.Type {
	case watcher.EventCreate, watcher.EventModify:
		fileInfo, err := scanner.ScanFile(event.Path)
		if err != nil {
			log.Printf("Failed to scan %s: %v", event.Path, err)
			return
		}
		if fileInfo == nil {
			return // File was skipped (binary, too large, etc.)
		}

		chunks, err := idx.IndexFile(ctx, *fileInfo)
		if err != nil {
			log.Printf("Failed to index %s: %v", event.Path, err)
			return
		}
		log.Printf("Indexed %s (%d chunks)", event.Path, chunks)

		// Extract symbols if language is supported
		ext := strings.ToLower(filepath.Ext(event.Path))
		if isTracedLanguage(ext, enabledLanguages) {
			symbols, refs, err := extractor.ExtractAll(ctx, fileInfo.Path, fileInfo.Content)
			if err != nil {
				log.Printf("Failed to extract symbols from %s: %v", event.Path, err)
			} else if err := symbolStore.SaveFile(ctx, fileInfo.Path, symbols, refs); err != nil {
				log.Printf("Failed to save symbols for %s: %v", event.Path, err)
			} else {
				log.Printf("Extracted %d symbols from %s", len(symbols), event.Path)
			}
		}

	case watcher.EventDelete, watcher.EventRename:
		if err := idx.RemoveFile(ctx, event.Path); err != nil {
			log.Printf("Failed to remove %s from index: %v", event.Path, err)
			return
		}
		// Also remove from symbol index
		if err := symbolStore.DeleteFile(ctx, event.Path); err != nil {
			log.Printf("Failed to remove symbols for %s: %v", event.Path, err)
		}
		log.Printf("Removed %s from index", event.Path)
	}
}

// isTracedLanguage checks if a file extension is in the enabled languages list.
func isTracedLanguage(ext string, enabledLanguages []string) bool {
	for _, lang := range enabledLanguages {
		if ext == lang {
			return true
		}
	}
	return false
}

// printProgress displays a progress bar for indexing
func printProgress(current, total int, filePath string) {
	if total == 0 {
		return
	}

	// Calculate percentage
	percent := float64(current) / float64(total) * 100

	// Build progress bar (20 chars width)
	barWidth := 20
	filled := int(float64(barWidth) * float64(current) / float64(total))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Truncate file path if too long
	maxPathLen := 35
	displayPath := filePath
	if len(filePath) > maxPathLen {
		displayPath = "..." + filePath[len(filePath)-maxPathLen+3:]
	}

	// Print with carriage return to overwrite previous line
	fmt.Printf("\rIndexing [%s] %3.0f%% (%d/%d) %s", bar, percent, current, total, displayPath)
}
