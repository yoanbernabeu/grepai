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
	searchLimit   int
	searchJSON    bool
	searchCompact bool
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
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// Validate flag combination
	if searchCompact && !searchJSON {
		return fmt.Errorf("--compact flag requires --json flag")
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
		emb = embedder.NewOllamaEmbedder(
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
			embedder.WithOllamaDimensions(cfg.Embedder.Dimensions),
		)
	case "openai":
		var err error
		emb, err = embedder.NewOpenAIEmbedder(
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOpenAIDimensions(cfg.Embedder.Dimensions),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		emb = embedder.NewLMStudioEmbedder(
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
			embedder.WithLMStudioDimensions(cfg.Embedder.Dimensions),
		)
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
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.Dimensions)
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
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
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.Dimensions)
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
