package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

func TestRunIndexRoutesProjectAndWorkspace(t *testing.T) {
	oldProjectRunner := indexProjectRunner
	oldWorkspaceRunner := indexWorkspaceRunner
	oldWorkspace := indexWorkspace
	defer func() {
		indexProjectRunner = oldProjectRunner
		indexWorkspaceRunner = oldWorkspaceRunner
		indexWorkspace = oldWorkspace
	}()

	var projectCalls int
	var workspaceCalls []string
	indexProjectRunner = func(context.Context) error {
		projectCalls++
		return nil
	}
	indexWorkspaceRunner = func(_ context.Context, name string) error {
		workspaceCalls = append(workspaceCalls, name)
		return nil
	}
	cmd := &cobra.Command{}

	indexWorkspace = ""
	if err := runIndex(cmd, nil); err != nil {
		t.Fatalf("runIndex(project) failed: %v", err)
	}
	indexWorkspace = "team"
	if err := runIndex(cmd, nil); err != nil {
		t.Fatalf("runIndex(workspace) failed: %v", err)
	}

	if projectCalls != 1 {
		t.Fatalf("expected one project call, got %d", projectCalls)
	}
	if len(workspaceCalls) != 1 || workspaceCalls[0] != "team" {
		t.Fatalf("expected workspace runner for team, got %v", workspaceCalls)
	}
	if err := indexCmd.Args(indexCmd, []string{"project-path"}); err == nil {
		t.Fatal("expected positional path to be rejected")
	}
}

func TestRunProjectIndexRequiresInitializedProject(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	err = runProjectIndex(t.Context())
	if err == nil || !strings.Contains(err.Error(), "no grepai project found") {
		t.Fatalf("expected uninitialized project error, got %v", err)
	}
}

func TestRunWorkspaceIndexRequiresConfiguredWorkspace(t *testing.T) {
	cleanupHome := setTestHomeDirCLI(t, t.TempDir())
	defer cleanupHome()

	err := runWorkspaceIndex(t.Context(), "missing")
	if err == nil || !strings.Contains(err.Error(), "no workspaces configured") {
		t.Fatalf("expected missing workspace config error, got %v", err)
	}
}

func TestProjectIndexRuntimeSynchronizesChangesAndSymbols(t *testing.T) {
	ctx := t.Context()
	projectRoot := t.TempDir()
	writeTestSource(t, projectRoot, "a.go", "package sample\n\nfunc Alpha() {}\n")
	writeTestSource(t, projectRoot, "b.go", "package sample\n\nfunc Beta() {}\n")

	cfg := config.DefaultConfig()
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatalf("save config: %v", err)
	}
	emb := &countingEmbedder{}
	vectorStore := store.NewGOBStore(config.GetIndexPath(projectRoot))
	if err := vectorStore.Load(ctx); err != nil {
		t.Fatalf("load vector store: %v", err)
	}
	runtime, err := newProjectIndexRuntime(ctx, projectRoot, cfg, emb, vectorStore, nil)
	if err != nil {
		t.Fatalf("newProjectIndexRuntime: %v", err)
	}
	if err := runtime.runInitialIndex(ctx, true, nil, nil, nil, nil); err != nil {
		t.Fatalf("first index: %v", err)
	}
	if emb.embedCalls+emb.embedBatchCalls == 0 {
		t.Fatal("expected first index to call the embedder")
	}
	if symbols, err := runtime.symbolStore.LookupSymbol(ctx, "Beta"); err != nil || len(symbols) == 0 {
		t.Fatalf("expected Beta in symbol index, symbols=%v err=%v", symbols, err)
	}
	if err := runtime.close(); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}
	if err := vectorStore.Close(); err != nil {
		t.Fatalf("close first vector store: %v", err)
	}

	cfg, err = config.Load(projectRoot)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	emb.embedCalls = 0
	emb.embedBatchCalls = 0
	vectorStore = store.NewGOBStore(config.GetIndexPath(projectRoot))
	if err := vectorStore.Load(ctx); err != nil {
		t.Fatalf("reload vector store: %v", err)
	}
	runtime, err = newProjectIndexRuntime(ctx, projectRoot, cfg, emb, vectorStore, nil)
	if err != nil {
		t.Fatalf("new unchanged runtime: %v", err)
	}
	if err := runtime.runInitialIndex(ctx, true, nil, nil, nil, nil); err != nil {
		t.Fatalf("unchanged index: %v", err)
	}
	if emb.embedCalls+emb.embedBatchCalls != 0 {
		t.Fatalf("expected unchanged index to skip embedding, got embed=%d batch=%d", emb.embedCalls, emb.embedBatchCalls)
	}
	if err := runtime.close(); err != nil {
		t.Fatalf("close unchanged runtime: %v", err)
	}
	if err := vectorStore.Close(); err != nil {
		t.Fatalf("close unchanged vector store: %v", err)
	}

	cfg, err = config.Load(projectRoot)
	if err != nil {
		t.Fatalf("reload config before changes: %v", err)
	}
	writeTestSource(t, projectRoot, "a.go", "package sample\n\nfunc AlphaChanged() {}\n")
	writeTestSource(t, projectRoot, "c.go", "package sample\n\nfunc Gamma() {}\n")
	newMtime := cfg.Watch.LastIndexTime.Add(2 * time.Second)
	for _, name := range []string{"a.go", "c.go"} {
		path := filepath.Join(projectRoot, name)
		if err := os.Chtimes(path, newMtime, newMtime); err != nil {
			t.Fatalf("set mtime for %s: %v", name, err)
		}
	}
	if err := os.Remove(filepath.Join(projectRoot, "b.go")); err != nil {
		t.Fatalf("remove b.go: %v", err)
	}

	emb.embedCalls = 0
	emb.embedBatchCalls = 0
	vectorStore = store.NewGOBStore(config.GetIndexPath(projectRoot))
	if err := vectorStore.Load(ctx); err != nil {
		t.Fatalf("load changed vector store: %v", err)
	}
	runtime, err = newProjectIndexRuntime(ctx, projectRoot, cfg, emb, vectorStore, nil)
	if err != nil {
		t.Fatalf("new changed runtime: %v", err)
	}
	defer runtime.close()
	defer vectorStore.Close()
	if err := runtime.runInitialIndex(ctx, true, nil, nil, nil, nil); err != nil {
		t.Fatalf("changed index: %v", err)
	}
	if emb.embedCalls+emb.embedBatchCalls == 0 {
		t.Fatal("expected changed and new files to call the embedder")
	}
	for _, path := range []string{"a.go", "c.go"} {
		if doc, err := vectorStore.GetDocument(ctx, path); err != nil || doc == nil {
			t.Fatalf("expected %s in vector index, doc=%v err=%v", path, doc, err)
		}
	}
	if doc, err := vectorStore.GetDocument(ctx, "b.go"); err != nil || doc != nil {
		t.Fatalf("expected b.go removed from vector index, doc=%v err=%v", doc, err)
	}
	if symbols, err := runtime.symbolStore.LookupSymbol(ctx, "AlphaChanged"); err != nil || len(symbols) == 0 {
		t.Fatalf("expected AlphaChanged in symbol index, symbols=%v err=%v", symbols, err)
	}
	if symbols, err := runtime.symbolStore.LookupSymbol(ctx, "Beta"); err != nil || len(symbols) != 0 {
		t.Fatalf("expected Beta removed from symbol index, symbols=%v err=%v", symbols, err)
	}
}

