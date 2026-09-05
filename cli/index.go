package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/framework"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

var indexWorkspace string

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Update the index once and exit",
	Long: `Scan a project, update its vector, symbol, and optional RPG indexes, then exit.

Unlike grepai watch, this command does not start a file watcher or daemon.
Unchanged files are skipped using the same incremental indexing logic as the
initial scan performed by grepai watch.

Examples:
  grepai index
  grepai index --workspace myworkspace`,
	Args: cobra.NoArgs,
	RunE: runIndex,
}

var (
	indexProjectRunner   = runProjectIndex
	indexWorkspaceRunner = runWorkspaceIndex
)

type workspaceProjectIndexRuntimeFactory func(context.Context, *config.Workspace, config.ProjectEntry, embedder.Embedder, store.VectorStore) (*projectIndexRuntime, error)

func init() {
	indexCmd.Flags().StringVar(&indexWorkspace, "workspace", "", "Workspace name for multi-project indexing")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if indexWorkspace != "" {
		return indexWorkspaceRunner(ctx, indexWorkspace)
	}
	return indexProjectRunner(ctx)
}

// projectIndexRuntime owns the per-project components shared by one-shot
// indexing and the initial indexing phase of watch.
type projectIndexRuntime struct {
	projectRoot     string
	cfg             *config.Config
	vectorStore     store.VectorStore
	ignoreMatcher   *indexer.IgnoreMatcher
	scanner         *indexer.Scanner
	idx             *indexer.Indexer
	extractor       *trace.RegexExtractor
	symbolStore     *trace.GOBSymbolStore
	rpgEncoder      *rpg.RPGEncoder
	rpgStore        rpg.RPGStore
	tracedLanguages []string
	processor       *framework.ProcessorRegistry
}

func newProjectIndexRuntime(ctx context.Context, projectRoot string, cfg *config.Config, emb embedder.Embedder, vectorStore store.VectorStore, extraExtensions []string) (*projectIndexRuntime, error) {
	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, cfg.Ignore, cfg.ExternalGitignore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}

	scanner := indexer.NewScanner(projectRoot, ignoreMatcher).
		WithCustomExtensions(extraExtensions).
		WithCustomExtensions(cfg.Chunking.CustomExtensions)
	chunker := indexer.NewChunker(cfg.Chunking.Size, cfg.Chunking.Overlap)
	processor := buildFrameworkRegistry(cfg)
	idx := indexer.NewIndexer(projectRoot, vectorStore, emb, chunker, scanner, cfg.Watch.LastIndexTime, processor)

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index for %s: %v", projectRoot, err)
	}

	runtime := &projectIndexRuntime{
		projectRoot:     projectRoot,
		cfg:             cfg,
		vectorStore:     vectorStore,
		ignoreMatcher:   ignoreMatcher,
		scanner:         scanner,
		idx:             idx,
		extractor:       trace.NewRegexExtractor(),
		symbolStore:     symbolStore,
		tracedLanguages: cfg.Trace.EnabledLanguages,
		processor:       processor,
	}
	if len(runtime.tracedLanguages) == 0 {
		runtime.tracedLanguages = config.DefaultConfig().Trace.EnabledLanguages
	}

	if cfg.RPG.Enabled {
		runtime.rpgStore = rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
		if err := runtime.rpgStore.Load(ctx); err != nil {
			log.Printf("Warning: failed to load RPG index for %s: %v", projectRoot, err)
		}

		var featureExtractor rpg.FeatureExtractor
		switch cfg.RPG.FeatureMode {
		case "llm", "hybrid":
			if cfg.RPG.LLMEndpoint == "" || cfg.RPG.LLMModel == "" {
				log.Printf("Warning: RPG feature_mode=%q but llm_endpoint or llm_model is empty, falling back to local extractor", cfg.RPG.FeatureMode)
				featureExtractor = rpg.NewLocalExtractor()
			} else {
				featureExtractor = rpg.NewLLMExtractor(rpg.LLMExtractorConfig{
					Provider: cfg.RPG.LLMProvider,
					Model:    cfg.RPG.LLMModel,
					Endpoint: cfg.RPG.LLMEndpoint,
					APIKey:   cfg.RPG.LLMAPIKey,
					Timeout:  time.Duration(cfg.RPG.LLMTimeoutMs) * time.Millisecond,
				})
			}
		default:
			featureExtractor = rpg.NewLocalExtractor()
		}

		runtime.rpgEncoder = rpg.NewRPGEncoder(runtime.rpgStore, featureExtractor, projectRoot, rpg.RPGEncoderConfig{
			DriftThreshold:       cfg.RPG.DriftThreshold,
			MaxTraversalDepth:    cfg.RPG.MaxTraversalDepth,
			FeatureGroupStrategy: cfg.RPG.FeatureGroupStrategy,
		})
	}

	return runtime, nil
}

