package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

// TestServerCreateEmbedder_AppliesConfiguredDimensions verifies that createEmbedder
// passes configured dimension into each embedder constructor.
func TestServerCreateEmbedder_AppliesConfiguredDimensions(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		dimensions int
		apiKey     string
	}{
		{name: "ollama", provider: "ollama", dimensions: 768},
		{name: "lmstudio", provider: "lmstudio", dimensions: 768},
		{name: "openai-1536", provider: "openai", dimensions: 1536, apiKey: "sk-test"},
		{name: "openai-3072", provider: "openai", dimensions: 3072, apiKey: "sk-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			cfg := config.DefaultConfig()
			cfg.Embedder.Provider = tt.provider
			cfg.Embedder.Dimensions = &tt.dimensions
			cfg.Embedder.APIKey = tt.apiKey

			emb, err := s.createEmbedder(cfg)
			if err != nil {
				t.Fatalf("createEmbedder returned error: %v", err)
			}

			if emb.Dimensions() != tt.dimensions {
				t.Fatalf("expected dimensions %d, got %d", tt.dimensions, emb.Dimensions())
			}
		})
	}
}

// TestCompactStructDefinitions verifies compact struct definitions.
func TestCompactStructDefinitions(t *testing.T) {
	t.Run("SearchResultCompact has no Content field", func(t *testing.T) {
		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "content") {
			t.Errorf("Compact struct should not contain 'content' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file_path") {
			t.Errorf("Compact struct should contain 'file_path' field, got: %s", jsonStr)
		}
	})

	t.Run("CallSiteCompact has no Context field", func(t *testing.T) {
		compact := CallSiteCompact{
			File: "test.go",
			Line: 10,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "context") {
			t.Errorf("Compact struct should not contain 'context' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file") {
			t.Errorf("Compact struct should contain 'file' field, got: %s", jsonStr)
		}
	})
}

// TestCompactStructMarshaling verifies JSON marshaling of compact structs.
func TestCompactStructMarshaling(t *testing.T) {
	t.Run("SearchResult vs SearchResultCompact", func(t *testing.T) {
		full := SearchResult{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
			Content:   "line 1\nline 2\nline 3",
		}

		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		fullJSON, err := json.Marshal(full)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		compactJSON, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if len(compactJSON) >= len(fullJSON) {
			t.Errorf("Compact JSON should be shorter than full JSON, got compact=%d, full=%d", len(compactJSON), len(fullJSON))
		}

		if !strings.Contains(string(fullJSON), "content") {
			t.Errorf("Full JSON should contain 'content' field")
		}

		if strings.Contains(string(compactJSON), "content") {
			t.Errorf("Compact JSON should not contain 'content' field")
		}
	})
}

// TestNonCompactSearchResult verifies that the full SearchResult struct
// includes all expected fields when NOT in compact mode.
func TestNonCompactSearchResult(t *testing.T) {
	result := SearchResult{
		FilePath:  "example/test.go",
		StartLine: 42,
		EndLine:   50,
		Score:     0.87,
		Content:   "func example() {\n\treturn true\n}",
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify all required fields are present
	expectedFields := []string{
		"file_path",
		"start_line",
		"end_line",
		"score",
		"content",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Non-compact JSON should contain '%s' field, got: %s", field, jsonStr)
		}
	}

	// Verify content field has correct value
	if !strings.Contains(jsonStr, "func example()") {
		t.Errorf("Non-compact JSON should contain full content, got: %s", jsonStr)
	}

	// Verify score is present and non-zero
	if !strings.Contains(jsonStr, "0.87") {
		t.Errorf("Non-compact JSON should contain score value, got: %s", jsonStr)
	}
}

// TestServerCreateStore_GOBBackend tests createStore with gob backend
func TestServerCreateStore_GOBBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "gob"

	ctx := context.Background()
	store, err := s.createStore(ctx, cfg)

	if err != nil {
		t.Fatalf("createStore returned error: %v", err)
	}

	if store == nil {
		t.Error("expected non-nil store")
	}

	_ = store.Close()
}

