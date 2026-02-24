package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/yoanbernabeu/grepai/config"
)

// TestDiscoveryToolsCompile verifies that the discovery tools are properly integrated
// and can be called without errors. This is a smoke test to ensure the handlers
// are callable and the MCP server is properly configured.
func TestDiscoveryToolsCompile(t *testing.T) {
	// This test primarily checks that the code compiles and the Server type
	// has the new handler methods.

	server := &Server{
		projectRoot:   "/tmp/test",
		workspaceName: "test",
	}

	// Verify the methods are accessible on Server.
	_ = server.handleListWorkspaces
	_ = server.handleListProjects

	// The handlers are defined and callable, which verifies:
	// 1. The code compiles without errors
	// 2. The methods are properly attached to the Server type
	// 3. The signatures match what MCP expects
}

// TestEncodeOutput verifies the output formatting for both JSON and TOON formats
func TestEncodeOutput(t *testing.T) {
	data := map[string]interface{}{
		"name":   "test",
		"value":  42,
		"active": true,
	}

	tests := []struct {
		name      string
		format    string
		expectErr bool
	}{
		{
			name:      "json format",
			format:    "json",
			expectErr: false,
		},
		{
			name:      "toon format",
			format:    "toon",
			expectErr: false,
		},
		{
			name:      "default format (json)",
			format:    "",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := encodeOutput(data, tt.format)
			if (err != nil) != tt.expectErr {
				t.Errorf("encodeOutput() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if len(output) == 0 {
				t.Error("encodeOutput() returned empty string")
			}
		})
	}
}

func TestHandleListWorkspaces_OnlyReturnsWorkspaceInfo(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	cfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"alpha": {
				Name: "alpha",
				Projects: []config.ProjectEntry{
					{Name: "api", Path: "/tmp/alpha-api"},
				},
			},
			"beta": {
				Name: "beta",
				Projects: []config.ProjectEntry{
					{Name: "web", Path: "/tmp/beta-web"},
				},
			},
		},
	}
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	s := &Server{}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"format": "json"},
		},
	}

	result, err := s.handleListWorkspaces(context.Background(), req)
	if err != nil {
		t.Fatalf("handleListWorkspaces returned error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("handleListWorkspaces returned no content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	var workspaces []map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &workspaces); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}

	for _, ws := range workspaces {
		if _, ok := ws["name"]; !ok {
			t.Fatalf("workspace entry missing name: %#v", ws)
		}
		if _, hasProjects := ws["projects"]; hasProjects {
			t.Fatalf("workspace entry should not include projects: %#v", ws)
		}
	}
}

func TestHandleListProjects_DefaultsToServerWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	cfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"tymemud": {
				Name: "tymemud",
				Projects: []config.ProjectEntry{
					{Name: "api", Path: "/tmp/tymemud-api"},
					{Name: "web", Path: "/tmp/tymemud-web"},
				},
			},
		},
	}
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	s := &Server{workspaceName: "tymemud"}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"format": "json"},
		},
	}

	result, err := s.handleListProjects(context.Background(), req)
	if err != nil {
		t.Fatalf("handleListProjects returned error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("handleListProjects returned no content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	var projects []map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &projects); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	projectNames := map[string]bool{}
	for _, p := range projects {
		name, hasName := p["name"].(string)
		path, hasPath := p["path"].(string)
		if !hasName || !hasPath || name == "" || path == "" {
			t.Fatalf("invalid project entry: %#v", p)
		}
		projectNames[name] = true
	}

	if !projectNames["api"] || !projectNames["web"] {
		t.Fatalf("expected projects api and web, got %#v", projectNames)
	}
}

func TestHandleSearch_NoWorkspaceHint_WhenWorkspacesConfigured(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	cfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"tymemud": {
				Name: "tymemud",
			},
		},
	}
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	s := &Server{}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"query": "hello"},
		},
	}

	result, err := s.handleSearch(context.Background(), req)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("handleSearch returned no content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	if !strings.Contains(textContent.Text, "no workspace was provided") {
		t.Fatalf("expected workspace guidance in error, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "provide the workspace parameter") {
		t.Fatalf("expected workspace parameter guidance in error, got: %s", textContent.Text)
	}
}

func TestHandleSearch_NoWorkspaceHint_WhenNoWorkspacesConfigured(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	s := &Server{}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"query": "hello"},
		},
	}

	result, err := s.handleSearch(context.Background(), req)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("handleSearch returned no content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	if !strings.Contains(textContent.Text, "failed to read config file") {
		t.Fatalf("expected fallback config error, got: %s", textContent.Text)
	}
	if strings.Contains(textContent.Text, "no workspace was provided") {
		t.Fatalf("workspace hint should not appear when no workspaces are configured: %s", textContent.Text)
	}
}