func (runtime *projectIndexRuntime) runInitialIndex(ctx context.Context, isBackgroundChild bool, onScan func(current, total int, file string), onEmbed func(info indexer.BatchProgressInfo), onRPG func(step string, current, total int), onStats watchStatsObserver) error {
	existingDocuments, err := runtime.vectorStore.ListDocuments(ctx)
	if err != nil {
		return fmt.Errorf("failed to list indexed documents for %s: %w", runtime.projectRoot, err)
	}

	stats, err := runInitialScan(ctx, runtime.idx, runtime.scanner, runtime.extractor, runtime.symbolStore, runtime.tracedLanguages, runtime.cfg.Watch.LastIndexTime, isBackgroundChild, onScan, onEmbed, runtime.processor)
	if err != nil {
		return err
	}
	if err := runtime.removeDeletedSymbols(ctx, existingDocuments, stats.ScannedFiles); err != nil {
		return err
	}

	if stats.FilesIndexed > 0 || stats.ChunksCreated > 0 {
		runtime.cfg.Watch.LastIndexTime = time.Now()
		if err := runtime.cfg.Save(runtime.projectRoot); err != nil {
			log.Printf("Warning: failed to save config for %s: %v", runtime.projectRoot, err)
		}
	}

	if runtime.rpgEncoder != nil {
		if err := runtime.rpgEncoder.BuildFull(ctx, runtime.symbolStore, runtime.vectorStore, onRPG); err != nil {
			log.Printf("Warning: failed to build RPG graph for %s: %v", runtime.projectRoot, err)
		} else {
			rpgStats := runtime.rpgEncoder.Stats()
			log.Printf("RPG graph built for %s: %d nodes, %d edges", runtime.projectRoot, rpgStats.TotalNodes, rpgStats.TotalEdges)
		}
	}

	emitInitialStatsSnapshot(ctx, runtime.vectorStore, runtime.symbolStore, runtime.projectRoot, onStats)

	if err := runtime.vectorStore.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist index for %s: %w", runtime.projectRoot, err)
	}
	if runtime.rpgStore != nil {
		if err := runtime.rpgStore.Persist(ctx); err != nil {
			return fmt.Errorf("failed to persist RPG graph for %s: %w", runtime.projectRoot, err)
		}
	}

	return nil
}

func (runtime *projectIndexRuntime) removeDeletedSymbols(ctx context.Context, existingDocuments []string, scannedFiles []indexer.FileMeta) error {
	present := make(map[string]struct{}, len(scannedFiles))
	for _, file := range scannedFiles {
		present[file.Path] = struct{}{}
	}
	for _, path := range existingDocuments {
		if _, ok := present[path]; ok {
			continue
		}
		if err := runtime.symbolStore.DeleteFile(ctx, path); err != nil {
			return fmt.Errorf("failed to remove symbols for %s: %w", path, err)
		}
	}
	if err := runtime.symbolStore.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist symbol index for %s: %w", runtime.projectRoot, err)
	}
	return nil
}

func (runtime *projectIndexRuntime) close() error {
	var errs []error
	if runtime.symbolStore != nil {
		if err := runtime.symbolStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close symbol store for %s: %w", runtime.projectRoot, err))
		}
	}
	if runtime.rpgStore != nil {
		if err := runtime.rpgStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close RPG store for %s: %w", runtime.projectRoot, err))
		}
	}
	return errors.Join(errs...)
}

func runProjectIndex(ctx context.Context) (resultErr error) {
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("Indexing project: %s\n", projectRoot)
	fmt.Printf("Provider: %s (%s)\n", cfg.Embedder.Provider, cfg.Embedder.Model)
	fmt.Printf("Backend: %s\n", cfg.Store.Backend)

	emb, err := initializeEmbedder(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, emb.Close()) }()

	vectorStore, err := initializeStore(ctx, cfg, projectRoot)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, vectorStore.Close()) }()

	runtime, err := newProjectIndexRuntime(ctx, projectRoot, cfg, emb, vectorStore, nil)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, runtime.close()) }()

	if err := runtime.runInitialIndex(ctx, false, nil, nil, nil, nil); err != nil {
		return err
	}
	fmt.Println("Index update complete.")
	return nil
}

func runWorkspaceIndex(ctx context.Context, workspaceName string) (resultErr error) {
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}
	ws, err := wsCfg.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	fmt.Printf("Indexing workspace: %s\n", ws.Name)
	fmt.Printf("Backend: %s\n", ws.Store.Backend)
	fmt.Printf("Embedder: %s (%s)\n", ws.Embedder.Provider, ws.Embedder.Model)
	fmt.Printf("Projects: %d\n", len(ws.Projects))

	embCfg := &config.Config{Embedder: ws.Embedder}
	emb, err := initializeEmbedder(ctx, embCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize embedder: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, emb.Close()) }()

	sharedStore, err := initializeWorkspaceStore(ctx, ws)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, sharedStore.Close()) }()

	if err := indexWorkspaceProjects(ctx, ws, emb, sharedStore, newWorkspaceProjectIndexRuntime); err != nil {
		return err
	}

	fmt.Println("\nWorkspace index update complete.")
	return nil
}

func indexWorkspaceProjects(ctx context.Context, ws *config.Workspace, emb embedder.Embedder, sharedStore store.VectorStore, runtimeFactory workspaceProjectIndexRuntimeFactory) error {
	var projectErrs []error
	for _, project := range ws.Projects {
		fmt.Printf("\nIndexing project: %s (%s)\n", project.Name, project.Path)
		if _, err := os.Stat(project.Path); err != nil {
			projectErrs = append(projectErrs, fmt.Errorf("%s (%s): %w", project.Name, project.Path, err))
			continue
		}

		runtime, err := runtimeFactory(ctx, ws, project, emb, sharedStore)
		if err != nil {
			projectErrs = append(projectErrs, fmt.Errorf("%s (%s): %w", project.Name, project.Path, err))
			continue
		}
		indexErr := runtime.runInitialIndex(ctx, false, nil, nil, nil, nil)
		closeErr := runtime.close()
		if err := errors.Join(indexErr, closeErr); err != nil {
			projectErrs = append(projectErrs, fmt.Errorf("%s (%s): %w", project.Name, project.Path, err))
		}
	}

	if len(projectErrs) > 0 {
		messages := make([]string, 0, len(projectErrs))
		for _, err := range projectErrs {
			messages = append(messages, err.Error())
		}
		return fmt.Errorf("workspace indexing completed with %d project error(s):\n  - %s", len(projectErrs), strings.Join(messages, "\n  - "))
	}
	return nil
}
