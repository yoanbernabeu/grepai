package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	WorkspaceConfigFileName = "workspace.yaml"
)

// WorkspaceConfig holds global workspace configuration.
// Stored at ~/.grepai/workspace.yaml
type WorkspaceConfig struct {
	Version    int                  `yaml:"version"`
	Workspaces map[string]Workspace `yaml:"workspaces"`
}

// Workspace represents a multi-project workspace configuration.
type Workspace struct {
	Name     string         `yaml:"name"`
	Store    StoreConfig    `yaml:"store"`
	Embedder EmbedderConfig `yaml:"embedder"`
	Projects []ProjectEntry `yaml:"projects"`
}

// ProjectEntry represents a single project within a workspace.
type ProjectEntry struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// GetGlobalConfigDir returns the global grepai config directory path.
// ~/.grepai on Unix-like systems
func GetGlobalConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".grepai"), nil
}

// GetWorkspaceConfigPath returns the path to the workspace config file.
func GetWorkspaceConfigPath() (string, error) {
	globalDir, err := GetGlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(globalDir, WorkspaceConfigFileName), nil
}

// LoadWorkspaceConfig loads the workspace configuration from ~/.grepai/workspace.yaml.
// Returns nil, nil if the file doesn't exist.
func LoadWorkspaceConfig() (*WorkspaceConfig, error) {
	configPath, err := GetWorkspaceConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workspace config: %w", err)
	}

	var cfg WorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse workspace config: %w", err)
	}

	// Apply defaults
	if cfg.Workspaces == nil {
		cfg.Workspaces = make(map[string]Workspace)
	}

	return &cfg, nil
}

// SaveWorkspaceConfig saves the workspace configuration to ~/.grepai/workspace.yaml.
func SaveWorkspaceConfig(cfg *WorkspaceConfig) error {
	globalDir, err := GetGlobalConfigDir()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return fmt.Errorf("failed to create global config directory: %w", err)
	}

	configPath := filepath.Join(globalDir, WorkspaceConfigFileName)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write workspace config: %w", err)
	}

	return nil
}

// GetWorkspace returns a workspace by name.
func (c *WorkspaceConfig) GetWorkspace(name string) (*Workspace, error) {
	if c == nil || c.Workspaces == nil {
		return nil, fmt.Errorf("workspace %q not found", name)
	}

	ws, ok := c.Workspaces[name]
	if !ok {
		return nil, fmt.Errorf("workspace %q not found", name)
	}

	return &ws, nil
}

// AddWorkspace adds a new workspace to the configuration.
func (c *WorkspaceConfig) AddWorkspace(ws Workspace) error {
	if c.Workspaces == nil {
		c.Workspaces = make(map[string]Workspace)
	}

	if _, exists := c.Workspaces[ws.Name]; exists {
		return fmt.Errorf("workspace %q already exists", ws.Name)
	}

	c.Workspaces[ws.Name] = ws
	return nil
}

// RemoveWorkspace removes a workspace from the configuration.
func (c *WorkspaceConfig) RemoveWorkspace(name string) error {
	if c.Workspaces == nil {
		return fmt.Errorf("workspace %q not found", name)
	}

	if _, exists := c.Workspaces[name]; !exists {
		return fmt.Errorf("workspace %q not found", name)
	}

	delete(c.Workspaces, name)
	return nil
}

// AddProject adds a project to a workspace.
func (c *WorkspaceConfig) AddProject(workspaceName string, project ProjectEntry) error {
	ws, err := c.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}

	// Check if project already exists
	for _, p := range ws.Projects {
		if p.Name == project.Name {
			return fmt.Errorf("project %q already exists in workspace %q", project.Name, workspaceName)
		}
		if p.Path == project.Path {
			return fmt.Errorf("project path %q already exists in workspace %q", project.Path, workspaceName)
		}
	}

	ws.Projects = append(ws.Projects, project)
	c.Workspaces[workspaceName] = *ws
	return nil
}

// RemoveProject removes a project from a workspace.
func (c *WorkspaceConfig) RemoveProject(workspaceName, projectName string) error {
	ws, err := c.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}

	found := false
	projects := make([]ProjectEntry, 0, len(ws.Projects))
	for _, p := range ws.Projects {
		if p.Name == projectName {
			found = true
			continue
		}
		projects = append(projects, p)
	}

	if !found {
		return fmt.Errorf("project %q not found in workspace %q", projectName, workspaceName)
	}

	ws.Projects = projects
	c.Workspaces[workspaceName] = *ws
	return nil
}

// ValidateWorkspaceBackend validates that the workspace uses a supported backend.
// GOB backend is not supported for workspaces (file-based, can't be shared).
func ValidateWorkspaceBackend(ws *Workspace) error {
	if ws.Store.Backend == "gob" || ws.Store.Backend == "" {
		return fmt.Errorf("workspace %q uses GOB backend which is not supported for multi-project workspaces; use 'postgres' or 'qdrant' instead", ws.Name)
	}

	if ws.Store.Backend != "postgres" && ws.Store.Backend != "qdrant" {
		return fmt.Errorf("unknown backend %q for workspace %q; supported backends: postgres, qdrant", ws.Store.Backend, ws.Name)
	}

	return nil
}

// DefaultWorkspaceConfig returns an empty workspace configuration.
func DefaultWorkspaceConfig() *WorkspaceConfig {
	return &WorkspaceConfig{
		Version:    1,
		Workspaces: make(map[string]Workspace),
	}
}

// ListWorkspaces returns a list of all workspace names.
func (c *WorkspaceConfig) ListWorkspaces() []string {
	if c == nil || c.Workspaces == nil {
		return nil
	}

	names := make([]string, 0, len(c.Workspaces))
	for name := range c.Workspaces {
		names = append(names, name)
	}
	return names
}
