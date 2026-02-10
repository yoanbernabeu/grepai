// Package mcp provides an MCP (Model Context Protocol) server for grepai.
// This allows AI agents to use grepai as a native tool.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/alpkeskin/gotoon"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

// Server wraps the MCP server with grepai functionality.
type Server struct {
	mcpServer     *server.MCPServer
	projectRoot   string
	workspaceName string // non-empty when started via --workspace or auto-detect
}

// SearchResult is a lightweight struct for MCP output.
type SearchResult struct {
	FilePath    string  `json:"file_path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Score       float32 `json:"score"`
	Content     string  `json:"content"`
	FeaturePath string  `json:"feature_path,omitempty"`
	SymbolName  string  `json:"symbol_name,omitempty"`
}

// SearchResultCompact is a minimal struct for compact output (no content field).
type SearchResultCompact struct {
	FilePath    string  `json:"file_path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Score       float32 `json:"score"`
	FeaturePath string  `json:"feature_path,omitempty"`
	SymbolName  string  `json:"symbol_name,omitempty"`
}

// CallSiteCompact is a minimal struct for compact output (no context field).
type CallSiteCompact struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// CallerInfoCompact is a compact version of trace.CallerInfo for compact output.
type CallerInfoCompact struct {
	Symbol   trace.Symbol    `json:"symbol"`
	CallSite CallSiteCompact `json:"call_site"`
}

// CalleeInfoCompact is a compact version of trace.CalleeInfo for compact output.
type CalleeInfoCompact struct {
	Symbol   trace.Symbol    `json:"symbol"`
	CallSite CallSiteCompact `json:"call_site"`
}

// CallEdgeCompact is a compact version of trace.CallEdge for compact output.
type CallEdgeCompact struct {
	CallerName string `json:"caller_name"`
	CalleeName string `json:"callee_name"`
	File       string `json:"file"`
	Line       int    `json:"line"`
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
	RPGEnabled   bool   `json:"rpg_enabled"`
	RPGNodes     int    `json:"rpg_nodes,omitempty"`
	RPGEdges     int    `json:"rpg_edges,omitempty"`
}

