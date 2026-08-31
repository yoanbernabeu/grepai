package trace

import (
	"context"
	"fmt"

	"github.com/yoanbernabeu/grepai/config"
)

// WorkspaceProjects resolves which projects a workspace-scoped command
// should cover: every project in the workspace, or just projectName when
// non-empty. Callers that need the project roots (to re-extract symbols,
// for instance) use this directly; LoadWorkspaceSymbolStores builds on it.
func WorkspaceProjects(workspaceName, projectName string) ([]config.ProjectEntry, error) {
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return nil, fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(workspaceName)
	if err != nil {
		return nil, err
	}

	if projectName == "" {
		return ws.Projects, nil
	}
	for _, p := range ws.Projects {
		if p.Name == projectName {
			return []config.ProjectEntry{p}, nil
		}
	}
	return nil, fmt.Errorf("project %q not found in workspace %q", projectName, workspaceName)
}

// LoadWorkspaceSymbolStores opens the persisted symbol index of every
// project the workspace scope covers.
func LoadWorkspaceSymbolStores(ctx context.Context, workspaceName, projectName string) ([]SymbolStore, error) {
	projects, err := WorkspaceProjects(workspaceName, projectName)
	if err != nil {
		return nil, err
	}

	stores := make([]SymbolStore, 0, len(projects))
	for _, p := range projects {
		ss := NewGOBSymbolStore(config.GetSymbolIndexPath(p.Path))
		if err := ss.Load(ctx); err != nil {
			ss.Close()
			CloseSymbolStores(stores)
			return nil, fmt.Errorf("failed to load symbol index for project %s: %w", p.Name, err)
		}
		stores = append(stores, ss)
	}
	return stores, nil
}

// CloseSymbolStores closes all symbol stores in the slice.
func CloseSymbolStores(stores []SymbolStore) {
	for _, s := range stores {
		s.Close()
	}
}
