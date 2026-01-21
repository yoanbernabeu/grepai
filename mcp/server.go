// Package mcp provides an MCP (Model Context Protocol) server for grepai.
// This allows AI agents to use grepai as a native tool.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

// Server wraps the MCP server with grepai functionality.
type Server struct {
	mcpServer   *server.MCPServer
	projectRoot string
}

// SearchResult is a lightweight struct for MCP output.
type SearchResult struct {
	FilePath  string  `json:"file_path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
	Content   string  `json:"content"`
}

// IndexStatus represents the current state of the index.
type IndexStatus struct {
	TotalFiles   int    `json:"total_files"`
	TotalChunks  int    `json:"total_chunks"`
	IndexSize    string `json:"index_size"`
	LastUpdated  string `json:"last_updated"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	SymbolsReady bool   `json:"symbols_ready"`
}

// NewServer creates a new MCP server for grepai.
func NewServer(projectRoot string) (*Server, error) {
	s := &Server{
		projectRoot: projectRoot,
	}

	// Create MCP server
	s.mcpServer = server.NewMCPServer(
		"grepai",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register tools
	s.registerTools()

	return s, nil
}

// registerTools registers all grepai tools with the MCP server.
func (s *Server) registerTools() {
	// grepai_search tool
	searchTool := mcp.NewTool("grepai_search",
		mcp.WithDescription("Semantic code search. Search your codebase using natural language queries. Returns the most relevant code chunks with file paths, line numbers, and similarity scores."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Natural language search query (e.g., 'user authentication flow', 'error handling middleware')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 10)"),
		),
	)
	s.mcpServer.AddTool(searchTool, s.handleSearch)

	// grepai_trace_callers tool
	traceCallersTool := mcp.NewTool("grepai_trace_callers",
		mcp.WithDescription("Find all functions that call the specified symbol. Useful for understanding code dependencies before modifying a function."),
		mcp.WithString("symbol",
			mcp.Required(),
			mcp.Description("Name of the function/method to find callers for"),
		),
	)
	s.mcpServer.AddTool(traceCallersTool, s.handleTraceCallers)

	// grepai_trace_callees tool
	traceCalleesTool := mcp.NewTool("grepai_trace_callees",
		mcp.WithDescription("Find all functions called by the specified symbol. Useful for understanding what a function depends on."),
		mcp.WithString("symbol",
			mcp.Required(),
			mcp.Description("Name of the function/method to find callees for"),
		),
	)
	s.mcpServer.AddTool(traceCalleesTool, s.handleTraceCallees)

	// grepai_trace_graph tool
	traceGraphTool := mcp.NewTool("grepai_trace_graph",
		mcp.WithDescription("Build a complete call graph around a symbol showing both callers and callees up to a specified depth."),
		mcp.WithString("symbol",
			mcp.Required(),
			mcp.Description("Name of the function/method to build graph for"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Maximum depth for graph traversal (default: 2)"),
		),
	)
	s.mcpServer.AddTool(traceGraphTool, s.handleTraceGraph)

	// grepai_index_status tool
	indexStatusTool := mcp.NewTool("grepai_index_status",
		mcp.WithDescription("Check the health and status of the grepai index. Returns statistics about indexed files, chunks, and configuration."),
	)
	s.mcpServer.AddTool(indexStatusTool, s.handleIndexStatus)
}

// handleSearch handles the grepai_search tool call.
func (s *Server) handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	limit := request.GetInt("limit", 10)
	if limit <= 0 {
		limit = 10
	}

	// Load configuration
	cfg, err := config.Load(s.projectRoot)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load configuration: %v", err)), nil
	}

	// Initialize embedder
	emb, err := s.createEmbedder(cfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to initialize embedder: %v", err)), nil
	}
	defer emb.Close()

	// Initialize store
	st, err := s.createStore(ctx, cfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to initialize store: %v", err)), nil
	}
	defer st.Close()

	// Create searcher and search
	searcher := search.NewSearcher(st, emb, cfg.Search)
	results, err := searcher.Search(ctx, query, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	// Convert to lightweight results
	searchResults := make([]SearchResult, len(results))
	for i, r := range results {
		searchResults[i] = SearchResult{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	// Return JSON result
	jsonBytes, err := json.MarshalIndent(searchResults, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleTraceCallers handles the grepai_trace_callers tool call.
func (s *Server) handleTraceCallers(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolName, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}

	// Initialize symbol store
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(s.projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load symbol index: %v. Run 'grepai watch' first", err)), nil
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return mcp.NewToolResultError("symbol index is empty. Run 'grepai watch' first to build the index"), nil
	}

	// Lookup symbol
	symbols, err := symbolStore.LookupSymbol(ctx, symbolName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup symbol: %v", err)), nil
	}

	if len(symbols) == 0 {
		result := trace.TraceResult{Query: symbolName, Mode: "fast"}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}

	// Find callers
	refs, err := symbolStore.LookupCallers(ctx, symbolName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup callers: %v", err)), nil
	}

	result := trace.TraceResult{
		Query:  symbolName,
		Mode:   "fast",
		Symbol: &symbols[0],
	}

	// Convert refs to CallerInfo
	for _, ref := range refs {
		callerSyms, _ := symbolStore.LookupSymbol(ctx, ref.CallerName)
		var callerSym trace.Symbol
		if len(callerSyms) > 0 {
			callerSym = callerSyms[0]
		} else {
			callerSym = trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
		}
		result.Callers = append(result.Callers, trace.CallerInfo{
			Symbol: callerSym,
			CallSite: trace.CallSite{
				File:    ref.File,
				Line:    ref.Line,
				Context: ref.Context,
			},
		})
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleTraceCallees handles the grepai_trace_callees tool call.
func (s *Server) handleTraceCallees(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolName, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}

	// Initialize symbol store
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(s.projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load symbol index: %v. Run 'grepai watch' first", err)), nil
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return mcp.NewToolResultError("symbol index is empty. Run 'grepai watch' first to build the index"), nil
	}

	// Lookup symbol
	symbols, err := symbolStore.LookupSymbol(ctx, symbolName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup symbol: %v", err)), nil
	}

	if len(symbols) == 0 {
		result := trace.TraceResult{Query: symbolName, Mode: "fast"}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}

	// Find callees
	refs, err := symbolStore.LookupCallees(ctx, symbolName, symbols[0].File)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup callees: %v", err)), nil
	}

	result := trace.TraceResult{
		Query:  symbolName,
		Mode:   "fast",
		Symbol: &symbols[0],
	}

	for _, ref := range refs {
		calleeSyms, _ := symbolStore.LookupSymbol(ctx, ref.SymbolName)
		var calleeSym trace.Symbol
		if len(calleeSyms) > 0 {
			calleeSym = calleeSyms[0]
		} else {
			calleeSym = trace.Symbol{Name: ref.SymbolName}
		}
		result.Callees = append(result.Callees, trace.CalleeInfo{
			Symbol: calleeSym,
			CallSite: trace.CallSite{
				File:    ref.File,
				Line:    ref.Line,
				Context: ref.Context,
			},
		})
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleTraceGraph handles the grepai_trace_graph tool call.
func (s *Server) handleTraceGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolName, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}

	depth := request.GetInt("depth", 2)
	if depth <= 0 {
		depth = 2
	}

	// Initialize symbol store
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(s.projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load symbol index: %v. Run 'grepai watch' first", err)), nil
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return mcp.NewToolResultError("symbol index is empty. Run 'grepai watch' first to build the index"), nil
	}

	graph, err := symbolStore.GetCallGraph(ctx, symbolName, depth)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to build call graph: %v", err)), nil
	}

	result := trace.TraceResult{
		Query: symbolName,
		Mode:  "fast",
		Graph: graph,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleIndexStatus handles the grepai_index_status tool call.
func (s *Server) handleIndexStatus(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Load configuration
	cfg, err := config.Load(s.projectRoot)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load configuration: %v", err)), nil
	}

	// Initialize store
	st, err := s.createStore(ctx, cfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to initialize store: %v", err)), nil
	}
	defer st.Close()

	// Get stats
	stats, err := st.GetStats(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get stats: %v", err)), nil
	}

	// Check symbol index
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(s.projectRoot))
	symbolsReady := false
	if err := symbolStore.Load(ctx); err == nil {
		if symbolStats, err := symbolStore.GetStats(ctx); err == nil && symbolStats.TotalSymbols > 0 {
			symbolsReady = true
		}
		symbolStore.Close()
	}

	status := IndexStatus{
		TotalFiles:   stats.TotalFiles,
		TotalChunks:  stats.TotalChunks,
		IndexSize:    formatBytes(stats.IndexSize),
		LastUpdated:  stats.LastUpdated.Format("2006-01-02 15:04:05"),
		Provider:     cfg.Embedder.Provider,
		Model:        cfg.Embedder.Model,
		SymbolsReady: symbolsReady,
	}

	jsonBytes, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal status: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// createEmbedder creates an embedder based on configuration.
func (s *Server) createEmbedder(cfg *config.Config) (embedder.Embedder, error) {
	switch cfg.Embedder.Provider {
	case "ollama":
		return embedder.NewOllamaEmbedder(
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
			embedder.WithOllamaDimensions(cfg.Embedder.Dimensions),
		), nil
	case "openai":
		return embedder.NewOpenAIEmbedder(
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOpenAIDimensions(cfg.Embedder.Dimensions),
		)
	case "lmstudio":
		return embedder.NewLMStudioEmbedder(
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
			embedder.WithLMStudioDimensions(cfg.Embedder.Dimensions),
		), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Embedder.Provider)
	}
}

// createStore creates a vector store based on configuration.
func (s *Server) createStore(ctx context.Context, cfg *config.Config) (store.VectorStore, error) {
	switch cfg.Store.Backend {
	case "gob":
		indexPath := config.GetIndexPath(s.projectRoot)
		gobStore := store.NewGOBStore(indexPath)
		if err := gobStore.Load(ctx); err != nil {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
		return gobStore, nil
	case "postgres":
		return store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, s.projectRoot, cfg.Embedder.Dimensions)
	case "qdrant":
		collectionName := cfg.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = store.SanitizeCollectionName(s.projectRoot)
		}
		return store.NewQdrantStore(ctx, cfg.Store.Qdrant.Endpoint, cfg.Store.Qdrant.Port, cfg.Store.Qdrant.UseTLS, collectionName, cfg.Store.Qdrant.APIKey, cfg.Embedder.Dimensions)
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
}

// Serve starts the MCP server using stdio transport.
func (s *Server) Serve() error {
	return server.ServeStdio(s.mcpServer)
}

// formatBytes formats bytes to human readable string.
func formatBytes(b int64) string {
	if b == 0 {
		return "N/A"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