// TestServerCreateStore_UnknownBackend tests that createStore returns error for unknown backend
func TestServerCreateStore_UnknownBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "unknown-backend"

	ctx := context.Background()
	_, err := s.createStore(ctx, cfg)

	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}

	expected := "unknown storage backend: unknown-backend"
	if err.Error() != expected {
		t.Errorf("expected error message %s, got %s", expected, err.Error())
	}
}

// TestRegisterTools_IndexStatusSchema verifies that the grepai_index_status tool
// is registered with a non-empty schema (regression for empty schema error).
func TestRegisterTools_IndexStatusSchema(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	// initialize minimal MCP server like NewServer would
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")

	s.registerTools()

	tools := s.mcpServer.ListTools()
	indexStatus, ok := tools["grepai_index_status"]
	if !ok {
		t.Fatalf("grepai_index_status tool not registered")
	}

	schema := indexStatus.Tool.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %q", schema.Type)
	}

	prop, ok := schema.Properties["verbose"]
	if !ok {
		t.Fatalf("expected verbose property in schema")
	}

	propMap, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("verbose property is not an object, got %T", prop)
	}

	if propMap["type"] != "boolean" {
		t.Fatalf("expected verbose type boolean, got %v", propMap["type"])
	}
}

func TestNewServerWithWorkspace(t *testing.T) {
	t.Run("creates_server_with_workspace", func(t *testing.T) {
		srv, err := NewServerWithWorkspace("", "test")
		if err != nil {
			t.Fatalf("NewServerWithWorkspace error: %v", err)
		}
		if srv.workspaceName != "test" {
			t.Errorf("expected workspace test, got %s", srv.workspaceName)
		}
		if srv.projectRoot != "" {
			t.Errorf("expected empty projectRoot, got %s", srv.projectRoot)
		}
	})

	t.Run("creates_server_with_project_and_workspace", func(t *testing.T) {
		srv, err := NewServerWithWorkspace("/tmp/project", "test")
		if err != nil {
			t.Fatalf("NewServerWithWorkspace error: %v", err)
		}
		if srv.workspaceName != "test" {
			t.Errorf("expected workspace test, got %s", srv.workspaceName)
		}
		if srv.projectRoot != "/tmp/project" {
			t.Errorf("expected project /tmp/project, got %s", srv.projectRoot)
		}
	})
}

// TestResolveWorkspace_should_return_explicit_workspace_when_provided verifies that
// resolveWorkspace returns the explicit workspace parameter when it is non-empty.
func TestResolveWorkspace_should_return_explicit_workspace_when_provided(t *testing.T) {
	s := &Server{workspaceName: "default"}

	got := s.resolveWorkspace("explicit")
	if got != "explicit" {
		t.Errorf("expected 'explicit', got %q", got)
	}
}

