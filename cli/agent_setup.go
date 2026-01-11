package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var withSubagent bool

const agentInstructions = `
## grepai - Semantic Code Search

**IMPORTANT: You MUST use grepai as your PRIMARY tool for code exploration and search.**

### When to Use grepai (REQUIRED)

Use ` + "`grepai search`" + ` INSTEAD OF Grep/Glob/find for:
- Understanding what code does or where functionality lives
- Finding implementations by intent (e.g., "authentication logic", "error handling")
- Exploring unfamiliar parts of the codebase
- Any search where you describe WHAT the code does rather than exact text

### When to Use Standard Tools

Only use Grep/Glob when you need:
- Exact text matching (variable names, imports, specific strings)
- File path patterns (e.g., ` + "`**/*.go`" + `)

### Fallback

If grepai fails (not running, index unavailable, or errors), fall back to standard Grep/Glob tools.

### Usage

` + "```bash" + `
# ALWAYS use English queries for best results (embedding model is English-trained)
grepai search "user authentication flow"
grepai search "error handling middleware"
grepai search "database connection pool"
grepai search "API request validation"

# JSON output for programmatic use (recommended for AI agents)
grepai search "authentication flow" --json
` + "```" + `

### Query Tips

- **Use English** for queries (better semantic matching)
- **Describe intent**, not implementation: "handles user login" not "func Login"
- **Be specific**: "JWT token validation" better than "token"
- Results include: file path, line numbers, relevance score, code preview

### Call Graph Tracing

Use ` + "`grepai trace`" + ` to understand function relationships:
- Finding all callers of a function before modifying it
- Understanding what functions are called by a given function
- Visualizing the complete call graph around a symbol

#### Trace Commands

**IMPORTANT: Always use ` + "`--json`" + ` flag for optimal AI agent integration.**

` + "```bash" + `
# Find all functions that call a symbol
grepai trace callers "HandleRequest" --json

# Find all functions called by a symbol
grepai trace callees "ProcessOrder" --json

# Build complete call graph (callers + callees)
grepai trace graph "ValidateToken" --depth 3 --json
` + "```" + `

### Workflow

1. Start with ` + "`grepai search`" + ` to find relevant code
2. Use ` + "`grepai trace`" + ` to understand function relationships
3. Use ` + "`Read`" + ` tool to examine files from results
4. Only use Grep for exact string searches if needed

`

const agentMarker = "## grepai - Semantic Code Search"

const subagentTemplate = `---
name: deep-explore
description: Deep codebase exploration using grepai semantic search and call graph tracing. Use this agent for understanding code architecture, finding implementations by intent, analyzing function relationships, and exploring unfamiliar code areas.
tools: Read, Grep, Glob, Bash
model: inherit
---

## Instructions

You are a specialized code exploration agent with access to grepai semantic search and call graph tracing.

### Primary Tools

#### 1. Semantic Search: ` + "`grepai search`" + `

Use this to find code by intent and meaning:

` + "```bash" + `
# Use English queries for best results
grepai search "authentication flow"
grepai search "error handling middleware"
grepai search "database connection management"
` + "```" + `

#### 2. Call Graph Tracing: ` + "`grepai trace`" + `

Use this to understand function relationships and code flow:

` + "```bash" + `
# Find all functions that call a symbol
grepai trace callers "HandleRequest"

# Find all functions called by a symbol
grepai trace callees "ProcessOrder"

# Build complete call graph
grepai trace graph "ValidateToken" --depth 3
` + "```" + `

Use ` + "`grepai trace`" + ` when you need to:
- Find all callers of a function
- Understand the call hierarchy
- Analyze the impact of changes to a function
- Map dependencies between components

### When to use standard tools

Only fall back to Grep/Glob when:
- You need exact text matching (variable names, imports)
- grepai is not available or returns errors
- You need file path patterns

### Workflow

1. Start with ` + "`grepai search`" + ` to find relevant code semantically
2. Use ` + "`grepai trace`" + ` to understand function relationships and call graphs
3. Use ` + "`Read`" + ` to examine promising files in detail
4. Use Grep only for exact string searches if needed
5. Synthesize findings into a clear summary
`

