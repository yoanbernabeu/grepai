package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alpkeskin/gotoon"
	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
)

var (
	searchLimit     int
	searchJSON      bool
	searchTOON      bool
	searchCompact   bool
	searchWorkspace string
	searchProjects  []string
	searchPath      string
)

// SearchResultJSON is a lightweight struct for JSON output (excludes vector, hash, updated_at)
type SearchResultJSON struct {
	FilePath    string  `json:"file_path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Score       float32 `json:"score"`
	Content     string  `json:"content"`
	FeaturePath string  `json:"feature_path,omitempty"`
	SymbolName  string  `json:"symbol_name,omitempty"`
}

// SearchResultCompactJSON is a minimal struct for compact JSON output (no content field)
type SearchResultCompactJSON struct {
	FilePath    string  `json:"file_path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Score       float32 `json:"score"`
	FeaturePath string  `json:"feature_path,omitempty"`
	SymbolName  string  `json:"symbol_name,omitempty"`
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
	searchCmd.Flags().BoolVarP(&searchTOON, "toon", "t", false, "Output results in TOON format (token-efficient for AI agents)")
	searchCmd.Flags().BoolVarP(&searchCompact, "compact", "c", false, "Output minimal format without content (requires --json or --toon)")
	searchCmd.Flags().StringVar(&searchWorkspace, "workspace", "", "Workspace name for cross-project search")
	searchCmd.Flags().StringArrayVar(&searchProjects, "project", nil, "Project name(s) to search (requires --workspace, can be repeated)")
	searchCmd.Flags().StringVar(&searchPath, "path", "", "Path prefix to filter search results")
	searchCmd.MarkFlagsMutuallyExclusive("json", "toon")
}

// rpgEnrichment holds RPG context for a search result
type rpgEnrichment struct {
	FeaturePath string
	SymbolName  string
}

// enrichWithRPG enriches search results with RPG feature paths and symbol names
func enrichWithRPG(projectRoot string, cfg *config.Config, results []store.SearchResult) []rpgEnrichment {
	enrichments := make([]rpgEnrichment, len(results))
	if !cfg.RPG.Enabled {
		return enrichments
	}

	ctx := context.Background()
	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
	if err := rpgStore.Load(ctx); err != nil {
		// Silently fail - RPG enrichment is best-effort
		return enrichments
	}
	defer rpgStore.Close()

	graph := rpgStore.GetGraph()
	qe := rpg.NewQueryEngine(graph)

	for i, r := range results {
		nodes := graph.GetNodesByFile(r.Chunk.FilePath)
		for _, n := range nodes {
			// Find symbol node that overlaps with the chunk's line range
			if n.Kind == rpg.KindSymbol && n.StartLine <= r.Chunk.EndLine && r.Chunk.StartLine <= n.EndLine {
				// Found overlapping symbol node
				fetchResult, err := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: n.ID})
				if err == nil && fetchResult != nil {
					enrichments[i].FeaturePath = fetchResult.FeaturePath
					enrichments[i].SymbolName = n.SymbolName
				}
				break
			}
		}
	}

	return enrichments
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// Validate flag combination
	if searchCompact && !searchJSON && !searchTOON {
		return fmt.Errorf("--compact flag requires --json or --toon flag")
	}

	// Validate workspace-related flags
	if len(searchProjects) > 0 && searchWorkspace == "" {
		return fmt.Errorf("--project flag requires --workspace flag")
	}

	// Workspace mode
	if searchWorkspace != "" {
		return runWorkspaceSearch(ctx, query, searchProjects, searchPath)
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
	emb, err := embedder.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize embedder: %w", err)
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

	normalizedPath, err := search.NormalizeProjectPathPrefix(searchPath, projectRoot)
	if err != nil {
		return fmt.Errorf("invalid --path value: %w", err)
	}

	// Search with boosting
	results, err := searcher.Search(ctx, query, searchLimit, normalizedPath)
	if err != nil {
		if searchJSON {
			return outputSearchErrorJSON(err)
		}
		if searchTOON {
			return outputSearchErrorTOON(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// Enrich results with RPG context
	enrichments := enrichWithRPG(projectRoot, cfg, results)

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results, enrichments)
		}
		return outputSearchJSON(results, enrichments)
	}

	// TOON output mode
	if searchTOON {
		if searchCompact {
			return outputSearchCompactTOON(results, enrichments)
		}
		return outputSearchTOON(results, enrichments)
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
func outputSearchJSON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			Content:     r.Chunk.Content,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchCompactJSON outputs results in minimal JSON format (without content)
func outputSearchCompactJSON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	jsonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultCompactJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchErrorJSON outputs an error in JSON format
func outputSearchErrorJSON(err error) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(map[string]string{"error": err.Error()})
	return nil
}

// outputSearchTOON outputs results in TOON format for AI agents
func outputSearchTOON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	toonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			Content:     r.Chunk.Content,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		return fmt.Errorf("failed to encode TOON: %w", err)
	}
	fmt.Println(output)
	return nil
}

