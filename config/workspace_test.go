package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceConfigOperations(t *testing.T) {
	// Create temp directory for test config
	tmpDir, err := os.MkdirTemp("", "grepai-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test DefaultWorkspaceConfig
	t.Run("DefaultWorkspaceConfig", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()
		if cfg.Version != 1 {
			t.Errorf("expected version 1, got %d", cfg.Version)
		}
		if cfg.Workspaces == nil {
			t.Error("expected non-nil workspaces map")
		}
		if len(cfg.Workspaces) != 0 {
			t.Errorf("expected 0 workspaces, got %d", len(cfg.Workspaces))
		}
	})

	// Test AddWorkspace
	t.Run("AddWorkspace", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
				Postgres: PostgresConfig{
					DSN: "postgres://localhost:5432/test",
				},
			},
			Embedder: EmbedderConfig{
				Provider:   "ollama",
				Model:      "nomic-embed-text",
				Endpoint:   "http://localhost:11434",
				Dimensions: 768,
			},
			Projects: []ProjectEntry{},
		}

		err := cfg.AddWorkspace(ws)
		if err != nil {
			t.Errorf("failed to add workspace: %v", err)
		}

		if len(cfg.Workspaces) != 1 {
			t.Errorf("expected 1 workspace, got %d", len(cfg.Workspaces))
		}

		// Try to add duplicate
		err = cfg.AddWorkspace(ws)
		if err == nil {
			t.Error("expected error when adding duplicate workspace")
		}
	})

	// Test GetWorkspace
	t.Run("GetWorkspace", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
			},
		}
		cfg.AddWorkspace(ws)

		retrieved, err := cfg.GetWorkspace("test-workspace")
		if err != nil {
			t.Errorf("failed to get workspace: %v", err)
		}
		if retrieved.Name != "test-workspace" {
			t.Errorf("expected workspace name 'test-workspace', got %s", retrieved.Name)
		}

		// Try to get non-existent workspace
		_, err = cfg.GetWorkspace("non-existent")
		if err == nil {
			t.Error("expected error when getting non-existent workspace")
		}
	})

	// Test RemoveWorkspace
	t.Run("RemoveWorkspace", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
			},
		}
		cfg.AddWorkspace(ws)

		err := cfg.RemoveWorkspace("test-workspace")
		if err != nil {
			t.Errorf("failed to remove workspace: %v", err)
		}

		if len(cfg.Workspaces) != 0 {
			t.Errorf("expected 0 workspaces after removal, got %d", len(cfg.Workspaces))
		}

		// Try to remove non-existent workspace
		err = cfg.RemoveWorkspace("non-existent")
		if err == nil {
			t.Error("expected error when removing non-existent workspace")
		}
	})

	// Test AddProject
	t.Run("AddProject", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
			},
			Projects: []ProjectEntry{},
		}
		cfg.AddWorkspace(ws)

		project := ProjectEntry{
			Name: "test-project",
			Path: "/path/to/project",
		}

		err := cfg.AddProject("test-workspace", project)
		if err != nil {
			t.Errorf("failed to add project: %v", err)
		}

		retrieved, _ := cfg.GetWorkspace("test-workspace")
		if len(retrieved.Projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(retrieved.Projects))
		}

		// Try to add duplicate project name
		err = cfg.AddProject("test-workspace", project)
		if err == nil {
			t.Error("expected error when adding duplicate project name")
		}

		// Try to add duplicate project path
		project2 := ProjectEntry{
			Name: "different-name",
			Path: "/path/to/project",
		}
		err = cfg.AddProject("test-workspace", project2)
		if err == nil {
			t.Error("expected error when adding duplicate project path")
		}
	})

	// Test RemoveProject
	t.Run("RemoveProject", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
			},
			Projects: []ProjectEntry{
				{Name: "project1", Path: "/path/1"},
				{Name: "project2", Path: "/path/2"},
			},
		}
		cfg.AddWorkspace(ws)

		err := cfg.RemoveProject("test-workspace", "project1")
		if err != nil {
			t.Errorf("failed to remove project: %v", err)
		}

		retrieved, _ := cfg.GetWorkspace("test-workspace")
		if len(retrieved.Projects) != 1 {
			t.Errorf("expected 1 project after removal, got %d", len(retrieved.Projects))
		}

		// Try to remove non-existent project
		err = cfg.RemoveProject("test-workspace", "non-existent")
		if err == nil {
			t.Error("expected error when removing non-existent project")
		}
	})

	// Test SaveWorkspaceConfig and LoadWorkspaceConfig
	t.Run("SaveAndLoadWorkspaceConfig", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		ws := Workspace{
			Name: "test-workspace",
			Store: StoreConfig{
				Backend: "postgres",
				Postgres: PostgresConfig{
					DSN: "postgres://localhost:5432/test",
				},
			},
			Embedder: EmbedderConfig{
				Provider:   "ollama",
				Model:      "nomic-embed-text",
				Endpoint:   "http://localhost:11434",
				Dimensions: 768,
			},
			Projects: []ProjectEntry{
				{Name: "project1", Path: "/path/1"},
			},
		}
		cfg.AddWorkspace(ws)

		err := SaveWorkspaceConfig(cfg)
		if err != nil {
			t.Errorf("failed to save workspace config: %v", err)
		}

		// Verify file exists
		configPath, _ := GetWorkspaceConfigPath()
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("workspace config file was not created")
		}

		// Load and verify
		loaded, err := LoadWorkspaceConfig()
		if err != nil {
			t.Errorf("failed to load workspace config: %v", err)
		}
		if loaded == nil {
			t.Fatal("loaded config is nil")
		}
		if len(loaded.Workspaces) != 1 {
			t.Errorf("expected 1 workspace, got %d", len(loaded.Workspaces))
		}

		loadedWs, _ := loaded.GetWorkspace("test-workspace")
		if loadedWs.Store.Backend != "postgres" {
			t.Errorf("expected postgres backend, got %s", loadedWs.Store.Backend)
		}
		if len(loadedWs.Projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(loadedWs.Projects))
		}
	})

	// Test ListWorkspaces
	t.Run("ListWorkspaces", func(t *testing.T) {
		cfg := DefaultWorkspaceConfig()

		cfg.AddWorkspace(Workspace{Name: "ws1", Store: StoreConfig{Backend: "postgres"}})
		cfg.AddWorkspace(Workspace{Name: "ws2", Store: StoreConfig{Backend: "qdrant"}})

		names := cfg.ListWorkspaces()
		if len(names) != 2 {
			t.Errorf("expected 2 workspace names, got %d", len(names))
		}
	})

	// Test LoadWorkspaceConfig with non-existent file
	t.Run("LoadWorkspaceConfig_NonExistent", func(t *testing.T) {
		// Create new temp dir without config file
		emptyDir, _ := os.MkdirTemp("", "grepai-empty")
		defer os.RemoveAll(emptyDir)

		os.Setenv("HOME", emptyDir)

		cfg, err := LoadWorkspaceConfig()
		if err != nil {
			t.Errorf("expected no error for non-existent file, got %v", err)
		}
		if cfg != nil {
			t.Error("expected nil config for non-existent file")
		}
	})
}

