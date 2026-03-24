package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

var migrateModelCmd = &cobra.Command{
	Use:   "migrate-model <provider/model>",
	Short: "Stamp untagged chunks with an embedding model identifier",
	Long: `Stamp all chunks that have an empty EmbedModel field with the given
provider/model string. This is required when enabling multi_model on an
existing index so that legacy chunks become searchable under the new
filtering rules.

No embedding API calls are made; this is a metadata-only operation.

Example:
  grepai migrate-model ollama/nomic-embed-text`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrateModel,
}

func init() {
	rootCmd.AddCommand(migrateModelCmd)
}

func runMigrateModel(cmd *cobra.Command, args []string) error {
	modelTag := args[0]

	// Validate model tag format
	if !strings.Contains(modelTag, "/") {
		return fmt.Errorf("model tag must be in provider/model format (e.g. ollama/nomic-embed-text), got %q", modelTag)
	}

	parts := strings.SplitN(modelTag, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("model tag must have non-empty provider and model (e.g. ollama/nomic-embed-text), got %q", modelTag)
	}

	ctx := context.Background()

	// Find project root
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Only GOB backend supported for now
	if cfg.Store.Backend != "gob" {
		return fmt.Errorf("migrate-model currently supports only the gob backend, got %q", cfg.Store.Backend)
	}

	// Load store
	indexPath := config.GetIndexPath(projectRoot)
	gobStore := store.NewGOBStore(indexPath)
	if err := gobStore.Load(ctx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	// Get all chunks
	allChunks, err := gobStore.GetAllChunks(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chunks: %w", err)
	}

	// Find chunks with empty EmbedModel and stamp them
	var migrated int
	var updated []store.Chunk
	for _, chunk := range allChunks {
		if chunk.EmbedModel == "" {
			chunk.EmbedModel = modelTag
			updated = append(updated, chunk)
			migrated++
		}
	}

	if migrated == 0 {
		fmt.Println("No untagged chunks found. All chunks already have a model tag.")
		return nil
	}

	// Save updated chunks
	if err := gobStore.SaveChunks(ctx, updated); err != nil {
		return fmt.Errorf("failed to save updated chunks: %w", err)
	}

	// Persist to disk
	if err := gobStore.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist index: %w", err)
	}

	fmt.Printf("Migrated %d chunks with model tag %q.\n", migrated, modelTag)
	return nil
}