// encodeOutput encodes data in the specified format (json or toon).
func encodeOutput(data any, format string) (string, error) {
	switch format {
	case "toon":
		return gotoon.Encode(data)
	default: // "json"
		jsonBytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}
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

// NewServerWithWorkspace creates a new MCP server with workspace context.
// projectRoot may be empty when in workspace-only mode (no local .grepai/).
func NewServerWithWorkspace(projectRoot, workspaceName string) (*Server, error) {
	s := &Server{
		projectRoot:   projectRoot,
		workspaceName: workspaceName,
	}

	s.mcpServer = server.NewMCPServer(
		"grepai",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

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
		mcp.WithBoolean("compact",
			mcp.Description("Return minimal output without content (default: false)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
		mcp.WithString("workspace",
			mcp.Description("Workspace name for cross-project search (optional)"),
		),
		mcp.WithString("projects",
			mcp.Description("Comma-separated list of project names to search within workspace (requires workspace)"),
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
		mcp.WithBoolean("compact",
			mcp.Description("Return minimal output without context (default: false)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
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
		mcp.WithBoolean("compact",
			mcp.Description("Return minimal output without context (default: false)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
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
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
	)
	s.mcpServer.AddTool(traceGraphTool, s.handleTraceGraph)

	// grepai_index_status tool
	indexStatusTool := mcp.NewTool("grepai_index_status",
		mcp.WithDescription("Check the health and status of the grepai index. Returns statistics about indexed files, chunks, and configuration."),
		mcp.WithBoolean("verbose", mcp.Description("Include additional debug details when available (optional).")),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
	)
	s.mcpServer.AddTool(indexStatusTool, s.handleIndexStatus)

	// grepai_rpg_search tool
	rpgSearchTool := mcp.NewTool("grepai_rpg_search",
		mcp.WithDescription("Search RPG nodes using Jaccard-based semantic matching with scope and kind filtering."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Natural language or feature query to search for"),
		),
		mcp.WithString("scope",
			mcp.Description("Area/category path to narrow search (e.g., 'cli', 'rpg/query')"),
		),
		mcp.WithString("kinds",
			mcp.Description("Comma-separated node kinds to filter: area, category, subcategory, file, symbol, chunk (default: symbol)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 10)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
	)
	s.mcpServer.AddTool(rpgSearchTool, s.handleRPGSearch)

	// grepai_rpg_fetch tool
	rpgFetchTool := mcp.NewTool("grepai_rpg_fetch",
		mcp.WithDescription("Fetch detailed information about a specific RPG node including hierarchy, edges, and context."),
		mcp.WithString("node_id",
			mcp.Required(),
			mcp.Description("Node ID to fetch (e.g., 'sym:main.go:HandleRequest')"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
	)
	s.mcpServer.AddTool(rpgFetchTool, s.handleRPGFetch)

	// grepai_rpg_explore tool
	rpgExploreTool := mcp.NewTool("grepai_rpg_explore",
		mcp.WithDescription("Explore the RPG graph using BFS traversal from a starting node with configurable depth and edge type filtering."),
		mcp.WithString("start_node_id",
			mcp.Required(),
			mcp.Description("Starting node ID for graph traversal"),
		),
		mcp.WithString("direction",
			mcp.Description("Traversal direction: 'forward', 'reverse', or 'both' (default: 'both')"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Maximum BFS depth (default: 2)"),
		),
		mcp.WithString("edge_types",
			mcp.Description("Comma-separated edge types to follow: feature_parent, contains, invokes, imports, maps_to_chunk, semantic_sim"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum nodes to return (default: 100)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default) or 'toon' (token-efficient)"),
		),
	)
	s.mcpServer.AddTool(rpgExploreTool, s.handleRPGExplore)
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

	compact := request.GetBool("compact", false)
	format := request.GetString("format", "json")
	workspace := request.GetString("workspace", "")
	projects := request.GetString("projects", "")

	// Auto-inject workspace when server is in workspace mode
	if workspace == "" && s.workspaceName != "" {
		workspace = s.workspaceName
	}

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Workspace mode
	if workspace != "" {
		return s.handleWorkspaceSearch(ctx, query, limit, compact, format, workspace, projects)
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

	// RPG enrichment
	type rpgInfo struct {
		featurePath string
		symbolName  string
	}
	rpgData := make(map[int]rpgInfo)
	rpgSt, qe, rpgErr := s.tryLoadRPG(ctx)
	if rpgErr != nil {
		log.Printf("Warning: RPG enrichment unavailable: %v", rpgErr)
	}
	if rpgSt != nil && qe != nil {
		defer rpgSt.Close()
		graph := rpgSt.GetGraph()
		for i, r := range results {
			nodes := graph.GetNodesByFile(r.Chunk.FilePath)
			for _, n := range nodes {
				if n.Kind == rpg.KindSymbol && n.StartLine <= r.Chunk.EndLine && r.Chunk.StartLine <= n.EndLine {
					fetchRes, err := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: n.ID})
					if err == nil && fetchRes != nil {
						rpgData[i] = rpgInfo{featurePath: fetchRes.FeaturePath, symbolName: n.SymbolName}
					}
					break
				}
			}
		}
	}

	var data any
	if compact {
		searchResultsCompact := make([]SearchResultCompact, len(results))
		for i, r := range results {
			searchResultsCompact[i] = SearchResultCompact{
				FilePath:  r.Chunk.FilePath,
				StartLine: r.Chunk.StartLine,
				EndLine:   r.Chunk.EndLine,
				Score:     r.Score,
			}
			if info, ok := rpgData[i]; ok {
				searchResultsCompact[i].FeaturePath = info.featurePath
				searchResultsCompact[i].SymbolName = info.symbolName
			}
		}
		data = searchResultsCompact
	} else {
		searchResults := make([]SearchResult, len(results))
		for i, r := range results {
			searchResults[i] = SearchResult{
				FilePath:  r.Chunk.FilePath,
				StartLine: r.Chunk.StartLine,
				EndLine:   r.Chunk.EndLine,
				Score:     r.Score,
				Content:   r.Chunk.Content,
			}
			if info, ok := rpgData[i]; ok {
				searchResults[i].FeaturePath = info.featurePath
				searchResults[i].SymbolName = info.symbolName
			}
		}
		data = searchResults
	}

	output, err := encodeOutput(data, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleWorkspaceSearch handles workspace-level search via MCP.
func (s *Server) handleWorkspaceSearch(ctx context.Context, query string, limit int, compact bool, format, workspaceName, projectsStr string) (*mcp.CallToolResult, error) {
	// Load workspace config
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load workspace config: %v", err)), nil
	}
	if wsCfg == nil {
		return mcp.NewToolResultError("no workspaces configured"), nil
	}

	ws, err := wsCfg.GetWorkspace(workspaceName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("workspace not found: %v", err)), nil
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Initialize embedder
	emb, err := s.createWorkspaceEmbedder(ws)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to initialize embedder: %v", err)), nil
	}
	defer emb.Close()

	// Initialize store
	st, err := s.createWorkspaceStore(ctx, ws)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to initialize store: %v", err)), nil
	}
	defer st.Close()

	// Create searcher with default config
	searchCfg := config.SearchConfig{
		Hybrid: config.HybridConfig{Enabled: false, K: 60},
		Boost:  config.DefaultConfig().Search.Boost,
	}
	searcher := search.NewSearcher(st, emb, searchCfg)

	// Search
	results, err := searcher.Search(ctx, query, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	// Filter by projects if specified
	// File paths are stored as: workspaceName/projectName/relativePath
	if projectsStr != "" {
		projectNames := strings.Split(projectsStr, ",")
		filteredResults := make([]store.SearchResult, 0)
		for _, r := range results {
			for _, projectName := range projectNames {
				projectName = strings.TrimSpace(projectName)
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

	var data any
	if compact {
		searchResultsCompact := make([]SearchResultCompact, len(results))
		for i, r := range results {
			searchResultsCompact[i] = SearchResultCompact{
				FilePath:  r.Chunk.FilePath,
				StartLine: r.Chunk.StartLine,
				EndLine:   r.Chunk.EndLine,
				Score:     r.Score,
			}
		}
		data = searchResultsCompact
	} else {
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
		data = searchResults
	}

	output, err := encodeOutput(data, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// createWorkspaceEmbedder creates an embedder based on workspace configuration.
func (s *Server) createWorkspaceEmbedder(ws *config.Workspace) (embedder.Embedder, error) {
	switch ws.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(ws.Embedder.Endpoint),
			embedder.WithOllamaModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*ws.Embedder.Dimensions))
		}
		return embedder.NewOllamaEmbedder(opts...), nil
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(ws.Embedder.Model),
			embedder.WithOpenAIKey(ws.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(ws.Embedder.Endpoint),
			embedder.WithOpenAIParallelism(ws.Embedder.Parallelism),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*ws.Embedder.Dimensions))
		}
		return embedder.NewOpenAIEmbedder(opts...)
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(ws.Embedder.Endpoint),
			embedder.WithLMStudioModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*ws.Embedder.Dimensions))
		}
		return embedder.NewLMStudioEmbedder(opts...), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", ws.Embedder.Provider)
	}
}

// createWorkspaceStore creates a vector store based on workspace configuration.
func (s *Server) createWorkspaceStore(ctx context.Context, ws *config.Workspace) (store.VectorStore, error) {
	projectID := "workspace:" + ws.Name

	switch ws.Store.Backend {
	case "postgres":
		return store.NewPostgresStore(ctx, ws.Store.Postgres.DSN, projectID, ws.Embedder.GetDimensions())
	case "qdrant":
		collectionName := ws.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = "workspace_" + ws.Name
		}
		return store.NewQdrantStore(ctx, ws.Store.Qdrant.Endpoint, ws.Store.Qdrant.Port, ws.Store.Qdrant.UseTLS, collectionName, ws.Store.Qdrant.APIKey, ws.Embedder.GetDimensions())
	default:
		return nil, fmt.Errorf("unsupported backend for workspace: %s", ws.Store.Backend)
	}
}

// enrichTraceSymbols enriches trace symbols with RPG feature paths.
// It loads the RPG store once and enriches all provided symbols in one pass.
func (s *Server) enrichTraceSymbols(ctx context.Context, symbols ...*trace.Symbol) {
	if s.projectRoot == "" {
		return
	}
	cfg, err := config.Load(s.projectRoot)
	if err != nil || !cfg.RPG.Enabled {
		return
	}
	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(s.projectRoot))
	if err := rpgStore.Load(ctx); err != nil {
		log.Printf("Warning: RPG enrichment unavailable for trace: %v", err)
		return
	}
	defer rpgStore.Close()

	graph := rpgStore.GetGraph()
	qe := rpg.NewQueryEngine(graph)

	for _, sym := range symbols {
		if sym == nil || sym.File == "" {
			continue
		}
		nodes := graph.GetNodesByFile(sym.File)
		for _, n := range nodes {
			if n.Kind == rpg.KindSymbol && n.SymbolName == sym.Name {
				fetchResult, fetchErr := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: n.ID})
				if fetchErr == nil && fetchResult != nil {
					sym.FeaturePath = fetchResult.FeaturePath
				}
				break
			}
		}
	}
}

// handleTraceCallers handles the grepai_trace_callers tool call.
func (s *Server) handleTraceCallers(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolName, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}

	compact := request.GetBool("compact", false)
	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Trace requires local project context
	if s.projectRoot == "" {
		return mcp.NewToolResultError("trace requires a project context; start mcp-serve from a project directory or use grepai_search with workspace parameter instead"), nil
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
		output, _ := encodeOutput(result, format)
		return mcp.NewToolResultText(output), nil
	}

	// Find callers
	refs, err := symbolStore.LookupCallers(ctx, symbolName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup callers: %v", err)), nil
	}

	var data any
	if compact {
		resultCompact := struct {
			Query   string              `json:"query"`
			Mode    string              `json:"mode"`
			Symbol  *trace.Symbol       `json:"symbol,omitempty"`
			Callers []CallerInfoCompact `json:"callers,omitempty"`
		}{
			Query:   symbolName,
			Mode:    "fast",
			Symbol:  &symbols[0],
			Callers: make([]CallerInfoCompact, 0, len(refs)),
		}

		for _, ref := range refs {
			callerSyms, _ := symbolStore.LookupSymbol(ctx, ref.CallerName)
			var callerSym trace.Symbol
			if len(callerSyms) > 0 {
				callerSym = callerSyms[0]
			} else {
				callerSym = trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
			}
			resultCompact.Callers = append(resultCompact.Callers, CallerInfoCompact{
				Symbol: callerSym,
				CallSite: CallSiteCompact{
					File: ref.File,
					Line: ref.Line,
				},
			})
		}

		// Enrich with RPG
		symPtrs := []*trace.Symbol{resultCompact.Symbol}
		for i := range resultCompact.Callers {
			symPtrs = append(symPtrs, &resultCompact.Callers[i].Symbol)
		}
		s.enrichTraceSymbols(ctx, symPtrs...)

		data = resultCompact
	} else {
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

		// Enrich with RPG
		symPtrs := []*trace.Symbol{result.Symbol}
		for i := range result.Callers {
			symPtrs = append(symPtrs, &result.Callers[i].Symbol)
		}
		s.enrichTraceSymbols(ctx, symPtrs...)

		data = result
	}

	output, err := encodeOutput(data, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleTraceCallees handles the grepai_trace_callees tool call.
func (s *Server) handleTraceCallees(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolName, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}

	compact := request.GetBool("compact", false)
	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Trace requires local project context
	if s.projectRoot == "" {
		return mcp.NewToolResultError("trace requires a project context; start mcp-serve from a project directory or use grepai_search with workspace parameter instead"), nil
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
		output, _ := encodeOutput(result, format)
		return mcp.NewToolResultText(output), nil
	}

	// Find callees
	refs, err := symbolStore.LookupCallees(ctx, symbolName, symbols[0].File)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to lookup callees: %v", err)), nil
	}

	var data any
	if compact {
		resultCompact := struct {
			Query   string              `json:"query"`
			Mode    string              `json:"mode"`
			Symbol  *trace.Symbol       `json:"symbol,omitempty"`
			Callees []CalleeInfoCompact `json:"callees,omitempty"`
		}{
			Query:   symbolName,
			Mode:    "fast",
			Symbol:  &symbols[0],
			Callees: make([]CalleeInfoCompact, 0, len(refs)),
		}

		for _, ref := range refs {
			calleeSyms, _ := symbolStore.LookupSymbol(ctx, ref.SymbolName)
			var calleeSym trace.Symbol
			if len(calleeSyms) > 0 {
				calleeSym = calleeSyms[0]
			} else {
				calleeSym = trace.Symbol{Name: ref.SymbolName}
			}
			resultCompact.Callees = append(resultCompact.Callees, CalleeInfoCompact{
				Symbol: calleeSym,
				CallSite: CallSiteCompact{
					File: ref.File,
					Line: ref.Line,
				},
			})
		}

		// Enrich with RPG
		symPtrs := []*trace.Symbol{resultCompact.Symbol}
		for i := range resultCompact.Callees {
			symPtrs = append(symPtrs, &resultCompact.Callees[i].Symbol)
		}
		s.enrichTraceSymbols(ctx, symPtrs...)

		data = resultCompact
	} else {
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

		// Enrich with RPG
		symPtrs := []*trace.Symbol{result.Symbol}
		for i := range result.Callees {
			symPtrs = append(symPtrs, &result.Callees[i].Symbol)
		}
		s.enrichTraceSymbols(ctx, symPtrs...)

		data = result
	}

	output, err := encodeOutput(data, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
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

	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Trace requires local project context
	if s.projectRoot == "" {
		return mcp.NewToolResultError("trace requires a project context; start mcp-serve from a project directory or use grepai_search with workspace parameter instead"), nil
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

	// Enrich graph nodes with RPG
	if result.Graph != nil {
		// Collect symbols as pointers for enrichment, paired with their map keys
		type symEntry struct {
			name string
			sym  trace.Symbol
		}
		entries := make([]symEntry, 0, len(result.Graph.Nodes))
		symPtrs := make([]*trace.Symbol, 0, len(result.Graph.Nodes))
		for name, sym := range result.Graph.Nodes {
			entries = append(entries, symEntry{name: name, sym: sym})
			symPtrs = append(symPtrs, &entries[len(entries)-1].sym)
		}
		s.enrichTraceSymbols(ctx, symPtrs...)
		for _, e := range entries {
			result.Graph.Nodes[e.name] = e.sym
		}
	}

	output, err := encodeOutput(result, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleIndexStatus handles the grepai_index_status tool call.
func (s *Server) handleIndexStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Index status requires local project context
	if s.projectRoot == "" {
		return mcp.NewToolResultError("index status requires a project context; start mcp-serve from a project directory"), nil
	}

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

	// Check RPG status
	rpgSt, _, _ := s.tryLoadRPG(ctx)
	if rpgSt != nil {
		status.RPGEnabled = true
		rpgStats := rpgSt.GetGraph().Stats()
		status.RPGNodes = rpgStats.TotalNodes
		status.RPGEdges = rpgStats.TotalEdges
		rpgSt.Close()
	}

	output, err := encodeOutput(status, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode status: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// createEmbedder creates an embedder based on configuration.
func (s *Server) createEmbedder(cfg *config.Config) (embedder.Embedder, error) {
	switch cfg.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*cfg.Embedder.Dimensions))
		}
		return embedder.NewOllamaEmbedder(opts...), nil
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOpenAIParallelism(cfg.Embedder.Parallelism),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*cfg.Embedder.Dimensions))
		}
		return embedder.NewOpenAIEmbedder(opts...)
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*cfg.Embedder.Dimensions))
		}
		return embedder.NewLMStudioEmbedder(opts...), nil
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
		return store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, s.projectRoot, cfg.Embedder.GetDimensions())
	case "qdrant":
		collectionName := cfg.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = store.SanitizeCollectionName(s.projectRoot)
		}
		return store.NewQdrantStore(ctx, cfg.Store.Qdrant.Endpoint, cfg.Store.Qdrant.Port, cfg.Store.Qdrant.UseTLS, collectionName, cfg.Store.Qdrant.APIKey, cfg.Embedder.GetDimensions())
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
}

