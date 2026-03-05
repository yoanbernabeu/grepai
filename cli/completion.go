package cli

import (
	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
)

var completionCmd = &cobra.Command{
	Use:   "completion [shell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for grepai.

Zsh:

  # Method 1: eval (add to ~/.zshrc)
  eval "$(grepai completion zsh)"

  # Method 2: Oh-My-Zsh
  mkdir -p ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/grepai
  grepai completion zsh > ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/grepai/_grepai
  # Then add "grepai" to plugins=(...) in ~/.zshrc

  # Method 3: Manual fpath
  grepai completion zsh > "${fpath[1]}/_grepai"
  # Then restart your shell

Bash:

  # Linux
  grepai completion bash > /etc/bash_completion.d/grepai

  # macOS (requires bash-completion@2)
  grepai completion bash > $(brew --prefix)/etc/bash_completion.d/grepai

Fish:

  grepai completion fish > ~/.config/fish/completions/grepai.fish

PowerShell:

  grepai completion powershell | Out-String | Invoke-Expression
`,
}

var completionZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Generate zsh completion script",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenZshCompletion(cmd.OutOrStdout())
	},
}

var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "Generate bash completion script",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
	},
}

var completionFishCmd = &cobra.Command{
	Use:   "fish",
	Short: "Generate fish completion script",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
	},
}

var completionPowershellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "Generate powershell completion script",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
	},
}

func init() {
	completionCmd.AddCommand(completionZshCmd)
	completionCmd.AddCommand(completionBashCmd)
	completionCmd.AddCommand(completionFishCmd)
	completionCmd.AddCommand(completionPowershellCmd)

	rootCmd.AddCommand(completionCmd)

	cobra.OnInitialize(registerCompletions)
}

func registerCompletions() {
	// Static flag completions for initCmd
	_ = initCmd.RegisterFlagCompletionFunc("provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"ollama\tLocal embedding with Ollama",
			"lmstudio\tLocal embedding with LM Studio",
			"openai\tCloud embedding with OpenAI",
			"synthetic\tCloud embedding with Synthetic (free)",
			"openrouter\tCloud multi-provider gateway",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = initCmd.RegisterFlagCompletionFunc("backend", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"gob\tLocal file-based storage",
			"postgres\tPostgreSQL with pgvector",
			"qdrant\tQdrant vector database",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// Static flag completions for workspaceCreateCmd
	_ = workspaceCreateCmd.RegisterFlagCompletionFunc("backend", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"postgres\tPostgreSQL with pgvector",
			"qdrant\tQdrant vector database",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = workspaceCreateCmd.RegisterFlagCompletionFunc("provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"ollama\tLocal embedding with Ollama",
			"lmstudio\tLocal embedding with LM Studio",
			"openai\tCloud embedding with OpenAI",
			"synthetic\tCloud embedding with Synthetic (free)",
			"openrouter\tCloud multi-provider gateway",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// Static flag completions for trace commands (mode)
	for _, cmd := range []*cobra.Command{traceCallersCmd, traceCalleesCmd, traceGraphCmd} {
		cmd := cmd
		_ = cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"fast\tRegex-based extraction (faster)",
				"precise\tTree-sitter extraction (more accurate)",
			}, cobra.ShellCompDirectiveNoFileComp
		})
	}

	// Dynamic workspace name completions for --workspace flags
	workspaceCompleter := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
	}
	_ = searchCmd.RegisterFlagCompletionFunc("workspace", workspaceCompleter)
	_ = watchCmd.RegisterFlagCompletionFunc("workspace", workspaceCompleter)
	_ = mcpServeCmd.RegisterFlagCompletionFunc("workspace", workspaceCompleter)
	for _, cmd := range []*cobra.Command{traceCallersCmd, traceCalleesCmd, traceGraphCmd} {
		_ = cmd.RegisterFlagCompletionFunc("workspace", workspaceCompleter)
	}

	// Dynamic project completion for searchCmd --project (depends on --workspace value)
	_ = searchCmd.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		wsName, _ := cmd.Flags().GetString("workspace")
		if wsName == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeProjectNames(wsName), cobra.ShellCompDirectiveNoFileComp
	})

	// Dynamic ValidArgsFunction for workspace subcommands
	workspaceShowCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	workspaceDeleteCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	workspaceStatusCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	workspaceAddCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	workspaceRemoveCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeWorkspaceNames(), cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return completeProjectNames(args[0]), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeWorkspaceNames() []string {
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.ListWorkspaces()
}

func completeProjectNames(workspaceName string) []string {
	cfg, err := config.LoadWorkspaceConfig()
	if err != nil || cfg == nil {
		return nil
	}
	ws, err := cfg.GetWorkspace(workspaceName)
	if err != nil {
		return nil
	}
	names := make([]string, len(ws.Projects))
	for i, p := range ws.Projects {
		names[i] = p.Name
	}
	return names
}