// outputSearchCompactTOON outputs results in minimal TOON format (without content)
func outputSearchCompactTOON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	toonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultCompactJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		return fmt.Errorf("failed to encode TOON: %w", err)
	}
	fmt.Println(output)
	return nil
}

// outputSearchErrorTOON outputs an error in TOON format
func outputSearchErrorTOON(err error) error {
	output, encErr := gotoon.Encode(map[string]string{"error": err.Error()})
	if encErr != nil {
		return fmt.Errorf("failed to encode TOON error: %w", encErr)
	}
	fmt.Println(output)
	return nil
}

// SearchJSON returns results in JSON format for AI agents
func SearchJSON(projectRoot string, query string, limit int) ([]store.SearchResult, error) {
	ctx := context.Background()

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, err
	}

	emb, err := embedder.NewFromConfig(cfg)
	if err != nil {
		return nil, err
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

	return searcher.Search(ctx, query, limit, "")
}

func init() {
	// Ensure the search command is registered
	_ = os.Getenv("GREPAI_DEBUG")
}

// runWorkspaceSearch handles workspace-level search operations
func runWorkspaceSearch(ctx context.Context, query string, projects []string, pathOpt string) error {
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

	normalizedPath, resolvedProjects, err := search.NormalizeWorkspacePathPrefix(pathOpt, ws, projects)
	if err != nil {
		return fmt.Errorf("invalid --path value: %w", err)
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Initialize embedder
	emb, err := embedder.NewFromWorkspaceConfig(ws)
	if err != nil {
		return fmt.Errorf("failed to initialize embedder: %w", err)
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

	// Construct full path prefix for database query
	// Database stores paths as: workspaceName/projectName/relativePath
	// When a single project is specified, include it in the path prefix to push filtering to database level
	fullPathPrefix := ws.Name + "/"
	if len(resolvedProjects) == 1 {
		// If exactly one project specified, include it in the path prefix for database-level filtering
		// This ensures file_path LIKE 'workspace/project/%' filter is applied
		fullPathPrefix += resolvedProjects[0] + "/"
	}
	if normalizedPath != "" {
		fullPathPrefix += normalizedPath
	}

	// Search
	results, err := searcher.Search(ctx, query, searchLimit, fullPathPrefix)
	if err != nil {
		if searchJSON {
			return outputSearchErrorJSON(err)
		}
		if searchTOON {
			return outputSearchErrorTOON(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by projects if specified (additional client-side filtering for multiple projects)
	// File paths are stored as: workspaceName/projectName/relativePath
	if len(resolvedProjects) > 0 {
		filteredResults := make([]store.SearchResult, 0)
		for _, r := range results {
			for _, projectName := range resolvedProjects {
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

	// Workspace mode doesn't have RPG enrichment (no single projectRoot)
	enrichments := make([]rpgEnrichment, len(results))

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results, enrichments)
		}
		return outputSearchJSON(results, enrichments)
	}

	// TOON output mode
	if searchTOON {
		if searchCompact {
			return outputSearchCompactTOON(results, enrichments)
		}
		return outputSearchTOON(results, enrichments)
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