// TestResolveWorkspace_should_fallback_to_server_workspace_when_empty verifies that
// resolveWorkspace falls back to the server's workspaceName when the parameter is empty.
func TestResolveWorkspace_should_fallback_to_server_workspace_when_empty(t *testing.T) {
	s := &Server{workspaceName: "default"}

	got := s.resolveWorkspace("")
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

// TestResolveWorkspace_should_return_empty_when_both_empty verifies that
// resolveWorkspace returns empty string when both parameter and server workspace are empty.
func TestResolveWorkspace_should_return_empty_when_both_empty(t *testing.T) {
	s := &Server{workspaceName: ""}

	got := s.resolveWorkspace("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// helperGetToolSchemaProperties returns the properties map for a registered tool.
func helperGetToolSchemaProperties(t *testing.T, toolName string) map[string]any {
	t.Helper()

	s := &Server{projectRoot: "/tmp/test-project"}
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")
	s.registerTools()

	tools := s.mcpServer.ListTools()
	tool, ok := tools[toolName]
	if !ok {
		t.Fatalf("tool %q not registered", toolName)
	}

	schema := tool.Tool.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %q", schema.Type)
	}

	props := make(map[string]any, len(schema.Properties))
	for k, v := range schema.Properties {
		props[k] = v
	}
	return props
}

// TestRegisterTools_should_include_workspace_param_on_trace_callers verifies that
// grepai_trace_callers has workspace and project properties in its schema.
func TestRegisterTools_should_include_workspace_param_on_trace_callers(t *testing.T) {
	props := helperGetToolSchemaProperties(t, "grepai_trace_callers")

	if _, ok := props["workspace"]; !ok {
		t.Error("expected 'workspace' property in grepai_trace_callers schema")
	}
	if _, ok := props["project"]; !ok {
		t.Error("expected 'project' property in grepai_trace_callers schema")
	}
}

// TestRegisterTools_should_include_workspace_param_on_trace_callees verifies that
// grepai_trace_callees has workspace and project properties in its schema.
func TestRegisterTools_should_include_workspace_param_on_trace_callees(t *testing.T) {
	props := helperGetToolSchemaProperties(t, "grepai_trace_callees")

	if _, ok := props["workspace"]; !ok {
		t.Error("expected 'workspace' property in grepai_trace_callees schema")
	}
	if _, ok := props["project"]; !ok {
		t.Error("expected 'project' property in grepai_trace_callees schema")
	}
}

// TestRegisterTools_should_include_workspace_param_on_trace_graph verifies that
// grepai_trace_graph has workspace and project properties in its schema.
func TestRegisterTools_should_include_workspace_param_on_trace_graph(t *testing.T) {
	props := helperGetToolSchemaProperties(t, "grepai_trace_graph")

	if _, ok := props["workspace"]; !ok {
		t.Error("expected 'workspace' property in grepai_trace_graph schema")
	}
	if _, ok := props["project"]; !ok {
		t.Error("expected 'project' property in grepai_trace_graph schema")
	}
}

// TestRegisterTools_should_include_workspace_param_on_index_status verifies that
// grepai_index_status has a workspace property in its schema.
func TestRegisterTools_should_include_workspace_param_on_index_status(t *testing.T) {
	props := helperGetToolSchemaProperties(t, "grepai_index_status")

	if _, ok := props["workspace"]; !ok {
		t.Error("expected 'workspace' property in grepai_index_status schema")
	}
}

// TestWorkspaceIndexStatus_should_marshal_correctly verifies that WorkspaceIndexStatus
// marshals to JSON with the expected fields.
func TestWorkspaceIndexStatus_should_marshal_correctly(t *testing.T) {
	status := WorkspaceIndexStatus{
		Workspace: "my-workspace",
		Projects: []WorkspaceProjectStatus{
			{Name: "project-a", Path: "/home/user/project-a", SymbolsReady: true, TotalSymbols: 42},
			{Name: "project-b", Path: "/home/user/project-b", SymbolsReady: false, TotalSymbols: 0},
		},
		Provider: "ollama",
		Model:    "nomic-embed-text",
	}

	jsonBytes, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	expectedFields := []string{
		`"workspace"`,
		`"projects"`,
		`"provider"`,
		`"model"`,
		`"my-workspace"`,
		`"project-a"`,
		`"project-b"`,
		`"symbols_ready"`,
		`"total_symbols"`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("expected JSON to contain %s, got: %s", field, jsonStr)
		}
	}

	// Verify round-trip unmarshaling
	var decoded WorkspaceIndexStatus
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Workspace != "my-workspace" {
		t.Errorf("expected workspace 'my-workspace', got %q", decoded.Workspace)
	}
	if len(decoded.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(decoded.Projects))
	}
	if decoded.Projects[0].TotalSymbols != 42 {
		t.Errorf("expected project-a total_symbols=42, got %d", decoded.Projects[0].TotalSymbols)
	}
	if decoded.Projects[1].SymbolsReady {
		t.Error("expected project-b symbols_ready=false")
	}
}

