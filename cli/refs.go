package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/alpkeskin/gotoon"
	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

var (
	refsJSON      bool
	refsTOON      bool
	refsWorkspace string
	refsProject   string
)

type refsUsage struct {
	Symbol   trace.Symbol   `json:"symbol"`
	Access   string         `json:"access"`
	AccessAt trace.CallSite `json:"access_at"`
}

type refsResult struct {
	Query   string      `json:"query"`
	Kind    string      `json:"kind"`
	Mode    string      `json:"mode"`
	Readers []refsUsage `json:"readers,omitempty"`
	Writers []refsUsage `json:"writers,omitempty"`
}

type refsGraphResult struct {
	Query   string      `json:"query"`
	Kind    string      `json:"kind"`
	Mode    string      `json:"mode"`
	Readers []refsUsage `json:"readers,omitempty"`
	Writers []refsUsage `json:"writers,omitempty"`
}

var refsCmd = &cobra.Command{
	Use:   "refs <subcommand> <symbol>",
	Short: "Trace property/data readers and writers",
	Long: `Refs command finds non-call data usage edges for a symbol name.

Use this for property/state usage (e.g. store.uid), while 'trace' remains call-graph focused.

Examples:
  grepai refs readers "uid"
  grepai refs writers "uid" --json`,
}

var refsReadersCmd = &cobra.Command{
	Use:   "readers <symbol>",
	Short: "Find functions/components that read a property or data symbol",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := runRefs(args[0], true)
		if err != nil {
			return err
		}
		return outputRefsResult(result, true)
	},
}

var refsWritersCmd = &cobra.Command{
	Use:   "writers <symbol>",
	Short: "Find functions/components that write a property or data symbol",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := runRefs(args[0], false)
		if err != nil {
			return err
		}
		return outputRefsResult(result, false)
	},
}

var refsGraphCmd = &cobra.Command{
	Use:   "graph <symbol>",
	Short: "Show readers and writers graph for a property/data symbol",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		readersResult, err := runRefs(args[0], true)
		if err != nil {
			return err
		}
		writersResult, err := runRefs(args[0], false)
		if err != nil {
			return err
		}

		graph := refsGraphResult{
			Query:   args[0],
			Kind:    "property",
			Mode:    "fast",
			Readers: readersResult.Readers,
			Writers: writersResult.Writers,
		}
		return outputRefsGraphResult(graph)
	},
}

func init() {
	for _, cmd := range []*cobra.Command{refsReadersCmd, refsWritersCmd, refsGraphCmd} {
		cmd.Flags().BoolVar(&refsJSON, "json", false, "Output results in JSON format")
		cmd.Flags().BoolVarP(&refsTOON, "toon", "t", false, "Output results in TOON format (token-efficient for AI agents)")
		cmd.MarkFlagsMutuallyExclusive("json", "toon")
		cmd.Flags().StringVar(&refsWorkspace, "workspace", "", "Workspace name for cross-project refs")
		cmd.Flags().StringVar(&refsProject, "project", "", "Project name within workspace (requires --workspace)")
	}

	refsCmd.AddCommand(refsReadersCmd)
	refsCmd.AddCommand(refsWritersCmd)
	refsCmd.AddCommand(refsGraphCmd)
	rootCmd.AddCommand(refsCmd)
}

