package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"gopkg.in/yaml.v3"
)

var (
	workspaceCreateUI bool
	workspaceStatusUI bool
)

var (
	workspaceStatusUISelector = shouldUseStatusUI
	workspaceStatusUIRunner   = runWorkspaceStatusUI
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
	Short: "Create a new workspace",
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

	// Non-interactive workspace create flags
	workspaceCreateCmd.Flags().String("backend", "", "Storage backend: postgres, qdrant")
	workspaceCreateCmd.Flags().String("provider", "", "Embedding provider: ollama, openai, lmstudio")
	workspaceCreateCmd.Flags().String("model", "", "Embedding model name")
	workspaceCreateCmd.Flags().String("endpoint", "", "Embedder endpoint URL")
	workspaceCreateCmd.Flags().String("dsn", "", "PostgreSQL DSN (when backend=postgres)")
	workspaceCreateCmd.Flags().String("qdrant-endpoint", "", "Qdrant endpoint (default: http://localhost)")
	workspaceCreateCmd.Flags().Int("qdrant-port", 0, "Qdrant gRPC port (default: 6334)")
	workspaceCreateCmd.Flags().String("collection", "", "Qdrant collection name (empty = auto)")
	workspaceCreateCmd.Flags().String("from", "", "Path to JSON/YAML file with workspace config")
	workspaceCreateCmd.Flags().Bool("yes", false, "Use defaults for unspecified values, skip prompts")
	workspaceCreateCmd.Flags().BoolVar(&workspaceCreateUI, "ui", false, "Run interactive Bubble Tea UI wizard")
	workspaceStatusCmd.Flags().BoolVar(&workspaceStatusUI, "ui", false, "Show workspace status in interactive UI")
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
	if ws.Embedder.Dimensions != nil {
		fmt.Printf("  Dimensions: %d\n", *ws.Embedder.Dimensions)
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
		if workspaceStatusUI && workspaceStatusUISelector(isInteractiveTerminal(), false) {
			return workspaceStatusUIRunner(cfg, args)
		}
		return showWorkspaceStatus(ws)
	}

	if workspaceStatusUI && workspaceStatusUISelector(isInteractiveTerminal(), false) {
		return workspaceStatusUIRunner(cfg, args)
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

// buildWorkspaceFromFlags constructs a Workspace from CLI flags.
// If useYes is true, missing values use sensible defaults.
// If useYes is false and backend is empty, returns an error.
func buildWorkspaceFromFlags(name, backend, provider, model, dsn, endpoint, qdrantEndpoint string, qdrantPort int, collection string, useYes bool) (*config.Workspace, error) {
	if backend == "" {
		if useYes {
			backend = "qdrant"
		} else {
			return nil, fmt.Errorf("--backend is required in non-interactive mode (or use --yes for defaults)")
		}
	}
	if provider == "" {
		provider = "ollama"
	}
	if model == "" {
		switch provider {
		case "openai":
			model = "text-embedding-3-small"
		default:
			model = "nomic-embed-text"
		}
	}

	var storeConfig config.StoreConfig
	storeConfig.Backend = backend

	switch backend {
	case "postgres":
		if dsn == "" {
			dsn = "postgres://localhost:5432/grepai"
		}
		storeConfig.Postgres.DSN = dsn
	case "qdrant":
		if qdrantEndpoint == "" {
			qdrantEndpoint = "http://localhost"
		}
		if qdrantPort == 0 {
			qdrantPort = 6334
		}
		storeConfig.Qdrant.Endpoint = qdrantEndpoint
		storeConfig.Qdrant.Port = qdrantPort
		storeConfig.Qdrant.Collection = collection
	default:
		return nil, fmt.Errorf("unsupported backend: %s (use postgres or qdrant)", backend)
	}

	var embedderConfig config.EmbedderConfig
	embedderConfig.Provider = provider
	embedderConfig.Model = model

	switch provider {
	case "ollama":
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		embedderConfig.Endpoint = endpoint
		dim := 768
		embedderConfig.Dimensions = &dim
	case "lmstudio":
		if endpoint == "" {
			endpoint = "http://127.0.0.1:1234"
		}
		embedderConfig.Endpoint = endpoint
		dim := 768
		embedderConfig.Dimensions = &dim
	case "openai":
		if endpoint == "" {
			endpoint = "https://api.openai.com/v1"
		}
		embedderConfig.Endpoint = endpoint
	default:
		return nil, fmt.Errorf("unsupported provider: %s (use ollama, openai, or lmstudio)", provider)
	}

	return &config.Workspace{
		Name:     name,
		Store:    storeConfig,
		Embedder: embedderConfig,
		Projects: []config.ProjectEntry{},
	}, nil
}

// WorkspaceFileConfig is the struct for --from file input.
type WorkspaceFileConfig struct {
	Store    config.StoreConfig    `yaml:"store" json:"store"`
	Embedder config.EmbedderConfig `yaml:"embedder" json:"embedder"`
}

// buildWorkspaceFromFile loads workspace config from a JSON or YAML file.
func buildWorkspaceFromFile(name, filePath string) (*config.Workspace, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var fileCfg WorkspaceFileConfig

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			if err2 := json.Unmarshal(data, &fileCfg); err2 != nil {
				return nil, fmt.Errorf("failed to parse config file (tried YAML and JSON): %w", err2)
			}
		}
	}

	return &config.Workspace{
		Name:     name,
		Store:    fileCfg.Store,
		Embedder: fileCfg.Embedder,
		Projects: []config.ProjectEntry{},
	}, nil
}