// Serve starts the MCP server using stdio transport.
func (s *Server) Serve() error {
	// Create stdio server with title fix wrapper
	stdioServer := server.NewStdioServer(s.mcpServer)

	// Wrap stdout to intercept and fix responses
	fixedStdout := &titleFixWriter{Writer: os.Stdout}

	// Start listening with fixed stdout
	ctx := context.Background()
	return stdioServer.Listen(ctx, os.Stdin, fixedStdout)
}

// titleFixWriter wraps io.Writer to fix tool titles in responses
type titleFixWriter struct {
	Writer io.Writer
}

func (w *titleFixWriter) Write(p []byte) (n int, err error) {
	// Try to parse as JSON
	var data map[string]interface{}
	if err2 := json.Unmarshal(p, &data); err2 != nil {
		// Not valid JSON, write as-is
		return w.Writer.Write(p)
	}

	// Check if this is a tools/list response
	if result, ok := data["result"].(map[string]interface{}); ok {
		if tools, ok := result["tools"].([]interface{}); ok {
			// Fix each tool
			for _, toolIf := range tools {
				tool, ok := toolIf.(map[string]interface{})
				if !ok {
					continue
				}

				// 1. Move title from annotations to root
				if annotations, ok := tool["annotations"].(map[string]interface{}); ok {
					if title, ok := annotations["title"].(string); ok {
						tool["title"] = title
						delete(annotations, "title")
					}
				}

				// 2. Add $schema to inputSchema (required by Windsurf)
				if inputSchema, ok := tool["inputSchema"].(map[string]interface{}); ok {
					if _, hasSchema := inputSchema["$schema"]; !hasSchema {
						inputSchema["$schema"] = "http://json-schema.org/draft-07/schema#"
					}
				}
			}

			// Marshal back and write with newline
			fixed, _ := json.Marshal(data)
			fixed = append(fixed, '\n')
			return w.Writer.Write(fixed)
		}
	}

	// Write original data
	return w.Writer.Write(p)
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

// tryLoadRPG attempts to load the RPG store. Returns nil values if RPG is disabled or unavailable.
func (s *Server) tryLoadRPG(ctx context.Context) (rpg.RPGStore, *rpg.QueryEngine, error) {
	if s.projectRoot == "" {
		return nil, nil, nil
	}
	cfg, err := config.Load(s.projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	if !cfg.RPG.Enabled {
		return nil, nil, nil
	}
	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(s.projectRoot))
	if err := rpgStore.Load(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to load RPG store: %w", err)
	}
	graph := rpgStore.GetGraph()
	if graph.Stats().TotalNodes == 0 {
		rpgStore.Close()
		return nil, nil, nil
	}
	qe := rpg.NewQueryEngine(graph)
	return rpgStore, qe, nil
}

// handleRPGSearch handles the grepai_rpg_search tool call.
func (s *Server) handleRPGSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	scope := request.GetString("scope", "")
	kindsStr := request.GetString("kinds", "")
	limit := request.GetInt("limit", 10)
	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Load RPG
	rpgSt, qe, _ := s.tryLoadRPG(ctx)
	if rpgSt == nil {
		return mcp.NewToolResultError("RPG is not enabled or index is empty"), nil
	}
	defer rpgSt.Close()

	// Parse kinds
	var kinds []rpg.NodeKind
	if kindsStr != "" {
		kindParts := strings.Split(kindsStr, ",")
		for _, k := range kindParts {
			k = strings.TrimSpace(k)
			switch k {
			case "area":
				kinds = append(kinds, rpg.KindArea)
			case "category":
				kinds = append(kinds, rpg.KindCategory)
			case "subcategory":
				kinds = append(kinds, rpg.KindSubcategory)
			case "file":
				kinds = append(kinds, rpg.KindFile)
			case "symbol":
				kinds = append(kinds, rpg.KindSymbol)
			case "chunk":
				kinds = append(kinds, rpg.KindChunk)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid kind: %s", k)), nil
			}
		}
	}

	// Build request
	req := rpg.SearchNodeRequest{
		Query: query,
		Scope: scope,
		Kinds: kinds,
		Limit: limit,
	}

	// Execute search
	results, err := qe.SearchNode(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	// Encode output
	output, err := encodeOutput(results, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleRPGFetch handles the grepai_rpg_fetch tool call.
func (s *Server) handleRPGFetch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	nodeID, err := request.RequireString("node_id")
	if err != nil {
		return mcp.NewToolResultError("node_id parameter is required"), nil
	}

	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Load RPG
	rpgSt, qe, _ := s.tryLoadRPG(ctx)
	if rpgSt == nil {
		return mcp.NewToolResultError("RPG is not enabled or index is empty"), nil
	}
	defer rpgSt.Close()

	// Fetch node
	result, err := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: nodeID})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("fetch failed: %v", err)), nil
	}

	if result == nil {
		return mcp.NewToolResultError(fmt.Sprintf("node not found: %s", nodeID)), nil
	}

	// Encode output
	output, err := encodeOutput(result, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode result: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleRPGExplore handles the grepai_rpg_explore tool call.
func (s *Server) handleRPGExplore(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startNodeID, err := request.RequireString("start_node_id")
	if err != nil {
		return mcp.NewToolResultError("start_node_id parameter is required"), nil
	}

	direction := request.GetString("direction", "both")
	depth := request.GetInt("depth", 2)
	edgeTypesStr := request.GetString("edge_types", "")
	limit := request.GetInt("limit", 100)
	format := request.GetString("format", "json")

	// Validate format
	if format != "json" && format != "toon" {
		return mcp.NewToolResultError("format must be 'json' or 'toon'"), nil
	}

	// Validate direction
	if direction != "forward" && direction != "reverse" && direction != "both" {
		return mcp.NewToolResultError("direction must be 'forward', 'reverse', or 'both'"), nil
	}

	// Load RPG
	rpgSt, qe, _ := s.tryLoadRPG(ctx)
	if rpgSt == nil {
		return mcp.NewToolResultError("RPG is not enabled or index is empty"), nil
	}
	defer rpgSt.Close()

	// Parse edge types
	var edgeTypes []rpg.EdgeType
	if edgeTypesStr != "" {
		edgeParts := strings.Split(edgeTypesStr, ",")
		for _, et := range edgeParts {
			et = strings.TrimSpace(et)
			switch et {
			case "feature_parent":
				edgeTypes = append(edgeTypes, rpg.EdgeFeatureParent)
			case "contains":
				edgeTypes = append(edgeTypes, rpg.EdgeContains)
			case "invokes":
				edgeTypes = append(edgeTypes, rpg.EdgeInvokes)
			case "imports":
				edgeTypes = append(edgeTypes, rpg.EdgeImports)
			case "maps_to_chunk":
				edgeTypes = append(edgeTypes, rpg.EdgeMapsToChunk)
			case "semantic_sim":
				edgeTypes = append(edgeTypes, rpg.EdgeSemanticSim)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid edge type: %s", et)), nil
			}
		}
	}

	// Build request
	req := rpg.ExploreRequest{
		StartNodeID: startNodeID,
		Direction:   direction,
		Depth:       depth,
		EdgeTypes:   edgeTypes,
		Limit:       limit,
	}

	// Execute exploration
	result, err := qe.Explore(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("explore failed: %v", err)), nil
	}

	if result == nil {
		return mcp.NewToolResultError(fmt.Sprintf("start node not found: %s", startNodeID)), nil
	}

	// Encode output
	output, err := encodeOutput(result, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode result: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}
