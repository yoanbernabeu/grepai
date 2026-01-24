package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage multi-project workspaces",
	Long: `Manage multi-project workspaces for cross-project indexing and search.

A workspace allows you to index and search across multiple projects using a
shared vector store (PostgreSQL or Qdrant).

Note: GOB backend is not supported for workspaces as it's file-based.`,
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workspaces",
	RunE:  runWorkspaceList,
}

var workspaceShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show workspace details",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceShow,
}

var workspaceStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show workspace indexing status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorkspaceStatus,
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new workspace (interactive)",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceCreate,
}

var workspaceAddCmd = &cobra.Command{
	Use:   "add <workspace> <path>",
	Short: "Add a project to a workspace",
	Long: `Add a project to an existing workspace.

The project name will be derived from the directory name.
The project path must be an absolute path to a directory.`,
	Args: cobra.ExactArgs(2),
	RunE: runWorkspaceAdd,
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove <workspace> <project>",
	Short: "Remove a project from a workspace",
	Args:  cobra.ExactArgs(2),
	RunE:  runWorkspaceRemove,
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceDelete,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
	workspaceCmd.AddCommand(workspaceStatusCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceAddCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}

	if cfg == nil || len(cfg.Workspaces) == 0 {
		fmt.Println("No workspaces configured.")
		fmt.Println("\nCreate one with: grepai workspace create <name>")
		return nil
	}

	fmt.Printf("Workspaces (%d):\n\n", len(cfg.Workspaces))
	for name, ws := range cfg.Workspaces {
		fmt.Printf("  %s\n", name)
		fmt.Printf("    Backend: %s\n", ws.Store.Backend)
		fmt.Printf("    Embedder: %s (%s)\n", ws.Embedder.Provider, ws.Embedder.Model)
		fmt.Printf("    Projects: %d\n", len(ws.Projects))
	}

	return nil
}

func runWorkspaceShow(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]

	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}

	ws, err := cfg.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}

	fmt.Printf("Workspace: %s\n\n", ws.Name)
	fmt.Printf("Store:\n")
	fmt.Printf("  Backend: %s\n", ws.Store.Backend)
	switch ws.Store.Backend {
	case "postgres":
		// Mask DSN for security
		dsn := ws.Store.Postgres.DSN
		if dsn != "" {
			dsn = maskDSN(dsn)
		}
		fmt.Printf("  DSN: %s\n", dsn)
	case "qdrant":
		fmt.Printf("  Endpoint: %s\n", ws.Store.Qdrant.Endpoint)
		fmt.Printf("  Port: %d\n", ws.Store.Qdrant.Port)
		if ws.Store.Qdrant.Collection != "" {
			fmt.Printf("  Collection: %s\n", ws.Store.Qdrant.Collection)
		}
	}

	fmt.Printf("\nEmbedder:\n")
	fmt.Printf("  Provider: %s\n", ws.Embedder.Provider)
	fmt.Printf("  Model: %s\n", ws.Embedder.Model)
	if ws.Embedder.Endpoint != "" {
		fmt.Printf("  Endpoint: %s\n", ws.Embedder.Endpoint)
	}
	if ws.Embedder.Dimensions > 0 {
		fmt.Printf("  Dimensions: %d\n", ws.Embedder.Dimensions)
	}

	fmt.Printf("\nProjects (%d):\n", len(ws.Projects))
	for _, p := range ws.Projects {
		fmt.Printf("  - %s: %s\n", p.Name, p.Path)
	}

	return nil
}

func runWorkspaceStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}

	if cfg == nil || len(cfg.Workspaces) == 0 {
		fmt.Println("No workspaces configured.")
		return nil
	}

	// If name provided, show status for that workspace only
	if len(args) > 0 {
		ws, err := cfg.GetWorkspace(args[0])
		if err != nil {
			return err
		}
		return showWorkspaceStatus(ws)
	}

	// Otherwise show status for all workspaces
	for _, ws := range cfg.Workspaces {
		if err := showWorkspaceStatus(&ws); err != nil {
			fmt.Printf("Error getting status for %s: %v\n", ws.Name, err)
		}
		fmt.Println()
	}

	return nil
}

func showWorkspaceStatus(ws *config.Workspace) error {
	fmt.Printf("Workspace: %s\n", ws.Name)
	fmt.Printf("  Backend: %s\n", ws.Store.Backend)
	fmt.Printf("  Projects: %d\n", len(ws.Projects))

	// Check if each project path exists
	for _, p := range ws.Projects {
		exists := "✓"
		if _, err := os.Stat(p.Path); os.IsNotExist(err) {
			exists = "✗ (path not found)"
		}
		fmt.Printf("    - %s: %s %s\n", p.Name, p.Path, exists)
	}

	return nil
}

