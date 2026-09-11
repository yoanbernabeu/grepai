package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestIndexStatusHandlerProjectReportsSymbolReadiness(t *testing.T) {
	// Given a configured project with a real GOB symbol index.
	root := t.TempDir()
	cfg := config.DefaultConfig()
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	seedTraceHandlerProject(t, root, "project")
	server := &Server{projectRoot: root}

	// When the actual status handler runs.
	result, err := server.handleIndexStatus(context.Background(), traceHandlerRequest(map[string]any{"format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then it observes the loaded symbol store before closing it.
	var status IndexStatus
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &status); err != nil {
		t.Fatal(err)
	}
	if !status.SymbolsReady || status.Provider != cfg.Embedder.Provider {
		t.Fatalf("status = %#v", status)
	}
}

func TestIndexStatusHandlerWorkspaceReportsEachProject(t *testing.T) {
	// Given an isolated workspace with two configured symbol projects.
	home := isolateMCPTestHome(t)
	projects := make([]config.ProjectEntry, 0, 2)
	for _, name := range []string{"one", "two"} {
		root := filepath.Join(home, name)
		cfg := config.DefaultConfig()
		if err := cfg.Save(root); err != nil {
			t.Fatal(err)
		}
		seedTraceHandlerProject(t, root, name)
		projects = append(projects, config.ProjectEntry{Name: name, Path: root})
	}
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{Name: "status", Projects: projects})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}
	server := &Server{workspaceName: "status"}

	// When workspace status runs through the registered handler implementation.
	result, err := server.handleIndexStatus(context.Background(), traceHandlerRequest(map[string]any{"format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then both projects report their independently loaded symbol counts.
	var status WorkspaceIndexStatus
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 2 || !status.Projects[0].SymbolsReady || !status.Projects[1].SymbolsReady {
		t.Fatalf("workspace status = %#v", status)
	}
}

func TestIndexStatusHandlerWorkspaceDefaultsMissingProjectConfig(t *testing.T) {
	// Given a workspace project whose populated GOB symbol index predates any
	// local config, alongside a project with neither config nor index.
	home := isolateMCPTestHome(t)
	legacy := filepath.Join(home, "legacy")
	seedTraceHandlerProject(t, legacy, "legacy")
	empty := filepath.Join(home, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{Name: "missing-config", Projects: []config.ProjectEntry{
		{Name: "legacy", Path: legacy},
		{Name: "empty", Path: empty},
	}})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}
	server := &Server{workspaceName: "missing-config"}

	// When workspace status runs through the actual handler.
	result, err := server.handleIndexStatus(context.Background(), traceHandlerRequest(map[string]any{"format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then the config-less populated index reports ready with its real count,
	// while the empty project stays not ready.
	var status WorkspaceIndexStatus
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 2 {
		t.Fatalf("workspace status = %#v", status)
	}
	if got := status.Projects[0]; !got.SymbolsReady || got.TotalSymbols != 3 {
		t.Fatalf("config-less populated project status = %#v", got)
	}
	if got := status.Projects[1]; got.SymbolsReady || got.TotalSymbols != 0 {
		t.Fatalf("config-less empty project status = %#v", got)
	}

	// And no config files were created as a side effect.
	if config.Exists(legacy) || config.Exists(empty) {
		t.Fatalf("status handler created config files: legacy=%v empty=%v", config.Exists(legacy), config.Exists(empty))
	}
}

func TestIndexStatusHandlerWorkspaceMalformedProjectConfigStaysNotReady(t *testing.T) {
	// Given a workspace project with a populated index but a malformed config.
	home := isolateMCPTestHome(t)
	root := filepath.Join(home, "broken")
	seedTraceHandlerProject(t, root, "broken")
	if err := os.WriteFile(config.GetConfigPath(root), []byte("store: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{Name: "malformed", Projects: []config.ProjectEntry{
		{Name: "broken", Path: root},
	}})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}
	server := &Server{workspaceName: "malformed"}

	// When workspace status runs through the actual handler.
	result, err := server.handleIndexStatus(context.Background(), traceHandlerRequest(map[string]any{"format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then the real config failure is preserved as not ready, not defaulted away.
	var status WorkspaceIndexStatus
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 1 || status.Projects[0].SymbolsReady || status.Projects[0].TotalSymbols != 0 {
		t.Fatalf("malformed-config project status = %#v", status.Projects)
	}
}