func TestValidateWorkspaceBackend(t *testing.T) {
	tests := []struct {
		name      string
		workspace Workspace
		wantErr   bool
	}{
		{
			name: "postgres_valid",
			workspace: Workspace{
				Name:  "test",
				Store: StoreConfig{Backend: "postgres"},
			},
			wantErr: false,
		},
		{
			name: "qdrant_valid",
			workspace: Workspace{
				Name:  "test",
				Store: StoreConfig{Backend: "qdrant"},
			},
			wantErr: false,
		},
		{
			name: "gob_invalid",
			workspace: Workspace{
				Name:  "test",
				Store: StoreConfig{Backend: "gob"},
			},
			wantErr: true,
		},
		{
			name: "empty_invalid",
			workspace: Workspace{
				Name:  "test",
				Store: StoreConfig{Backend: ""},
			},
			wantErr: true,
		},
		{
			name: "unknown_invalid",
			workspace: Workspace{
				Name:  "test",
				Store: StoreConfig{Backend: "unknown"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspaceBackend(&tt.workspace)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkspaceBackend() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetGlobalConfigDir(t *testing.T) {
	// Create temp dir to use as home
	tmpDir, err := os.MkdirTemp("", "grepai-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	dir, err := GetGlobalConfigDir()
	if err != nil {
		t.Errorf("failed to get global config dir: %v", err)
	}

	expected := filepath.Join(tmpDir, ".grepai")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestGetWorkspaceConfigPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "grepai-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := GetWorkspaceConfigPath()
	if err != nil {
		t.Errorf("failed to get workspace config path: %v", err)
	}

	expected := filepath.Join(tmpDir, ".grepai", "workspace.yaml")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}