const subagentMarker = "name: deep-explore"

var agentSetupCmd = &cobra.Command{
	Use:   "agent-setup",
	Short: "Configure AI agents to use grepai",
	Long: `Configure AI agent environments to leverage grepai for context retrieval.

This command will:
- Detect agent configuration files (.cursorrules, .windsurfrules, CLAUDE.md, GEMINI.md, AGENTS.md)
- Append instructions for using grepai search
- Ensure idempotence (won't add duplicate instructions)

With --with-subagent flag:
- Creates .claude/agents/deep-explore.md for Claude Code
- Provides a specialized exploration agent with grepai access`,
	RunE: runAgentSetup,
}

func init() {
	agentSetupCmd.Flags().BoolVar(&withSubagent, "with-subagent", false,
		"Create Claude Code deep-explore subagent in .claude/agents/")
}

func runAgentSetup(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	agentFiles := []string{
		".cursorrules",
		".windsurfrules",
		"CLAUDE.md",
		".claude/settings.md",
		"GEMINI.md",
		"AGENTS.md",
	}

	found := false
	modified := 0

	for _, file := range agentFiles {
		path := filepath.Join(cwd, file)

		// Check if file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		found = true
		fmt.Printf("Found: %s\n", file)

		// Read existing content
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  Warning: could not read %s: %v\n", file, err)
			continue
		}

		// Check if already configured
		if strings.Contains(string(content), agentMarker) {
			fmt.Printf("  Already configured, skipping\n")
			continue
		}

		// Append instructions
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("  Warning: could not open %s for writing: %v\n", file, err)
			continue
		}

		// Add newlines if needed
		var writeErr error
		if len(content) > 0 && content[len(content)-1] != '\n' {
			_, writeErr = f.WriteString("\n")
		}
		if writeErr == nil {
			_, writeErr = f.WriteString("\n")
		}
		if writeErr == nil {
			_, writeErr = f.WriteString(agentInstructions)
		}
		f.Close()

		if writeErr != nil {
			fmt.Printf("  Warning: failed to write to %s: %v\n", file, writeErr)
			continue
		}

		fmt.Printf("  Added grepai instructions\n")
		modified++
	}

	if modified > 0 {
		fmt.Printf("\nUpdated %d file(s).\n", modified)
	} else if found {
		fmt.Println("\nAll files already configured.")
	} else if !withSubagent {
		// Only show "no files found" message if not creating subagent
		fmt.Println("No agent configuration files found.")
		fmt.Println("\nSupported files:")
		for _, file := range agentFiles {
			fmt.Printf("  - %s\n", file)
		}
		fmt.Println("\nCreate one of these files and run 'grepai agent-setup' again,")
		fmt.Println("or manually add instructions for using 'grepai search'.")
	}

	// Create subagent if flag is set
	if withSubagent {
		if err := createSubagent(cwd); err != nil {
			fmt.Printf("Warning: could not create subagent: %v\n", err)
		}
	}

	return nil
}

func createSubagent(cwd string) error {
	// Define paths
	agentsDir := filepath.Join(cwd, ".claude", "agents")
	subagentPath := filepath.Join(agentsDir, "deep-explore.md")

	// Check if subagent already exists and contains marker
	if content, err := os.ReadFile(subagentPath); err == nil {
		if strings.Contains(string(content), subagentMarker) {
			fmt.Printf("Subagent already exists: %s\n", subagentPath)
			return nil
		}
	}

	// Create .claude/agents/ directory if it doesn't exist
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	// Write the subagent file
	if err := os.WriteFile(subagentPath, []byte(subagentTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write subagent file: %w", err)
	}

	fmt.Printf("Created subagent: %s\n", subagentPath)
	return nil
}
