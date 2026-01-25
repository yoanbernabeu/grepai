package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/mcp"
)

var mcpServeCmd = &cobra.Command{
	Use:   "mcp-serve [project-path]",
	Short: "Start grepai as an MCP server",
	Long: `Start grepai as an MCP (Model Context Protocol) server.

This allows AI agents to use grepai as a native tool through the MCP protocol.
The server communicates via stdio and exposes the following tools:

  - grepai_search: Semantic code search with natural language
  - grepai_trace_callers: Find all functions that call a symbol
  - grepai_trace_callees: Find all functions called by a symbol
  - grepai_trace_graph: Build a call graph around a symbol
  - grepai_index_status: Check index health and statistics

Arguments:
  project-path  Optional path to the grepai project directory.
                If not provided, searches for .grepai from current directory.

Configuration for Claude Code:
  claude mcp add grepai -- grepai mcp-serve

Configuration for Cursor (.cursor/mcp.json):
  {
    "mcpServers": {
      "grepai": {
        "command": "grepai",
        "args": ["mcp-serve"]
      }
    }
  }

Configuration for Cursor with explicit path (recommended for Windows):
  {
    "mcpServers": {
      "grepai": {
        "command": "grepai",
        "args": ["mcp-serve", "/path/to/your/project"]
      }
    }
  }`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMCPServe,
}

func init() {
	rootCmd.AddCommand(mcpServeCmd)
}

func runMCPServe(_ *cobra.Command, args []string) error {
	var projectRoot string
	var err error

	if len(args) > 0 {
		// Explicit path provided
		projectRoot = args[0]

		// Convert to absolute path if relative
		if !filepath.IsAbs(projectRoot) {
			projectRoot, err = filepath.Abs(projectRoot)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}
		}

		// Validate that it's a grepai project
		if !config.Exists(projectRoot) {
			return fmt.Errorf("no grepai project found at %s (run 'grepai init' first)", projectRoot)
		}
	} else {
		// Default behavior (backward compatibility)
		projectRoot, err = config.FindProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to find project root: %w", err)
		}
	}

	// Create and start MCP server
	server, err := mcp.NewServer(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	return server.Serve()
}