func runRefs(symbolName string, readers bool) (refsResult, error) {
	ctx := context.Background()

	if refsProject != "" && refsWorkspace == "" {
		return refsResult{}, fmt.Errorf("--project requires --workspace")
	}

	var stores []trace.SymbolStore
	if refsWorkspace != "" {
		var err error
		stores, err = trace.LoadWorkspaceSymbolStores(ctx, refsWorkspace, refsProject)
		if err != nil {
			return refsResult{}, err
		}
		defer trace.CloseSymbolStores(stores)
	} else {
		projectRoot, err := config.FindProjectRoot()
		if err != nil {
			return refsResult{}, fmt.Errorf("failed to find project root: %w", err)
		}

		symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
		if err := symbolStore.Load(ctx); err != nil {
			return refsResult{}, fmt.Errorf("failed to load symbol index: %w", err)
		}
		defer symbolStore.Close()

		stats, err := symbolStore.GetStats(ctx)
		if err != nil || stats.TotalSymbols == 0 {
			return refsResult{}, fmt.Errorf("symbol index is empty. Run 'grepai watch' first to build the index")
		}

		stores = []trace.SymbolStore{symbolStore}
	}

	result := refsResult{Query: symbolName, Kind: "property", Mode: "fast"}
	for _, ss := range stores {
		var refs []trace.Reference
		var err error
		if readers {
			refs, err = ss.LookupReaders(ctx, symbolName)
		} else {
			refs, err = ss.LookupWriters(ctx, symbolName)
		}
		if err != nil {
			log.Printf("Warning: failed to lookup refs for %q: %v", symbolName, err)
			continue
		}

		for _, ref := range refs {
			sym := resolveRefCallerSymbol(ctx, ss, ref)
			usage := refsUsage{
				Symbol: sym,
				Access: ref.Kind,
				AccessAt: trace.CallSite{
					File:    ref.File,
					Line:    ref.Line,
					Context: ref.Context,
				},
			}
			if readers {
				result.Readers = append(result.Readers, usage)
			} else {
				result.Writers = append(result.Writers, usage)
			}
		}
	}

	return result, nil
}

func resolveRefCallerSymbol(ctx context.Context, ss trace.SymbolStore, ref trace.Reference) trace.Symbol {
	if ref.CallerName == "" || ref.CallerName == "<top-level>" {
		return trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
	}

	candidates, err := ss.LookupSymbol(ctx, ref.CallerName)
	if err != nil || len(candidates) == 0 {
		return trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
	}
	best := pickBestSymbolForFile(candidates, ref.CallerFile)
	if best == nil {
		return trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
	}
	return *best
}

func outputRefsResult(result refsResult, readers bool) error {
	if refsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if refsTOON {
		output, err := gotoon.Encode(result)
		if err != nil {
			return fmt.Errorf("failed to encode TOON: %w", err)
		}
		fmt.Println(output)
		return nil
	}

	label := "Readers"
	usages := result.Readers
	if !readers {
		label = "Writers"
		usages = result.Writers
	}

	fmt.Printf("Symbol: %s (%s)\n", result.Query, result.Kind)
	fmt.Printf("\n%s (%d):\n", label, len(usages))
	fmt.Println(strings.Repeat("-", 60))

	if len(usages) == 0 {
		fmt.Println("No references found.")
		return nil
	}

	for i, usage := range usages {
		fmt.Printf("\n%d. %s\n", i+1, usage.Symbol.Name)
		if usage.Symbol.File != "" {
			fmt.Printf("   Defined: %s:%d\n", usage.Symbol.File, usage.Symbol.Line)
		}
		fmt.Printf("   Access: %s at %s:%d\n", usage.Access, usage.AccessAt.File, usage.AccessAt.Line)
		if usage.AccessAt.Context != "" {
			fmt.Printf("   Context: %s\n", truncate(usage.AccessAt.Context, 100))
		}
	}

	return nil
}

func outputRefsGraphResult(result refsGraphResult) error {
	if refsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if refsTOON {
		output, err := gotoon.Encode(result)
		if err != nil {
			return fmt.Errorf("failed to encode TOON: %w", err)
		}
		fmt.Println(output)
		return nil
	}

	fmt.Printf("Symbol: %s (%s)\n", result.Query, result.Kind)
	fmt.Printf("Readers: %d\n", len(result.Readers))
	fmt.Printf("Writers: %d\n", len(result.Writers))
	fmt.Println(strings.Repeat("-", 60))

	if len(result.Readers) > 0 {
		fmt.Println("Reader Sites:")
		for i, usage := range result.Readers {
			fmt.Printf("%d. %s @ %s:%d\n", i+1, usage.Symbol.Name, usage.AccessAt.File, usage.AccessAt.Line)
		}
	}
	if len(result.Writers) > 0 {
		if len(result.Readers) > 0 {
			fmt.Println()
		}
		fmt.Println("Writer Sites:")
		for i, usage := range result.Writers {
			fmt.Printf("%d. %s @ %s:%d\n", i+1, usage.Symbol.Name, usage.AccessAt.File, usage.AccessAt.Line)
		}
	}
	if len(result.Readers) == 0 && len(result.Writers) == 0 {
		fmt.Println("No references found.")
	}

	return nil
}
