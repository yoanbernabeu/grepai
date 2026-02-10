package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func setTestHomeDirCLI(t *testing.T, dir string) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		original := os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", dir)
		return func() { os.Setenv("USERPROFILE", original) }
	}
	original := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	return func() { os.Setenv("HOME", original) }
}

func TestCreateWorkspaceNonInteractive(t *testing.T) {
	t.Run("flags_qdrant_ollama", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-test-cli")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		ws, err := buildWorkspaceFromFlags("test-ws", "qdrant", "ollama", "nomic-embed-text", "", "", "http://localhost", 6334, "", false)
		if err != nil {
			t.Fatalf("buildWorkspaceFromFlags error: %v", err)
		}
		if ws.Name != "test-ws" {
			t.Errorf("expected name test-ws, got %s", ws.Name)
		}
		if ws.Store.Backend != "qdrant" {
			t.Errorf("expected qdrant backend, got %s", ws.Store.Backend)
		}
		if ws.Embedder.Provider != "ollama" {
			t.Errorf("expected ollama provider, got %s", ws.Embedder.Provider)
		}
		if ws.Embedder.Model != "nomic-embed-text" {
			t.Errorf("expected nomic-embed-text model, got %s", ws.Embedder.Model)
		}
	})

	t.Run("flags_postgres_openai", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-test-cli")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		ws, err := buildWorkspaceFromFlags("test-ws", "postgres", "openai", "text-embedding-3-small", "postgres://localhost:5432/grepai", "", "", 0, "", false)
		if err != nil {
			t.Fatalf("buildWorkspaceFromFlags error: %v", err)
		}
		if ws.Store.Backend != "postgres" {
			t.Errorf("expected postgres backend, got %s", ws.Store.Backend)
		}
		if ws.Store.Postgres.DSN != "postgres://localhost:5432/grepai" {
			t.Errorf("expected DSN, got %s", ws.Store.Postgres.DSN)
		}
		if ws.Embedder.Provider != "openai" {
			t.Errorf("expected openai provider, got %s", ws.Embedder.Provider)
		}
	})

	t.Run("yes_defaults", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-test-cli")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		ws, err := buildWorkspaceFromFlags("test-ws", "", "", "", "", "", "", 0, "", true)
		if err != nil {
			t.Fatalf("buildWorkspaceFromFlags error: %v", err)
		}
		if ws.Store.Backend != "qdrant" {
			t.Errorf("expected qdrant default, got %s", ws.Store.Backend)
		}
		if ws.Embedder.Provider != "ollama" {
			t.Errorf("expected ollama default, got %s", ws.Embedder.Provider)
		}
		if ws.Embedder.Model != "nomic-embed-text" {
			t.Errorf("expected nomic-embed-text default, got %s", ws.Embedder.Model)
		}
	})

	t.Run("backend_required_without_yes", func(t *testing.T) {
		_, err := buildWorkspaceFromFlags("test-ws", "", "", "", "", "", "", 0, "", false)
		if err == nil {
			t.Error("expected error when no backend and no --yes")
		}
	})

	t.Run("from_yaml_file", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-test-cli")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		yamlContent := `store:
  backend: qdrant
  qdrant:
    endpoint: http://localhost
    port: 6334
embedder:
  provider: ollama
  model: mxbai-embed-large
  endpoint: http://localhost:11434
  dimensions: 1024
`
		yamlPath := filepath.Join(tmpDir, "ws-config.yaml")
		os.WriteFile(yamlPath, []byte(yamlContent), 0644)

		ws, err := buildWorkspaceFromFile("test-ws", yamlPath)
		if err != nil {
			t.Fatalf("buildWorkspaceFromFile error: %v", err)
		}
		if ws.Store.Backend != "qdrant" {
			t.Errorf("expected qdrant, got %s", ws.Store.Backend)
		}
		if ws.Embedder.Model != "mxbai-embed-large" {
			t.Errorf("expected mxbai-embed-large, got %s", ws.Embedder.Model)
		}
		if ws.Embedder.Dimensions == nil || *ws.Embedder.Dimensions != 1024 {
			t.Errorf("expected dimensions 1024")
		}
	})

	t.Run("from_json_file", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-test-cli")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		jsonContent := `{
  "store": {"backend": "postgres", "postgres": {"dsn": "postgres://localhost/test"}},
  "embedder": {"provider": "openai", "model": "text-embedding-3-small", "endpoint": "https://api.openai.com/v1"}
}`
		jsonPath := filepath.Join(tmpDir, "ws-config.json")
		os.WriteFile(jsonPath, []byte(jsonContent), 0644)

		ws, err := buildWorkspaceFromFile("test-ws", jsonPath)
		if err != nil {
			t.Fatalf("buildWorkspaceFromFile error: %v", err)
		}
		if ws.Store.Backend != "postgres" {
			t.Errorf("expected postgres, got %s", ws.Store.Backend)
		}
		if ws.Embedder.Provider != "openai" {
			t.Errorf("expected openai, got %s", ws.Embedder.Provider)
		}
	})
}

func TestIntegration_WorkspaceCreateAndMCPResolve(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "grepai-integration")
	defer os.RemoveAll(tmpDir)
	cleanup := setTestHomeDirCLI(t, tmpDir)
	defer cleanup()

	// Step 1: Create workspace non-interactively
	ws, err := buildWorkspaceFromFlags("test", "qdrant", "ollama", "nomic-embed-text", "", "", "http://localhost", 6334, "", false)
	if err != nil {
		t.Fatalf("buildWorkspaceFromFlags: %v", err)
	}

	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		t.Fatalf("ValidateWorkspaceBackend: %v", err)
	}

	cfg := config.DefaultWorkspaceConfig()
	cfg.AddWorkspace(*ws)

	// Step 2: Add a project
	projectDir := filepath.Join(tmpDir, "myproject")
	os.MkdirAll(projectDir, 0755)
	cfg.AddProject("test", config.ProjectEntry{
		Name: "myproject",
		Path: projectDir,
	})
	config.SaveWorkspaceConfig(cfg)

	// Step 3: Verify FindWorkspaceForPath works
	name, foundWs, err := config.FindWorkspaceForPath(projectDir)
	if err != nil {
		t.Fatalf("FindWorkspaceForPath: %v", err)
	}
	if foundWs == nil {
		t.Fatal("expected workspace, got nil")
	}
	if name != "test" {
		t.Errorf("expected test, got %s", name)
	}

	// Step 4: Verify subdirectory also matches
	subDir := filepath.Join(projectDir, "src", "main")
	os.MkdirAll(subDir, 0755)
	name, foundWs, err = config.FindWorkspaceForPath(subDir)
	if err != nil {
		t.Fatalf("FindWorkspaceForPath subdir: %v", err)
	}
	if foundWs == nil {
		t.Fatal("expected workspace for subdir, got nil")
	}
	if name != "test" {
		t.Errorf("expected test for subdir, got %s", name)
	}

	// Step 5: Verify MCP resolution with --workspace
	projectRoot, wsName, err := resolveMCPTarget("", "test")
	if err != nil {
		t.Fatalf("resolveMCPTarget --workspace: %v", err)
	}
	if wsName != "test" {
		t.Errorf("expected workspace test, got %s", wsName)
	}
	_ = projectRoot
}
