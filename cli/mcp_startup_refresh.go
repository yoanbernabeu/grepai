package cli

import (
	"context"

	"github.com/yoanbernabeu/grepai/config"
)

func refreshMCPStartup(ctx context.Context, projectRoot, workspaceName string) error {
	if workspaceName == "" || projectRoot == "" {
		return nil
	}

	resolvedWorkspaceName, ws, project, err := config.FindWorkspaceProjectForPath(projectRoot)
	if err != nil {
		return err
	}
	if ws == nil || project == nil || resolvedWorkspaceName != workspaceName {
		return nil
	}
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	emb, err := initializeEmbedder(ctx, &config.Config{Embedder: ws.Embedder})
	if err != nil {
		return err
	}
	defer emb.Close()

	sharedStore, err := initializeWorkspaceStore(ctx, ws)
	if err != nil {
		return err
	}
	defer sharedStore.Close()

	return runWorkspaceProjectStartupRefresh(ctx, ws, *project, emb, sharedStore, true)
}