func runWorkspaceCreate(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]

	// Load or create config
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if cfg == nil {
		cfg = config.DefaultWorkspaceConfig()
	}

	// Check if workspace already exists
	if _, err := cfg.GetWorkspace(workspaceName); err == nil {
		return fmt.Errorf("workspace %q already exists", workspaceName)
	}

	reader := bufio.NewReader(os.Stdin)

	// Select backend
	fmt.Println("Select storage backend:")
	fmt.Println("  1. PostgreSQL (recommended for production)")
	fmt.Println("  2. Qdrant (for advanced vector search)")
	fmt.Print("Choice [1]: ")
	backendChoice, _ := reader.ReadString('\n')
	backendChoice = strings.TrimSpace(backendChoice)
	if backendChoice == "" {
		backendChoice = "1"
	}

	var storeConfig config.StoreConfig
	switch backendChoice {
	case "1":
		storeConfig.Backend = "postgres"
		fmt.Print("PostgreSQL DSN [postgres://localhost:5432/grepai]: ")
		dsn, _ := reader.ReadString('\n')
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			dsn = "postgres://localhost:5432/grepai"
		}
		storeConfig.Postgres.DSN = dsn
	case "2":
		storeConfig.Backend = "qdrant"
		fmt.Print("Qdrant endpoint [http://localhost]: ")
		endpoint, _ := reader.ReadString('\n')
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			endpoint = "http://localhost"
		}
		storeConfig.Qdrant.Endpoint = endpoint
		fmt.Print("Qdrant port [6334]: ")
		portStr, _ := reader.ReadString('\n')
		portStr = strings.TrimSpace(portStr)
		port := 6334
		if portStr != "" {
			_, _ = fmt.Sscanf(portStr, "%d", &port)
		}
		storeConfig.Qdrant.Port = port
		fmt.Print("Collection name (leave empty for auto): ")
		collection, _ := reader.ReadString('\n')
		storeConfig.Qdrant.Collection = strings.TrimSpace(collection)
	default:
		return fmt.Errorf("invalid choice: %s", backendChoice)
	}

	// Select embedder
	fmt.Println("\nSelect embedding provider:")
	fmt.Println("  1. Ollama (local, default)")
	fmt.Println("  2. OpenAI")
	fmt.Println("  3. LM Studio (local)")
	fmt.Print("Choice [1]: ")
	embedderChoice, _ := reader.ReadString('\n')
	embedderChoice = strings.TrimSpace(embedderChoice)
	if embedderChoice == "" {
		embedderChoice = "1"
	}

	var embedderConfig config.EmbedderConfig
	switch embedderChoice {
	case "1":
		embedderConfig.Provider = "ollama"
		fmt.Print("Ollama endpoint [http://localhost:11434]: ")
		endpoint, _ := reader.ReadString('\n')
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		embedderConfig.Endpoint = endpoint
		fmt.Print("Model [nomic-embed-text]: ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)
		if model == "" {
			model = "nomic-embed-text"
		}
		embedderConfig.Model = model
		embedderConfig.Dimensions = 768
	case "2":
		embedderConfig.Provider = "openai"
		fmt.Print("OpenAI API Key: ")
		apiKey, _ := reader.ReadString('\n')
		embedderConfig.APIKey = strings.TrimSpace(apiKey)
		fmt.Print("Model [text-embedding-3-small]: ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)
		if model == "" {
			model = "text-embedding-3-small"
		}
		embedderConfig.Model = model
		embedderConfig.Endpoint = "https://api.openai.com/v1"
		embedderConfig.Dimensions = 1536
	case "3":
		embedderConfig.Provider = "lmstudio"
		fmt.Print("LM Studio endpoint [http://127.0.0.1:1234]: ")
		endpoint, _ := reader.ReadString('\n')
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			endpoint = "http://127.0.0.1:1234"
		}
		embedderConfig.Endpoint = endpoint
		fmt.Print("Model [nomic-embed-text]: ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)
		if model == "" {
			model = "nomic-embed-text"
		}
		embedderConfig.Model = model
		embedderConfig.Dimensions = 768
	default:
		return fmt.Errorf("invalid choice: %s", embedderChoice)
	}

	// Create workspace
	ws := config.Workspace{
		Name:     workspaceName,
		Store:    storeConfig,
		Embedder: embedderConfig,
		Projects: []config.ProjectEntry{},
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(&ws); err != nil {
		return err
	}

	// Add workspace
	if err := cfg.AddWorkspace(ws); err != nil {
		return err
	}

	// Save config
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("\nWorkspace %q created successfully.\n", workspaceName)
	fmt.Printf("\nAdd projects with: grepai workspace add %s <path>\n", workspaceName)
	fmt.Printf("Start indexing with: grepai watch --workspace %s\n", workspaceName)

	return nil
}

func runWorkspaceAdd(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]
	projectPath := args[1]

	// Make path absolute
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Load config
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("no workspaces configured; create one first with: grepai workspace create <name>")
	}

	// Derive project name from directory
	projectName := filepath.Base(absPath)

	// Add project
	if err := cfg.AddProject(workspaceName, config.ProjectEntry{
		Name: projectName,
		Path: absPath,
	}); err != nil {
		return err
	}

	// Save config
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("Added project %q (%s) to workspace %q\n", projectName, absPath, workspaceName)
	return nil
}

func runWorkspaceRemove(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]
	projectName := args[1]

	// Load config
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("no workspaces configured")
	}

	// Remove project
	if err := cfg.RemoveProject(workspaceName, projectName); err != nil {
		return err
	}

	// Save config
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("Removed project %q from workspace %q\n", projectName, workspaceName)
	return nil
}

func runWorkspaceDelete(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]

	// Load config
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("no workspaces configured")
	}

	// Remove workspace
	if err := cfg.RemoveWorkspace(workspaceName); err != nil {
		return err
	}

	// Save config
	if err := config.SaveWorkspaceConfig(cfg); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("Deleted workspace %q\n", workspaceName)
	return nil
}

// maskDSN masks password in DSN for display
func maskDSN(dsn string) string {
	// Simple password masking for postgresql:// URLs
	if strings.Contains(dsn, "@") {
		parts := strings.SplitN(dsn, "@", 2)
		if len(parts) == 2 {
			prefix := parts[0]
			// Find password part (after : in user:pass)
			if idx := strings.LastIndex(prefix, ":"); idx != -1 {
				return prefix[:idx+1] + "****@" + parts[1]
			}
		}
	}
	return dsn
}
