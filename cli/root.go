package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version string
	rootCmd = &cobra.Command{
		Use:   "grepai",
		Short: "Semantic code search CLI",
		Long: `grepai is a privacy-first semantic code search tool.

Unlike grep (exact text matching), grepai indexes the meaning of your code
using vector embeddings, enabling natural language searches.

It runs as a background daemon, maintaining a real-time "mental map" of your
project to serve as reliable context for developers and AI agents.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
)

func SetVersion(v string) {
	version = v
}

func Execute() error {
	return rootCmd.Execute()
}

// GetRootCmd returns the root command for documentation generation
func GetRootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(agentSetupCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(workspaceCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("grepai version %s\n", version)
	},
}
