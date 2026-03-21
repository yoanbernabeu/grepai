package cli

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

func newWorkspaceProjectRuntime(ctx context.Context, ws *config.Workspace, project config.ProjectEntry, emb embedder.Embedder, sharedStore store.VectorStore) (*workspaceProjectRuntime, error) {
	projectCfg := config.DefaultConfig()
	if config.Exists(project.Path) {
		loadedCfg, err := config.Load(project.Path)
		if err != nil {
			log.Printf("Warning: failed to load config for %s, using defaults: %v", project.Name, err)
		} else {
			projectCfg = loadedCfg
		}
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(project.Path, projectCfg.Ignore, projectCfg.ExternalGitignore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}

	scanner := indexer.NewScanner(project.Path, ignoreMatcher)
	chunker := indexer.NewChunker(projectCfg.Chunking.Size, projectCfg.Chunking.Overlap)
	vectorStore := &projectPrefixStore{
		store:         sharedStore,
		workspaceName: ws.Name,
		projectName:   project.Name,
		projectPath:   project.Path,
	}
	idx := indexer.NewIndexer(project.Path, vectorStore, emb, chunker, scanner, projectCfg.Watch.LastIndexTime)
	extractor := trace.NewRegexExtractor()
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(project.Path))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index for %s: %v", project.Path, err)
	}

	tracedLanguages := projectCfg.Trace.EnabledLanguages
	if len(tracedLanguages) == 0 {
		tracedLanguages = []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php", ".lua", ".java", ".cs", ".fs", ".fsx", ".fsi"}
	}

	return &workspaceProjectRuntime{
		project:         project,
		cfg:             projectCfg,
		idx:             idx,
		ignoreMatcher:   ignoreMatcher,
		scanner:         scanner,
		extractor:       extractor,
		symbolStore:     symbolStore,
		vectorStore:     vectorStore,
		tracedLanguages: tracedLanguages,
	}, nil
}

func runWorkspaceProjectInitialScan(ctx context.Context, runtime *workspaceProjectRuntime, isBackgroundChild bool) error {
	stats, err := runInitialScan(
		ctx,
		runtime.idx,
		runtime.scanner,
		runtime.extractor,
		runtime.symbolStore,
		runtime.tracedLanguages,
		runtime.cfg.Watch.LastIndexTime,
		isBackgroundChild,
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if stats.FilesIndexed > 0 || stats.ChunksCreated > 0 {
		runtime.cfg.Watch.LastIndexTime = time.Now()
		if err := runtime.cfg.Save(runtime.project.Path); err != nil {
			log.Printf("Warning: failed to save config for %s: %v", runtime.project.Name, err)
		}
	}
	return nil
}

func runWorkspaceProjectStartupRefresh(ctx context.Context, ws *config.Workspace, project config.ProjectEntry, emb embedder.Embedder, sharedStore store.VectorStore, isBackgroundChild bool) error {
	runtime, err := newWorkspaceProjectRuntime(ctx, ws, project, emb, sharedStore)
	if err != nil {
		return err
	}
	defer runtime.symbolStore.Close()
	return runWorkspaceProjectInitialScan(ctx, runtime, isBackgroundChild)
}
