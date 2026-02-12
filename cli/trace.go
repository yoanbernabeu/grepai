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
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/trace"
)

var (
	traceMode      string
	traceDepth     int
	traceJSON      bool
	traceTOON      bool
	traceWorkspace string
	traceProject   string
)

var traceCmd = &cobra.Command{
	Use:   "trace <subcommand> <symbol>",
	Short: "Trace symbol callers and callees",
	Long: `Trace command helps you understand code dependencies by finding:
- callers: functions that call the specified symbol
- callees: functions that the specified symbol calls
- graph: full call graph visualization

Examples:
  grepai trace callers "Login"
  grepai trace callees "HandleRequest" --mode precise
  grepai trace graph "ProcessOrder" --depth 3 --json`,
}

var traceCallersCmd = &cobra.Command{
	Use:   "callers <symbol>",
	Short: "Find all functions that call the specified symbol",
	Long: `Find all functions that call the specified symbol.

Examples:
  grepai trace callers "Login"
  grepai trace callers "HandleRequest" --json
  grepai trace callers "ProcessOrder" --mode precise`,
	Args: cobra.ExactArgs(1),
	RunE: runTraceCallers,
}

var traceCalleesCmd = &cobra.Command{
	Use:   "callees <symbol>",
	Short: "Find all functions called by the specified symbol",
	Long: `Find all functions called by the specified symbol.

Examples:
  grepai trace callees "Login"
  grepai trace callees "HandleRequest" --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTraceCallees,
}

var traceGraphCmd = &cobra.Command{
	Use:   "graph <symbol>",
	Short: "Build a call graph around the specified symbol",
	Long: `Build a call graph showing callers and callees around a symbol.

Examples:
  grepai trace graph "Login" --depth 2
  grepai trace graph "HandleRequest" --depth 3 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTraceGraph,
}

func init() {
	// Add flags to all trace subcommands
	for _, cmd := range []*cobra.Command{traceCallersCmd, traceCalleesCmd, traceGraphCmd} {
		cmd.Flags().StringVarP(&traceMode, "mode", "m", "fast", "Extraction mode: fast (regex) or precise (tree-sitter)")
		cmd.Flags().BoolVar(&traceJSON, "json", false, "Output results in JSON format")
		cmd.Flags().BoolVarP(&traceTOON, "toon", "t", false, "Output results in TOON format (token-efficient for AI agents)")
		cmd.MarkFlagsMutuallyExclusive("json", "toon")
		cmd.Flags().StringVar(&traceWorkspace, "workspace", "", "Workspace name for cross-project trace")
		cmd.Flags().StringVar(&traceProject, "project", "", "Project name within workspace (requires --workspace)")
	}
	traceGraphCmd.Flags().IntVarP(&traceDepth, "depth", "d", 2, "Maximum depth for graph traversal")

	traceCmd.AddCommand(traceCallersCmd)
	traceCmd.AddCommand(traceCalleesCmd)
	traceCmd.AddCommand(traceGraphCmd)

	rootCmd.AddCommand(traceCmd)
}

