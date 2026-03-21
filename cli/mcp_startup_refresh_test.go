package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

func TestRefreshMCPStartup_NoopWithoutRequiredContext(t *testing.T) {
	oldFind := mcpFindWorkspaceProjectForPathRunner
	oldValidate := mcpValidateWorkspaceBackendRunner
	oldInitEmbedder := mcpInitializeEmbedderRunner
	oldInitStore := mcpInitializeWorkspaceStoreRunner
	oldRefresh := mcpStartupRefreshRunner
	defer func() {
		mcpFindWorkspaceProjectForPathRunner = oldFind
		mcpValidateWorkspaceBackendRunner = oldValidate
		mcpInitializeEmbedderRunner = oldInitEmbedder
		mcpInitializeWorkspaceStoreRunner = oldInitStore
		mcpStartupRefreshRunner = oldRefresh
	}()

	mcpFindWorkspaceProjectForPathRunner = func(path string) (string, *config.Workspace, *config.ProjectEntry, error) {
		t.Fatalf("unexpected workspace lookup for %q", path)
		return "", nil, nil, nil
	}
	mcpValidateWorkspaceBackendRunner = func(ws *config.Workspace) error {
		t.Fatalf("unexpected workspace validation for %v", ws)
		return nil
	}
	mcpInitializeEmbedderRunner = func(ctx context.Context, cfg *config.Config) (embedder.Embedder, error) {
		t.Fatalf("unexpected embedder init for %+v", cfg)
		return nil, nil
	}
	mcpInitializeWorkspaceStoreRunner = func(ctx context.Context, ws *config.Workspace) (store.VectorStore, error) {
		t.Fatalf("unexpected workspace store init for %v", ws)
		return nil, nil
	}
	mcpStartupRefreshRunner = func(ctx context.Context, ws *config.Workspace, project config.ProjectEntry, emb embedder.Embedder, sharedStore store.VectorStore, isBackgroundChild bool) error {
		t.Fatalf("unexpected refresh for workspace=%v project=%v", ws, project)
		return nil
	}

	if err := refreshMCPStartup(context.Background(), "", "test"); err != nil {
		t.Fatalf("workspace-only no-op failed: %v", err)
	}
	if err := refreshMCPStartup(context.Background(), "/tmp/project", ""); err != nil {
		t.Fatalf("project-only no-op failed: %v", err)
	}
}

func TestRefreshMCPStartup_NoopWhenWorkspaceMismatch(t *testing.T) {
	oldFind := mcpFindWorkspaceProjectForPathRunner
	oldValidate := mcpValidateWorkspaceBackendRunner
	oldInitEmbedder := mcpInitializeEmbedderRunner
	oldInitStore := mcpInitializeWorkspaceStoreRunner
	oldRefresh := mcpStartupRefreshRunner
	defer func() {
		mcpFindWorkspaceProjectForPathRunner = oldFind
		mcpValidateWorkspaceBackendRunner = oldValidate
		mcpInitializeEmbedderRunner = oldInitEmbedder
		mcpInitializeWorkspaceStoreRunner = oldInitStore
		mcpStartupRefreshRunner = oldRefresh
	}()

	project := &config.ProjectEntry{Name: "proj", Path: "/tmp/project"}
	ws := &config.Workspace{Name: "other"}
	mcpFindWorkspaceProjectForPathRunner = func(path string) (string, *config.Workspace, *config.ProjectEntry, error) {
		if path != "/tmp/project" {
			t.Fatalf("workspace lookup path = %q, want %q", path, "/tmp/project")
		}
		return "other", ws, project, nil
	}
	mcpValidateWorkspaceBackendRunner = func(ws *config.Workspace) error {
		t.Fatalf("unexpected validation for workspace %q", ws.Name)
		return nil
	}
	mcpInitializeEmbedderRunner = func(ctx context.Context, cfg *config.Config) (embedder.Embedder, error) {
		t.Fatal("unexpected embedder init")
		return nil, nil
	}
	mcpInitializeWorkspaceStoreRunner = func(ctx context.Context, ws *config.Workspace) (store.VectorStore, error) {
		t.Fatal("unexpected workspace store init")
		return nil, nil
	}
	mcpStartupRefreshRunner = func(ctx context.Context, ws *config.Workspace, project config.ProjectEntry, emb embedder.Embedder, sharedStore store.VectorStore, isBackgroundChild bool) error {
		t.Fatal("unexpected startup refresh")
		return nil
	}

	if err := refreshMCPStartup(context.Background(), "/tmp/project", "test"); err != nil {
		t.Fatalf("mismatch no-op failed: %v", err)
	}
}

