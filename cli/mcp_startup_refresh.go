package cli

import (
	"context"

	"github.com/yoanbernabeu/grepai/config"
)

var (
	mcpFindWorkspaceProjectForPathRunner = config.FindWorkspaceProjectForPath
	mcpValidateWorkspaceBackendRunner    = config.ValidateWorkspaceBackend
	mcpInitializeEmbedderRunner          = initializeEmbedder
	mcpInitializeWorkspaceStoreRunner    = initializeWorkspaceStore
	mcpStartupRefreshRunner              = runWorkspaceProjectStartupRefresh
)

func refreshMCPStartup(ctx context.Context, projectRoot, workspaceName string) error {
	if workspaceName == "" || projectRoot == "" {
		return nil
	}

	resolvedWorkspaceName, ws, project, err := mcpFindWorkspaceProjectForPathRunner(projectRoot)
	if err != nil {
		return err
	}
	if ws == nil || project == nil || resolvedWorkspaceName != workspaceName {
		return nil
	}
	if err := mcpValidateWorkspaceBackendRunner(ws); err != nil {
		return err
	}

	emb, err := mcpInitializeEmbedderRunner(ctx, &config.Config{Embedder: ws.Embedder})
	if err != nil {
		return err
	}
	defer emb.Close()

	sharedStore, err := mcpInitializeWorkspaceStoreRunner(ctx, ws)
	if err != nil {
		return err
	}
	defer sharedStore.Close()

	return mcpStartupRefreshRunner(ctx, ws, *project, emb, sharedStore, true)
}
