package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
)

var (
	searchLimit     int
	searchJSON      bool
	searchCompact   bool
	searchWorkspace string
	searchProjects  []string
)

// SearchResultJSON is a lightweight struct for JSON output (excludes vector, hash, updated_at)
type SearchResultJSON struct {
	FilePath  string  `json:"file_path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
	Content   string  `json:"content"`
}

// SearchResultCompactJSON is a minimal struct for compact JSON output (no content field)
type SearchResultCompactJSON struct {
	FilePath  string  `json:"file_path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search codebase with natural language",
	Long: `Search your codebase using natural language queries.

The search will:
- Vectorize your query using the configured embedding provider
- Calculate cosine similarity against indexed code chunks
- Return the most relevant results with file path, line numbers, and score`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "Maximum number of results to return")
	searchCmd.Flags().BoolVarP(&searchJSON, "json", "j", false, "Output results in JSON format (for AI agents)")
	searchCmd.Flags().BoolVarP(&searchCompact, "compact", "c", false, "Output minimal JSON without content (requires --json)")
	searchCmd.Flags().StringVar(&searchWorkspace, "workspace", "", "Workspace name for cross-project search")
	searchCmd.Flags().StringArrayVar(&searchProjects, "project", nil, "Project name(s) to search (requires --workspace, can be repeated)")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// Validate flag combination
	if searchCompact && !searchJSON {
		return fmt.Errorf("--compact flag requires --json flag")
	}

	// Validate workspace-related flags
	if len(searchProjects) > 0 && searchWorkspace == "" {
		return fmt.Errorf("--project flag requires --workspace flag")
	}

	// Workspace mode
	if searchWorkspace != "" {
		return runWorkspaceSearch(ctx, query)
	}

	// Find project root
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize embedder
	var emb embedder.Embedder
	switch cfg.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*cfg.Embedder.Dimensions))
		}
		emb = embedder.NewOllamaEmbedder(opts...)
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*cfg.Embedder.Dimensions))
		}
		var err error
		emb, err = embedder.NewOpenAIEmbedder(opts...)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*cfg.Embedder.Dimensions))
		}
		emb = embedder.NewLMStudioEmbedder(opts...)
	default:
		return fmt.Errorf("unknown embedding provider: %s", cfg.Embedder.Provider)
	}
	defer emb.Close()

	// Initialize store
	var st store.VectorStore
	switch cfg.Store.Backend {
	case "gob":
		indexPath := config.GetIndexPath(projectRoot)
		gobStore := store.NewGOBStore(indexPath)
		if err := gobStore.Load(ctx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}
		st = gobStore
	case "postgres":
		var err error
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
	case "qdrant":
		collectionName := cfg.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = store.SanitizeCollectionName(projectRoot)
		}
		var err error
		st, err = store.NewQdrantStore(ctx, cfg.Store.Qdrant.Endpoint, cfg.Store.Qdrant.Port, cfg.Store.Qdrant.UseTLS, collectionName, cfg.Store.Qdrant.APIKey, cfg.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to qdrant: %w", err)
		}
	default:
		return fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
	defer st.Close()

	// Create searcher with boost config
	searcher := search.NewSearcher(st, emb, cfg.Search)

	// Search with boosting
	results, err := searcher.Search(ctx, query, searchLimit)
	if err != nil {
		if searchJSON {
			return outputSearchError(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results)
		}
		return outputSearchJSON(results)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Display results
	fmt.Printf("Found %d results for: %q\n\n", len(results), query)

	for i, result := range results {
		fmt.Printf("─── Result %d (score: %.4f) ───\n", i+1, result.Score)
		fmt.Printf("File: %s:%d-%d\n", result.Chunk.FilePath, result.Chunk.StartLine, result.Chunk.EndLine)
		fmt.Println()

		// Display content with line numbers
		lines := strings.Split(result.Chunk.Content, "\n")
		// Skip the "File: xxx" prefix line if present
		startIdx := 0
		if len(lines) > 0 && strings.HasPrefix(lines[0], "File: ") {
			startIdx = 2 // Skip "File: xxx" and empty line
		}

		lineNum := result.Chunk.StartLine
		for j := startIdx; j < len(lines) && j < startIdx+15; j++ {
			fmt.Printf("%4d │ %s\n", lineNum, lines[j])
			lineNum++
		}
		if len(lines)-startIdx > 15 {
			fmt.Printf("     │ ... (%d more lines)\n", len(lines)-startIdx-15)
		}
		fmt.Println()
	}

	return nil
}