func runTraceCallers(cmd *cobra.Command, args []string) error {
	symbolName := args[0]
	ctx := context.Background()

	if traceProject != "" && traceWorkspace == "" {
		return fmt.Errorf("--project requires --workspace")
	}

	// Workspace mode: aggregate across projects
	if traceWorkspace != "" {
		stores, err := loadWorkspaceSymbolStores(ctx, traceWorkspace, traceProject)
		if err != nil {
			return err
		}
		defer closeSymbolStores(stores)

		result := trace.TraceResult{Query: symbolName, Mode: traceMode}
		for _, ss := range stores {
			symbols, _ := ss.LookupSymbol(ctx, symbolName)
			if len(symbols) > 0 && result.Symbol == nil {
				result.Symbol = &symbols[0]
			}
			refs, _ := ss.LookupCallers(ctx, symbolName)
			for _, ref := range refs {
				callerSyms, _ := ss.LookupSymbol(ctx, ref.CallerName)
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
		}

		if result.Symbol == nil {
			if traceJSON {
				return outputJSON(result)
			}
			if traceTOON {
				return outputTOON(result)
			}
			fmt.Printf("No symbol found: %s\n", symbolName)
			return nil
		}

		if traceJSON {
			return outputJSON(result)
		}
		if traceTOON {
			return outputTOON(result)
		}
		return displayCallersResult(result)
	}

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Initialize symbol store
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return fmt.Errorf("failed to load symbol index: %w", err)
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return fmt.Errorf("symbol index is empty. Run 'grepai watch' first to build the index")
	}

	// Lookup symbol
	symbols, err := symbolStore.LookupSymbol(ctx, symbolName)
	if err != nil {
		return fmt.Errorf("failed to lookup symbol: %w", err)
	}

	if len(symbols) == 0 {
		emptyResult := trace.TraceResult{Query: symbolName, Mode: traceMode}
		if traceJSON {
			return outputJSON(emptyResult)
		}
		if traceTOON {
			return outputTOON(emptyResult)
		}
		fmt.Printf("No symbol found: %s\n", symbolName)
		return nil
	}

	// Find callers
	refs, err := symbolStore.LookupCallers(ctx, symbolName)
	if err != nil {
		return fmt.Errorf("failed to lookup callers: %w", err)
	}

	result := trace.TraceResult{
		Query:  symbolName,
		Mode:   traceMode,
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

	// Enrich with RPG feature paths
	cfg, _ := config.Load(projectRoot)
	if cfg != nil {
		enrichTraceWithRPG(projectRoot, cfg, &result)
	}

	if traceJSON {
		return outputJSON(result)
	}
	if traceTOON {
		return outputTOON(result)
	}

	return displayCallersResult(result)
}

func runTraceCallees(cmd *cobra.Command, args []string) error {
	symbolName := args[0]
	ctx := context.Background()

	if traceProject != "" && traceWorkspace == "" {
		return fmt.Errorf("--project requires --workspace")
	}

	// Workspace mode: aggregate across projects
	if traceWorkspace != "" {
		stores, err := loadWorkspaceSymbolStores(ctx, traceWorkspace, traceProject)
		if err != nil {
			return err
		}
		defer closeSymbolStores(stores)

		result := trace.TraceResult{Query: symbolName, Mode: traceMode}
		for _, ss := range stores {
			symbols, _ := ss.LookupSymbol(ctx, symbolName)
			if len(symbols) > 0 && result.Symbol == nil {
				result.Symbol = &symbols[0]
			}
			if len(symbols) > 0 {
				refs, _ := ss.LookupCallees(ctx, symbolName, symbols[0].File)
				for _, ref := range refs {
					calleeSyms, _ := ss.LookupSymbol(ctx, ref.SymbolName)
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
			}
		}

		if result.Symbol == nil {
			if traceJSON {
				return outputJSON(result)
			}
			if traceTOON {
				return outputTOON(result)
			}
			fmt.Printf("No symbol found: %s\n", symbolName)
			return nil
		}

		if traceJSON {
			return outputJSON(result)
		}
		if traceTOON {
			return outputTOON(result)
		}
		return displayCalleesResult(result)
	}

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return fmt.Errorf("failed to load symbol index: %w", err)
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return fmt.Errorf("symbol index is empty. Run 'grepai watch' first to build the index")
	}

	// Lookup symbol
	symbols, err := symbolStore.LookupSymbol(ctx, symbolName)
	if err != nil {
		return fmt.Errorf("failed to lookup symbol: %w", err)
	}

	if len(symbols) == 0 {
		emptyResult := trace.TraceResult{Query: symbolName, Mode: traceMode}
		if traceJSON {
			return outputJSON(emptyResult)
		}
		if traceTOON {
			return outputTOON(emptyResult)
		}
		fmt.Printf("No symbol found: %s\n", symbolName)
		return nil
	}

	// Find callees
	refs, err := symbolStore.LookupCallees(ctx, symbolName, symbols[0].File)
	if err != nil {
		return fmt.Errorf("failed to lookup callees: %w", err)
	}

	result := trace.TraceResult{
		Query:  symbolName,
		Mode:   traceMode,
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

	// Enrich with RPG feature paths
	cfg, _ := config.Load(projectRoot)
	if cfg != nil {
		enrichTraceWithRPG(projectRoot, cfg, &result)
	}

	if traceJSON {
		return outputJSON(result)
	}
	if traceTOON {
		return outputTOON(result)
	}

	return displayCalleesResult(result)
}

func runTraceGraph(cmd *cobra.Command, args []string) error {
	symbolName := args[0]
	ctx := context.Background()

	if traceProject != "" && traceWorkspace == "" {
		return fmt.Errorf("--project requires --workspace")
	}

	// Workspace mode: aggregate call graphs across projects
	if traceWorkspace != "" {
		stores, err := loadWorkspaceSymbolStores(ctx, traceWorkspace, traceProject)
		if err != nil {
			return err
		}
		defer closeSymbolStores(stores)

		// Merge graphs from all project stores
		merged := &trace.CallGraph{
			Root:  symbolName,
			Nodes: make(map[string]trace.Symbol),
			Edges: []trace.CallEdge{},
			Depth: traceDepth,
		}
		edgeSeen := make(map[string]bool)

		for _, ss := range stores {
			graph, graphErr := ss.GetCallGraph(ctx, symbolName, traceDepth)
			if graphErr != nil {
				continue
			}
			for name, sym := range graph.Nodes {
				if _, exists := merged.Nodes[name]; !exists {
					merged.Nodes[name] = sym
				}
			}
			for _, edge := range graph.Edges {
				key := edge.Caller + "->" + edge.Callee
				if !edgeSeen[key] {
					merged.Edges = append(merged.Edges, edge)
					edgeSeen[key] = true
				}
			}
		}

		result := trace.TraceResult{
			Query: symbolName,
			Mode:  traceMode,
			Graph: merged,
		}

		if traceJSON {
			return outputJSON(result)
		}
		if traceTOON {
			return outputTOON(result)
		}
		return displayGraphResult(result)
	}

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		return fmt.Errorf("failed to load symbol index: %w", err)
	}
	defer symbolStore.Close()

	// Check if index exists
	stats, err := symbolStore.GetStats(ctx)
	if err != nil || stats.TotalSymbols == 0 {
		return fmt.Errorf("symbol index is empty. Run 'grepai watch' first to build the index")
	}

	graph, err := symbolStore.GetCallGraph(ctx, symbolName, traceDepth)
	if err != nil {
		return fmt.Errorf("failed to build call graph: %w", err)
	}

	result := trace.TraceResult{
		Query: symbolName,
		Mode:  traceMode,
		Graph: graph,
	}

	// Enrich with RPG feature paths
	cfg, _ := config.Load(projectRoot)
	if cfg != nil {
		enrichTraceWithRPG(projectRoot, cfg, &result)
	}

	if traceJSON {
		return outputJSON(result)
	}
	if traceTOON {
		return outputTOON(result)
	}

	return displayGraphResult(result)
}

// enrichTraceWithRPG enriches all symbols in a TraceResult with RPG feature paths.
func enrichTraceWithRPG(projectRoot string, cfg *config.Config, result *trace.TraceResult) {
	if !cfg.RPG.Enabled {
		return
	}

	ctx := context.Background()
	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
	if err := rpgStore.Load(ctx); err != nil {
		return // best-effort
	}
	defer rpgStore.Close()

	graph := rpgStore.GetGraph()
	qe := rpg.NewQueryEngine(graph)

	lookupFeaturePath := func(sym *trace.Symbol) {
		if sym == nil || sym.File == "" {
			return
		}
		nodes := graph.GetNodesByFile(sym.File)
		for _, n := range nodes {
			if n.Kind == rpg.KindSymbol && n.SymbolName == sym.Name {
				fetchResult, err := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: n.ID})
				if err == nil && fetchResult != nil {
					sym.FeaturePath = fetchResult.FeaturePath
				}
				return
			}
		}
	}

	// Enrich the main symbol
	if result.Symbol != nil {
		lookupFeaturePath(result.Symbol)
	}

	// Enrich callers
	for i := range result.Callers {
		lookupFeaturePath(&result.Callers[i].Symbol)
	}

	// Enrich callees
	for i := range result.Callees {
		lookupFeaturePath(&result.Callees[i].Symbol)
	}

	// Enrich graph nodes
	if result.Graph != nil {
		for name, sym := range result.Graph.Nodes {
			lookupFeaturePath(&sym)
			result.Graph.Nodes[name] = sym
		}
	}
}

