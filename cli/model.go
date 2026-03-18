package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/internal/managedassets"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage locally installed llama.cpp embedding models",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if !managedLlamaCPPSupported() {
			return managedLlamaCPPUnsupportedError()
		}
		return nil
	},
}

var modelInstallCmd = &cobra.Command{
	Use:   "install [model]",
	Short: "Install a managed local embedding model",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelID := managedassets.DefaultModelID
		if len(args) == 1 && args[0] != "" {
			modelID = args[0]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installing managed model %s...\n", modelID)
		model, err := managedassets.InstallModel(context.Background(), modelID, func(downloaded, total int64) {
			renderDownloadProgress("Model", downloaded, total)
		})
		fmt.Fprint(cmd.OutOrStdout(), "\r"+progressPadding()+"\r")
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installed model %s at %s\n", model.ID, model.Path)
		return nil
	},
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed managed local models",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		models, err := managedassets.LoadInstalledModels()
		if err != nil {
			return err
		}
		if len(models) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No managed local models installed")
			return nil
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "MODEL\tSIZE\tDIMENSIONS\tPATH")
		for _, model := range models {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", model.ID, formatSize(model.SizeBytes), model.Dimensions, model.Path)
		}
		return tw.Flush()
	},
}

var modelListAvailableCmd = &cobra.Command{
	Use:   "list-available",
	Short: "List available managed local models",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		models := managedassets.ListAvailableModels()
		if len(models) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No managed local models available")
			return nil
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "MODEL\tSIZE\tDIMENSIONS\tNAME")
		for _, model := range models {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", model.ID, formatSize(model.SizeBytes), model.Dimensions, model.Display)
		}
		return tw.Flush()
	},
}

var modelUseCmd = &cobra.Command{
	Use:   "use <model>",
	Short: "Use an installed managed local model for the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelDef, err := managedassets.LookupModel(args[0])
		if err != nil {
			return err
		}

		installedModels, err := managedassets.LoadInstalledModels()
		if err != nil {
			return err
		}
		installed := false
		for _, model := range installedModels {
			if model.ID == modelDef.ID {
				installed = true
				break
			}
		}
		if !installed {
			return fmt.Errorf("managed model %q is not installed; run 'grepai model install %s'", modelDef.ID, modelDef.ID)
		}

		projectRoot, err := config.FindProjectRoot()
		if err != nil {
			return err
		}
		cfg, err := config.Load(projectRoot)
		if err != nil {
			return err
		}

		cfg.Embedder.Provider = "llamacpp"
		cfg.Embedder.Model = modelDef.ID
		cfg.Embedder.ModelPath = ""
		cfg.Embedder.Endpoint = config.DefaultLlamaCPPEndpoint
		cfg.Embedder.Parallelism = 0
		dim := modelDef.Dimensions
		cfg.Embedder.Dimensions = &dim

		if err := cfg.Save(projectRoot); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Configured %s to use model %s\n", projectRoot, modelDef.ID)
		return nil
	},
}

var modelRemoveCmd = &cobra.Command{
	Use:   "remove <model>",
	Short: "Remove an installed managed local model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := managedassets.RemoveInstalledModel(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed model %s\n", args[0])
		return nil
	},
}

func init() {
	modelCmd.AddCommand(modelInstallCmd)
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelListAvailableCmd)
	modelCmd.AddCommand(modelUseCmd)
	modelCmd.AddCommand(modelRemoveCmd)
	rootCmd.AddCommand(modelCmd)
}

func renderDownloadProgress(label string, downloaded, total int64) {
	if total > 0 {
		percent := float64(downloaded) / float64(total) * 100
		fmt.Printf("\r%s [%s] %.0f%%", label, progressBar(int(percent), 30), percent)
		return
	}
	fmt.Printf("\r%s %d bytes", label, downloaded)
}

func progressPadding() string {
	return fmt.Sprintf("%60s", "")
}

func formatSize(sizeBytes int64) string {
	switch {
	case sizeBytes <= 0:
		return "-"
	case sizeBytes < 1024*1024:
		return fmt.Sprintf("%.0f KB", float64(sizeBytes)/1024)
	case sizeBytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(sizeBytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(sizeBytes)/(1024*1024*1024))
	}
}
