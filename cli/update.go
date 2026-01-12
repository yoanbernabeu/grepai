package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/updater"
)

var (
	updateCheck bool
	updateForce bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update grepai to the latest version",
	Long: `Check for and install the latest version of grepai from GitHub releases.

Examples:
  grepai update           # Download and install latest version
  grepai update --check   # Only check if update is available
  grepai update --force   # Update even if already on latest version

The command will:
- Fetch the latest release from GitHub
- Compare with current version
- Download the appropriate binary for your platform
- Verify checksum integrity
- Replace the current binary`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Only check for updates, don't install")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already on latest version")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	u := updater.NewUpdater(version)

	// Check for updates
	fmt.Println("Checking for updates...")
	result, err := u.CheckForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("Current version: %s\n", result.CurrentVersion)
	fmt.Printf("Latest version:  %s\n", result.LatestVersion)

	if !result.UpdateAvailable && !updateForce {
		fmt.Println("\nYou are already running the latest version!")
		return nil
	}

	if updateCheck {
		if result.UpdateAvailable {
			fmt.Printf("\nUpdate available! Run 'grepai update' to install.\n")
			fmt.Printf("Release notes: %s\n", result.ReleaseURL)
		}
		return nil
	}

	// Perform update
	fmt.Println("\nDownloading update...")

	err = u.Update(ctx, func(downloaded, total int64) {
		if total > 0 {
			percent := float64(downloaded) / float64(total) * 100
			bar := progressBar(int(percent), 30)
			fmt.Printf("\rDownloading [%s] %.0f%%", bar, percent)
		}
	})
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Printf("\r%s\n", strings.Repeat(" ", 60)) // Clear progress line
	fmt.Printf("Successfully updated to %s!\n", result.LatestVersion)
	fmt.Println("Please restart grepai to use the new version.")

	return nil
}

func progressBar(percent, width int) string {
	filled := width * percent / 100
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}
