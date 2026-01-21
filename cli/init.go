package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
)

var (
	initProvider       string
	initBackend        string
	initNonInteractive bool
)

const (
	openAI3SmallDimensions      = 1536
	lmStudioEmbeddingDimensions = 768
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize grepai in the current directory",
	Long: `Initialize grepai by creating a .grepai directory with configuration.

This command will:
- Create .grepai/config.yaml with default settings
- Prompt for embedding provider (Ollama or OpenAI)
- Prompt for storage backend (GOB file or PostgreSQL)
- Add .grepai/ to .gitignore if present`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initProvider, "provider", "p", "", "Embedding provider (ollama, lmstudio, or openai)")
	initCmd.Flags().StringVarP(&initBackend, "backend", "b", "", "Storage backend (gob or postgres)")
	initCmd.Flags().BoolVar(&initNonInteractive, "yes", false, "Use defaults without prompting")
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if already initialized
	if config.Exists(cwd) {
		fmt.Println("grepai is already initialized in this directory.")
		fmt.Printf("Configuration: %s\n", config.GetConfigPath(cwd))
		return nil
	}

	cfg := config.DefaultConfig()

	// Interactive mode
	if !initNonInteractive {
		reader := bufio.NewReader(os.Stdin)

		// Provider selection
		if initProvider == "" {
			fmt.Println("\nSelect embedding provider:")
			fmt.Println("  1) ollama (local, privacy-first, requires Ollama running)")
			fmt.Println("  2) lmstudio (local, OpenAI-compatible, requires LM Studio running)")
			fmt.Println("  3) openai (cloud, requires API key)")
			fmt.Print("Choice [1]: ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "2", "lmstudio":
				cfg.Embedder.Provider = "lmstudio"
				cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "http://127.0.0.1:1234"
				cfg.Embedder.Dimensions = lmStudioEmbeddingDimensions
			case "3", "openai":
				cfg.Embedder.Provider = "openai"
				cfg.Embedder.Model = "text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://api.openai.com/v1"
				cfg.Embedder.Dimensions = openAI3SmallDimensions
			default:
				cfg.Embedder.Provider = "ollama"
			}
		} else {
			cfg.Embedder.Provider = initProvider
			switch initProvider {
			case "lmstudio":
				cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "http://127.0.0.1:1234"
				cfg.Embedder.Dimensions = lmStudioEmbeddingDimensions
			case "openai":
				cfg.Embedder.Model = "text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://api.openai.com/v1"
				cfg.Embedder.Dimensions = openAI3SmallDimensions
			}
		}

		// Backend selection
		if initBackend == "" {
			fmt.Println("\nSelect storage backend:")
			fmt.Println("  1) gob (local file, recommended for most projects)")
			fmt.Println("  2) postgres (pgvector, for large monorepos or shared index)")
			fmt.Println("  3) qdrant (Docker-based vector database)")
			fmt.Print("Choice [1]: ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "2", "postgres":
				cfg.Store.Backend = "postgres"
				fmt.Print("PostgreSQL DSN: ")
				dsn, _ := reader.ReadString('\n')
				cfg.Store.Postgres.DSN = strings.TrimSpace(dsn)
			case "3", "qdrant":
				cfg.Store.Backend = "qdrant"
				fmt.Print("Qdrant endpoint [http://localhost:6333]: ")
				endpoint, _ := reader.ReadString('\n')
				endpoint = strings.TrimSpace(endpoint)
				if endpoint == "" {
					endpoint = "http://localhost:6333"
				}
				cfg.Store.Qdrant.Endpoint = endpoint

				fmt.Print("Collection name (optional, defaults to sanitized project path): ")
				collection, _ := reader.ReadString('\n')
				cfg.Store.Qdrant.Collection = strings.TrimSpace(collection)

				fmt.Print("API key (optional, for Qdrant Cloud): ")
				apiKey, _ := reader.ReadString('\n')
				cfg.Store.Qdrant.APIKey = strings.TrimSpace(apiKey)
			default:
				cfg.Store.Backend = "gob"
			}
		} else {
			cfg.Store.Backend = initBackend
		}
	} else {
		// Non-interactive with flags
		if initProvider != "" {
			cfg.Embedder.Provider = initProvider
		}
		if initBackend != "" {
			cfg.Store.Backend = initBackend
		}
	}

	// Save configuration
	if err := cfg.Save(cwd); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\nCreated configuration at %s\n", config.GetConfigPath(cwd))

	// Add .grepai/ to .gitignore
	gitignorePath := cwd + "/.gitignore"
	if _, err := os.Stat(gitignorePath); err == nil {
		if err := indexer.AddToGitignore(cwd, ".grepai/"); err != nil {
			fmt.Printf("Warning: could not update .gitignore: %v\n", err)
		} else {
			fmt.Println("Added .grepai/ to .gitignore")
		}
	}

	fmt.Println("\ngrepai initialized successfully!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Start the indexing daemon: grepai watch")
	fmt.Println("  2. Search your code: grepai search \"your query\"")

	switch cfg.Embedder.Provider {
	case "ollama":
		fmt.Println("\nMake sure Ollama is running with the nomic-embed-text model:")
		fmt.Println("  ollama pull nomic-embed-text")
	case "lmstudio":
		fmt.Println("\nMake sure LM Studio is running with an embedding model loaded.")
		fmt.Printf("  Model: %s\n", cfg.Embedder.Model)
		fmt.Printf("  Endpoint: %s\n", cfg.Embedder.Endpoint)
	case "openai":
		fmt.Println("\nMake sure OPENAI_API_KEY is set in your environment.")
	}

	return nil
}