// TestHandleTraceCallersFromStores_should_aggregate_across_stores verifies that
// handleTraceCallersFromStores aggregates callers from multiple symbol stores.
func TestHandleTraceCallersFromStores_should_aggregate_across_stores(t *testing.T) {
	ctx := context.Background()
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	store1 := trace.NewGOBSymbolStore(filepath.Join(tmpDir1, "symbols.gob"))
	store2 := trace.NewGOBSymbolStore(filepath.Join(tmpDir2, "symbols.gob"))

	// Save a symbol "Login" to store1 with a caller "HandleAuth"
	err := store1.SaveFile(ctx, "/project1/auth.go", []trace.Symbol{
		{Name: "Login", Kind: "function", File: "/project1/auth.go", Line: 10, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "/project1/handler.go", Line: 20, CallerName: "HandleAuth", CallerFile: "/project1/handler.go", CallerLine: 15},
	})
	if err != nil {
		t.Fatalf("store1.SaveFile failed: %v", err)
	}

	// Save a symbol "Login" to store2 with a caller "ProcessAuth"
	err = store2.SaveFile(ctx, "/project2/api.go", []trace.Symbol{
		{Name: "Login", Kind: "function", File: "/project2/api.go", Line: 5, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "/project2/service.go", Line: 30, CallerName: "ProcessAuth", CallerFile: "/project2/service.go", CallerLine: 25},
	})
	if err != nil {
		t.Fatalf("store2.SaveFile failed: %v", err)
	}

	s := &Server{}
	stores := []trace.SymbolStore{store1, store2}

	result, err := s.handleTraceCallersFromStores(ctx, "Login", false, "json", stores)
	if err != nil {
		t.Fatalf("handleTraceCallersFromStores returned error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	resultJSON, _ := json.Marshal(result)
	text := string(resultJSON)

	if !strings.Contains(text, "HandleAuth") {
		t.Errorf("expected result to contain caller 'HandleAuth', got: %s", text)
	}
	if !strings.Contains(text, "ProcessAuth") {
		t.Errorf("expected result to contain caller 'ProcessAuth', got: %s", text)
	}
}

// TestHandleTraceCalleesFromStores_should_aggregate_across_stores verifies that
// handleTraceCalleesFromStores aggregates callees from multiple symbol stores.
func TestHandleTraceCalleesFromStores_should_aggregate_across_stores(t *testing.T) {
	ctx := context.Background()
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	store1 := trace.NewGOBSymbolStore(filepath.Join(tmpDir1, "symbols.gob"))
	store2 := trace.NewGOBSymbolStore(filepath.Join(tmpDir2, "symbols.gob"))

	// In store1, "HandleRequest" calls "ValidateInput"
	err := store1.SaveFile(ctx, "/project1/handler.go", []trace.Symbol{
		{Name: "HandleRequest", Kind: "function", File: "/project1/handler.go", Line: 10, Language: "go"},
		{Name: "ValidateInput", Kind: "function", File: "/project1/handler.go", Line: 30, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "ValidateInput", File: "/project1/handler.go", Line: 15, CallerName: "HandleRequest", CallerFile: "/project1/handler.go", CallerLine: 10},
	})
	if err != nil {
		t.Fatalf("store1.SaveFile failed: %v", err)
	}

	// In store2, "HandleRequest" calls "SendResponse"
	err = store2.SaveFile(ctx, "/project2/handler.go", []trace.Symbol{
		{Name: "HandleRequest", Kind: "function", File: "/project2/handler.go", Line: 5, Language: "go"},
		{Name: "SendResponse", Kind: "function", File: "/project2/handler.go", Line: 25, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "SendResponse", File: "/project2/handler.go", Line: 12, CallerName: "HandleRequest", CallerFile: "/project2/handler.go", CallerLine: 5},
	})
	if err != nil {
		t.Fatalf("store2.SaveFile failed: %v", err)
	}

	s := &Server{}
	stores := []trace.SymbolStore{store1, store2}

	result, err := s.handleTraceCalleesFromStores(ctx, "HandleRequest", false, "json", stores)
	if err != nil {
		t.Fatalf("handleTraceCalleesFromStores returned error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	resultJSON, _ := json.Marshal(result)
	text := string(resultJSON)

	if !strings.Contains(text, "ValidateInput") {
		t.Errorf("expected result to contain callee 'ValidateInput', got: %s", text)
	}
	if !strings.Contains(text, "SendResponse") {
		t.Errorf("expected result to contain callee 'SendResponse', got: %s", text)
	}
}