func outputJSON(result trace.TraceResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputTOON(result trace.TraceResult) error {
	output, err := gotoon.Encode(result)
	if err != nil {
		return fmt.Errorf("failed to encode TOON: %w", err)
	}
	fmt.Println(output)
	return nil
}

func displayCallersResult(result trace.TraceResult) error {
	fmt.Printf("Symbol: %s (%s)\n", result.Symbol.Name, result.Symbol.Kind)
	fmt.Printf("File: %s:%d\n", result.Symbol.File, result.Symbol.Line)
	if result.Symbol.FeaturePath != "" {
		fmt.Printf("Feature: %s\n", result.Symbol.FeaturePath)
	}
	fmt.Printf("\nCallers (%d):\n", len(result.Callers))
	fmt.Println(strings.Repeat("-", 60))

	if len(result.Callers) == 0 {
		fmt.Println("No callers found.")
		return nil
	}

	for i, caller := range result.Callers {
		fmt.Printf("\n%d. %s\n", i+1, caller.Symbol.Name)
		if caller.Symbol.File != "" {
			fmt.Printf("   Defined: %s:%d\n", caller.Symbol.File, caller.Symbol.Line)
		}
		if caller.Symbol.FeaturePath != "" {
			fmt.Printf("   Feature: %s\n", caller.Symbol.FeaturePath)
		}
		fmt.Printf("   Calls at: %s:%d\n", caller.CallSite.File, caller.CallSite.Line)
		if caller.CallSite.Context != "" {
			fmt.Printf("   Context: %s\n", truncate(caller.CallSite.Context, 80))
		}
	}

	return nil
}

func displayCalleesResult(result trace.TraceResult) error {
	fmt.Printf("Symbol: %s (%s)\n", result.Symbol.Name, result.Symbol.Kind)
	fmt.Printf("File: %s:%d\n", result.Symbol.File, result.Symbol.Line)
	if result.Symbol.FeaturePath != "" {
		fmt.Printf("Feature: %s\n", result.Symbol.FeaturePath)
	}
	fmt.Printf("\nCallees (%d):\n", len(result.Callees))
	fmt.Println(strings.Repeat("-", 60))

	if len(result.Callees) == 0 {
		fmt.Println("No callees found.")
		return nil
	}

	for i, callee := range result.Callees {
		fmt.Printf("\n%d. %s\n", i+1, callee.Symbol.Name)
		if callee.Symbol.File != "" {
			fmt.Printf("   Defined: %s:%d\n", callee.Symbol.File, callee.Symbol.Line)
		}
		if callee.Symbol.FeaturePath != "" {
			fmt.Printf("   Feature: %s\n", callee.Symbol.FeaturePath)
		}
		fmt.Printf("   Called at: %s:%d\n", callee.CallSite.File, callee.CallSite.Line)
	}

	return nil
}

func displayGraphResult(result trace.TraceResult) error {
	fmt.Printf("Call Graph for: %s (depth: %d)\n", result.Query, result.Graph.Depth)
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nNodes (%d):\n", len(result.Graph.Nodes))
	for name, sym := range result.Graph.Nodes {
		if sym.FeaturePath != "" {
			fmt.Printf("  - %s (%s) @ %s:%d [%s]\n", name, sym.Kind, sym.File, sym.Line, sym.FeaturePath)
		} else {
			fmt.Printf("  - %s (%s) @ %s:%d\n", name, sym.Kind, sym.File, sym.Line)
		}
	}

	fmt.Printf("\nEdges (%d):\n", len(result.Graph.Edges))
	for _, edge := range result.Graph.Edges {
		fmt.Printf("  %s -> %s [%s:%d]\n", edge.Caller, edge.Callee, edge.File, edge.Line)
	}

	return nil
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// loadWorkspaceSymbolStores loads GOBSymbolStores for workspace projects.
// If projectName is non-empty, only that project's store is loaded.
func loadWorkspaceSymbolStores(ctx context.Context, workspaceName, projectName string) ([]trace.SymbolStore, error) {
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return nil, fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(workspaceName)
	if err != nil {
		return nil, err
	}

	var projects []config.ProjectEntry
	if projectName != "" {
		found := false
		for _, p := range ws.Projects {
			if p.Name == projectName {
				projects = []config.ProjectEntry{p}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("project %q not found in workspace %q", projectName, workspaceName)
		}
	} else {
		projects = ws.Projects
	}

	stores := make([]trace.SymbolStore, 0, len(projects))
	for _, p := range projects {
		ss := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(p.Path))
		if err := ss.Load(ctx); err != nil {
			ss.Close()
			// Close already-loaded stores on error
			for _, s := range stores {
				s.Close()
			}
			return nil, fmt.Errorf("failed to load symbol index for project %s: %w", p.Name, err)
		}
		stores = append(stores, ss)
	}
	return stores, nil
}

// closeSymbolStores closes all symbol stores in the slice.
func closeSymbolStores(stores []trace.SymbolStore) {
	for _, s := range stores {
		s.Close()
	}
}