// hasNonInteractiveFlags checks if any non-interactive flag was explicitly set.
func hasNonInteractiveFlags(cmd *cobra.Command) bool {
	flags := []string{"backend", "provider", "model", "endpoint", "dsn",
		"qdrant-endpoint", "qdrant-port", "collection", "from", "yes"}
	for _, f := range flags {
		if cmd.Flags().Changed(f) {
			return true
		}
	}
	return false
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

	fromFile, _ := cmd.Flags().GetString("from")

	var ws *config.Workspace

	if fromFile != "" {
		ws, err = buildWorkspaceFromFile(workspaceName, fromFile)
		if err != nil {
			return err
		}
	} else if hasNonInteractiveFlags(cmd) {
		backend, _ := cmd.Flags().GetString("backend")
		provider, _ := cmd.Flags().GetString("provider")
		model, _ := cmd.Flags().GetString("model")
		dsn, _ := cmd.Flags().GetString("dsn")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		qdrantEndpoint, _ := cmd.Flags().GetString("qdrant-endpoint")
		qdrantPort, _ := cmd.Flags().GetInt("qdrant-port")
		collection, _ := cmd.Flags().GetString("collection")
		useYes, _ := cmd.Flags().GetBool("yes")

		ws, err = buildWorkspaceFromFlags(workspaceName, backend, provider, model, dsn, endpoint, qdrantEndpoint, qdrantPort, collection, useYes)
		if err != nil {
			return err
		}
	} else {
		if workspaceCreateUI {
			ws, err = createWorkspaceTUI(workspaceName)
			if err != nil {
				return err
			}
		} else {
			ws, err = createWorkspaceInteractive(workspaceName)
			if err != nil {
				return err
			}
		}
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Add workspace
	if err := cfg.AddWorkspace(*ws); err != nil {
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

// createWorkspaceInteractive runs the original interactive prompt flow.
func createWorkspaceInteractive(workspaceName string) (*config.Workspace, error) {
	reader := bufio.NewReader(os.Stdin)

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
		return nil, fmt.Errorf("invalid choice: %s", backendChoice)
	}

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
		dim := 768
		embedderConfig.Dimensions = &dim
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
		dim := 768
		embedderConfig.Dimensions = &dim
	default:
		return nil, fmt.Errorf("invalid choice: %s", embedderChoice)
	}

	return &config.Workspace{
		Name:     workspaceName,
		Store:    storeConfig,
		Embedder: embedderConfig,
		Projects: []config.ProjectEntry{},
	}, nil
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