// outputSearchJSON outputs results in JSON format for AI agents
func outputSearchJSON(results []store.SearchResult) error {
	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchCompactJSON outputs results in minimal JSON format (without content)
func outputSearchCompactJSON(results []store.SearchResult) error {
	jsonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultCompactJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchError outputs an error in JSON format
func outputSearchError(err error) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(map[string]string{"error": err.Error()})
	return nil
}

// SearchJSON returns results in JSON format for AI agents
func SearchJSON(projectRoot string, query string, limit int) ([]store.SearchResult, error) {
	ctx := context.Background()

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, err
	}

	var emb embedder.Embedder
	switch cfg.Embedder.Provider {
	case "ollama":
		emb = embedder.NewOllamaEmbedder(
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
		)
	case "openai":
		var err error
		emb, err = embedder.NewOpenAIEmbedder(
			embedder.WithOpenAIModel(cfg.Embedder.Model),
		)
		if err != nil {
			return nil, err
		}
	case "lmstudio":
		emb = embedder.NewLMStudioEmbedder(
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
		)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Embedder.Provider)
	}
	defer emb.Close()

	var st store.VectorStore
	switch cfg.Store.Backend {
	case "gob":
		gobStore := store.NewGOBStore(config.GetIndexPath(projectRoot))
		if err := gobStore.Load(ctx); err != nil {
			return nil, err
		}
		st = gobStore
	case "postgres":
		var err error
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.GetDimensions())
		if err != nil {
			return nil, err
		}
	}
	defer st.Close()

	// Create searcher with boost config
	searcher := search.NewSearcher(st, emb, cfg.Search)

	return searcher.Search(ctx, query, limit)
}

func init() {
	// Ensure the search command is registered
	_ = os.Getenv("GREPAI_DEBUG")
}

// runWorkspaceSearch handles workspace-level search operations
func runWorkspaceSearch(ctx context.Context, query string) error {
	// Load workspace config
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(searchWorkspace)
	if err != nil {
		return err
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Initialize embedder
	var emb embedder.Embedder
	switch ws.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(ws.Embedder.Endpoint),
			embedder.WithOllamaModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*ws.Embedder.Dimensions))
		}
		emb = embedder.NewOllamaEmbedder(opts...)
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(ws.Embedder.Model),
			embedder.WithOpenAIKey(ws.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(ws.Embedder.Endpoint),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*ws.Embedder.Dimensions))
		}
		emb, err = embedder.NewOpenAIEmbedder(opts...)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(ws.Embedder.Endpoint),
			embedder.WithLMStudioModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*ws.Embedder.Dimensions))
		}
		emb = embedder.NewLMStudioEmbedder(opts...)
	default:
		return fmt.Errorf("unknown embedding provider: %s", ws.Embedder.Provider)
	}
	defer emb.Close()

	// Initialize store
	var st store.VectorStore
	projectID := "workspace:" + ws.Name

	switch ws.Store.Backend {
	case "postgres":
		st, err = store.NewPostgresStore(ctx, ws.Store.Postgres.DSN, projectID, ws.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
	case "qdrant":
		collectionName := ws.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = "workspace_" + ws.Name
		}
		st, err = store.NewQdrantStore(ctx, ws.Store.Qdrant.Endpoint, ws.Store.Qdrant.Port, ws.Store.Qdrant.UseTLS, collectionName, ws.Store.Qdrant.APIKey, ws.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to qdrant: %w", err)
		}
	default:
		return fmt.Errorf("unsupported backend for workspace: %s", ws.Store.Backend)
	}
	defer st.Close()

	// Create searcher with default search config
	searchCfg := config.SearchConfig{
		Hybrid: config.HybridConfig{Enabled: false, K: 60},
		Boost:  config.DefaultConfig().Search.Boost,
	}
	searcher := search.NewSearcher(st, emb, searchCfg)

	// Search
	results, err := searcher.Search(ctx, query, searchLimit)
	if err != nil {
		if searchJSON {
			return outputSearchError(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by projects if specified
	// File paths are stored as: workspaceName/projectName/relativePath
	if len(searchProjects) > 0 {
		filteredResults := make([]store.SearchResult, 0)
		for _, r := range results {
			for _, projectName := range searchProjects {
				// Match workspace/project/ prefix
				expectedPrefix := ws.Name + "/" + projectName + "/"
				if strings.HasPrefix(r.Chunk.FilePath, expectedPrefix) {
					filteredResults = append(filteredResults, r)
					break
				}
			}
		}
		results = filteredResults
	}

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results)
		}
		return outputSearchJSON(results)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Display results
	fmt.Printf("Found %d results for: %q in workspace %q\n\n", len(results), query, searchWorkspace)

	for i, result := range results {
		fmt.Printf("─── Result %d (score: %.4f) ───\n", i+1, result.Score)
		fmt.Printf("File: %s:%d-%d\n", result.Chunk.FilePath, result.Chunk.StartLine, result.Chunk.EndLine)
		fmt.Println()

		// Display content with line numbers
		lines := strings.Split(result.Chunk.Content, "\n")
		startIdx := 0
		if len(lines) > 0 && strings.HasPrefix(lines[0], "File: ") {
			startIdx = 2
		}

		lineNum := result.Chunk.StartLine
		for j := startIdx; j < len(lines) && j < startIdx+15; j++ {
			fmt.Printf("%4d │ %s\n", lineNum, lines[j])
			lineNum++
		}
		if len(lines)-startIdx > 15 {
			fmt.Printf("     │ ... (%d more lines)\n", len(lines)-startIdx-15)
		}
		fmt.Println()
	}

	return nil
}