func TestProjectIndexRuntimeBuildsRPG(t *testing.T) {
	ctx := t.Context()
	projectRoot := t.TempDir()
	writeTestSource(t, projectRoot, "main.go", "package sample\n\nfunc MainFeature() {}\n")
	cfg := config.DefaultConfig()
	cfg.RPG.Enabled = true
	cfg.RPG.FeatureMode = "local"
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatalf("save config: %v", err)
	}
	vectorStore := store.NewGOBStore(config.GetIndexPath(projectRoot))
	if err := vectorStore.Load(ctx); err != nil {
		t.Fatalf("load vector store: %v", err)
	}
	runtime, err := newProjectIndexRuntime(ctx, projectRoot, cfg, &noOpEmbedder{}, vectorStore, nil)
	if err != nil {
		t.Fatalf("newProjectIndexRuntime: %v", err)
	}
	if err := runtime.runInitialIndex(ctx, true, nil, nil, nil, nil); err != nil {
		t.Fatalf("index with RPG: %v", err)
	}
	if runtime.rpgEncoder == nil || runtime.rpgStore == nil {
		t.Fatal("expected RPG runtime to be initialized")
	}
	stats, err := runtime.rpgStore.GetStats(ctx)
	if err != nil {
		t.Fatalf("get RPG stats: %v", err)
	}
	if stats.TotalNodes == 0 {
		t.Fatal("expected RPG graph nodes")
	}
	if err := runtime.close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if err := vectorStore.Close(); err != nil {
		t.Fatalf("close vector store: %v", err)
	}
	if _, err := os.Stat(config.GetRPGIndexPath(projectRoot)); err != nil {
		t.Fatalf("expected persisted RPG index: %v", err)
	}
}

func TestIndexWorkspaceProjectsContinuesAndAggregatesErrors(t *testing.T) {
	ctx := t.Context()
	goodRoot := t.TempDir()
	writeTestSource(t, goodRoot, "main.go", "package sample\n\nfunc WorkspaceFeature() {}\n")
	missingRoot := filepath.Join(t.TempDir(), "missing")
	ws := &config.Workspace{
		Name: "team",
		Projects: []config.ProjectEntry{
			{Name: "missing", Path: missingRoot},
			{Name: "good", Path: goodRoot},
		},
	}
	sharedStore := store.NewGOBStore(filepath.Join(t.TempDir(), "workspace.gob"))
	if err := sharedStore.Load(ctx); err != nil {
		t.Fatalf("load shared store: %v", err)
	}
	defer sharedStore.Close()

	err := indexWorkspaceProjects(ctx, ws, &noOpEmbedder{}, sharedStore, newWorkspaceProjectIndexRuntime)
	if err == nil {
		t.Fatal("expected aggregate workspace error")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "1 project error") {
		t.Fatalf("unexpected aggregate error: %v", err)
	}
	doc, getErr := sharedStore.GetDocument(ctx, "team/good/main.go")
	if getErr != nil || doc == nil {
		t.Fatalf("expected valid project to be indexed with workspace prefix, doc=%v err=%v", doc, getErr)
	}
}

type persistErrorStore struct {
	store.VectorStore
	err error
}

func (s *persistErrorStore) Persist(context.Context) error {
	return s.err
}

func TestPersistInitialIndexReturnsStoreError(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.DefaultConfig()
	baseStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := baseStore.Load(t.Context()); err != nil {
		t.Fatalf("load vector store: %v", err)
	}
	wantErr := errors.New("persist failed")
	runtime, err := newProjectIndexRuntime(t.Context(), projectRoot, cfg, &noOpEmbedder{}, &persistErrorStore{
		VectorStore: baseStore,
		err:         wantErr,
	}, nil)
	if err != nil {
		t.Fatalf("newProjectIndexRuntime: %v", err)
	}
	defer runtime.close()

	err = runtime.persistInitialIndex(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected persistence error, got %v", err)
	}
}

func writeTestSource(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
