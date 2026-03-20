package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

func TestRunWorkspaceProjectStartupRefresh_SkipsUnchangedFiles(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to seed project file: %v", err)
	}

	ws := &config.Workspace{Name: "ws"}
	project := config.ProjectEntry{Name: "proj", Path: projectRoot}
	sharedStore := store.NewGOBStore(filepath.Join(projectRoot, "workspace-index.gob"))

	firstEmbedder := &countingEmbedder{}
	if err := runWorkspaceProjectStartupRefresh(ctx, ws, project, firstEmbedder, sharedStore, true); err != nil {
		t.Fatalf("first startup refresh failed: %v", err)
	}
	if firstEmbedder.embedCalls == 0 && firstEmbedder.embedBatchCalls == 0 {
		t.Fatal("expected first startup refresh to index at least one file")
	}

	secondEmbedder := &countingEmbedder{}
	if err := runWorkspaceProjectStartupRefresh(ctx, ws, project, secondEmbedder, sharedStore, true); err != nil {
		t.Fatalf("second startup refresh failed: %v", err)
	}
	if secondEmbedder.embedCalls != 0 || secondEmbedder.embedBatchCalls != 0 {
		t.Fatalf("expected unchanged second startup refresh to skip embedding, got embed=%d embedBatch=%d", secondEmbedder.embedCalls, secondEmbedder.embedBatchCalls)
	}
}