func TestRefreshMCPStartup_RefreshesMatchingWorkspaceProject(t *testing.T) {
	oldFind := mcpFindWorkspaceProjectForPathRunner
	oldValidate := mcpValidateWorkspaceBackendRunner
	oldInitEmbedder := mcpInitializeEmbedderRunner
	oldInitStore := mcpInitializeWorkspaceStoreRunner
	oldRefresh := mcpStartupRefreshRunner
	defer func() {
		mcpFindWorkspaceProjectForPathRunner = oldFind
		mcpValidateWorkspaceBackendRunner = oldValidate
		mcpInitializeEmbedderRunner = oldInitEmbedder
		mcpInitializeWorkspaceStoreRunner = oldInitStore
		mcpStartupRefreshRunner = oldRefresh
	}()

	project := &config.ProjectEntry{Name: "proj", Path: "/tmp/project"}
	ws := &config.Workspace{Name: "test"}
	emb := &countingEmbedder{}
	sharedStore := &mockVectorStore{}

	mcpFindWorkspaceProjectForPathRunner = func(path string) (string, *config.Workspace, *config.ProjectEntry, error) {
		return "test", ws, project, nil
	}

	validated := false
	mcpValidateWorkspaceBackendRunner = func(workspace *config.Workspace) error {
		validated = true
		if workspace != ws {
			t.Fatalf("validated workspace = %p, want %p", workspace, ws)
		}
		return nil
	}

	mcpInitializeEmbedderRunner = func(ctx context.Context, cfg *config.Config) (embedder.Embedder, error) {
		if cfg.Embedder != ws.Embedder {
			t.Fatalf("embedder config = %+v, want %+v", cfg.Embedder, ws.Embedder)
		}
		return emb, nil
	}
	mcpInitializeWorkspaceStoreRunner = func(ctx context.Context, workspace *config.Workspace) (store.VectorStore, error) {
		if workspace != ws {
			t.Fatalf("workspace store init = %p, want %p", workspace, ws)
		}
		return sharedStore, nil
	}

	refreshCalled := false
	mcpStartupRefreshRunner = func(ctx context.Context, workspace *config.Workspace, gotProject config.ProjectEntry, gotEmb embedder.Embedder, gotStore store.VectorStore, isBackgroundChild bool) error {
		refreshCalled = true
		if workspace != ws {
			t.Fatalf("refresh workspace = %p, want %p", workspace, ws)
		}
		if gotProject != *project {
			t.Fatalf("refresh project = %+v, want %+v", gotProject, *project)
		}
		if gotEmb != emb {
			t.Fatalf("refresh embedder = %T, want %T", gotEmb, emb)
		}
		if gotStore != sharedStore {
			t.Fatalf("refresh store = %T, want %T", gotStore, sharedStore)
		}
		if !isBackgroundChild {
			t.Fatal("expected startup refresh to run with background-child semantics")
		}
		return nil
	}

	if err := refreshMCPStartup(context.Background(), "/tmp/project", "test"); err != nil {
		t.Fatalf("refreshMCPStartup failed: %v", err)
	}
	if !validated {
		t.Fatal("expected workspace backend validation")
	}
	if !refreshCalled {
		t.Fatal("expected startup refresh to run")
	}
}

func TestRefreshMCPStartup_PropagatesLookupError(t *testing.T) {
	oldFind := mcpFindWorkspaceProjectForPathRunner
	defer func() { mcpFindWorkspaceProjectForPathRunner = oldFind }()

	wantErr := errors.New("lookup failed")
	mcpFindWorkspaceProjectForPathRunner = func(path string) (string, *config.Workspace, *config.ProjectEntry, error) {
		return "", nil, nil, wantErr
	}

	if err := refreshMCPStartup(context.Background(), "/tmp/project", "test"); !errors.Is(err, wantErr) {
		t.Fatalf("expected lookup failure %v, got %v", wantErr, err)
	}
}
