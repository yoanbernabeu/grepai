package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/git"
	"github.com/yoanbernabeu/grepai/indexer"
)

var (
	initProvider       string
	initModel          string
	initBackend        string
	initNonInteractive bool
	initInherit        bool
)

const (
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
	initCmd.Flags().StringVarP(&initProvider, "provider", "p", "", "Embedding provider (ollama, lmstudio, openai, synthetic, or openrouter)")
	initCmd.Flags().StringVarP(&initModel, "model", "m", "", "Embedding model (for openrouter: text-embedding-3-small, text-embedding-3-large, qwen3-embedding-8b)")
	initCmd.Flags().StringVarP(&initBackend, "backend", "b", "", "Storage backend (gob, postgres, or qdrant)")
	initCmd.Flags().BoolVar(&initNonInteractive, "yes", false, "Use defaults without prompting")
	initCmd.Flags().BoolVar(&initInherit, "inherit", false, "Inherit configuration from main worktree (for git worktrees)")
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
	skipPrompts := false

	// Detect git worktree and offer config inheritance
	gitInfo, gitErr := git.Detect(cwd)
	if gitErr == nil && gitInfo.IsWorktree && config.Exists(gitInfo.MainWorktree) {
		mainCfg, loadErr := config.Load(gitInfo.MainWorktree)
		if loadErr == nil {
			fmt.Printf("\nGit worktree detected.\n")
			fmt.Printf("  Main worktree: %s\n", gitInfo.MainWorktree)
			fmt.Printf("  Worktree ID:   %s\n", gitInfo.WorktreeID)
			fmt.Printf("  Backend:       %s\n", mainCfg.Store.Backend)

			shouldInherit := initInherit
			if !shouldInherit && !initNonInteractive {
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("\nInherit configuration from main worktree? [Y/n]: ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToLower(input))
				shouldInherit = input == "" || input == "y" || input == "yes"
			}

			if shouldInherit {
				cfg = mainCfg
				skipPrompts = true

				if cfg.Store.Backend == "gob" {
					fmt.Println("\nNote: GOB backend creates an independent index per worktree.")
					fmt.Println("For shared indexing across worktrees, consider using 'postgres' or 'qdrant' backend.")
				} else {
					fmt.Printf("\nUsing %s backend - each worktree maintains its own project scope within the shared store.\n", cfg.Store.Backend)
				}
			}
		} else {
			fmt.Printf("Warning: could not load main worktree config: %v\n", loadErr)
		}
	}

	// Interactive mode
	if !skipPrompts && !initNonInteractive {
		reader := bufio.NewReader(os.Stdin)

		// Provider selection
		if initProvider == "" {
			fmt.Println("\nSelect embedding provider:")
			fmt.Println("  1) ollama (local, privacy-first, requires Ollama running)")
			fmt.Println("  2) lmstudio (local, OpenAI-compatible, requires LM Studio running)")
			fmt.Println("  3) openai (cloud, requires API key)")
			fmt.Println("  4) synthetic (cloud, free embedding API)")
			fmt.Println("  5) openrouter (cloud, multi-provider gateway)")
			fmt.Print("Choice [1]: ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "2", "lmstudio":
				cfg.Embedder.Provider = "lmstudio"
				fmt.Print("LM Studio endpoint [http://127.0.0.1:1234]: ")
				endpoint, _ := reader.ReadString('\n')
				endpoint = strings.TrimSpace(endpoint)
				if endpoint == "" {
					endpoint = "http://127.0.0.1:1234"
				}
				cfg.Embedder.Endpoint = endpoint
				cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
				dim := lmStudioEmbeddingDimensions
				cfg.Embedder.Dimensions = &dim
			case "3", "openai":
				cfg.Embedder.Provider = "openai"
				cfg.Embedder.Model = "text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://api.openai.com/v1"
				// OpenAI: leave Dimensions nil to use model's native dimensions
			case "4", "synthetic":
				cfg.Embedder.Provider = "synthetic"
				cfg.Embedder.Model = "hf:nomic-ai/nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "https://api.synthetic.new/openai/v1"
				dim := 768
				cfg.Embedder.Dimensions = &dim
			case "5", "openrouter":
				cfg.Embedder.Provider = "openrouter"
				cfg.Embedder.Endpoint = "https://openrouter.ai/api/v1"
				// OpenRouter: leave Dimensions nil to use model's native dimensions

				// Model selection for OpenRouter
				fmt.Println("\nSelect OpenRouter embedding model:")
				fmt.Println("  1) openai/text-embedding-3-small (1536 dims, fast, recommended)")
				fmt.Println("  2) openai/text-embedding-3-large (3072 dims, most capable)")
				fmt.Println("  3) qwen/qwen3-embedding-8b (4096 dims, 32K context, best for code)")
				fmt.Print("Choice [1]: ")

				modelInput, _ := reader.ReadString('\n')
				modelInput = strings.TrimSpace(modelInput)

				switch modelInput {
				case "2":
					cfg.Embedder.Model = "openai/text-embedding-3-large"
				case "3":
					cfg.Embedder.Model = "qwen/qwen3-embedding-8b"
				default:
					cfg.Embedder.Model = "openai/text-embedding-3-small"
				}
			default:
				cfg.Embedder.Provider = "ollama"
				fmt.Print("Ollama endpoint [http://localhost:11434]: ")
				endpoint, _ := reader.ReadString('\n')
				endpoint = strings.TrimSpace(endpoint)
				if endpoint == "" {
					endpoint = "http://localhost:11434"
				}
				cfg.Embedder.Endpoint = endpoint
			}
		} else {
			cfg.Embedder.Provider = initProvider
			switch initProvider {
			case "lmstudio":
				cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "http://127.0.0.1:1234"
				dim := lmStudioEmbeddingDimensions
				cfg.Embedder.Dimensions = &dim
			case "openai":
				cfg.Embedder.Model = "text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://api.openai.com/v1"
				// OpenAI: leave Dimensions nil to use model's native dimensions
			case "synthetic":
				cfg.Embedder.Model = "hf:nomic-ai/nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "https://api.synthetic.new/openai/v1"
				dim := 768
				cfg.Embedder.Dimensions = &dim
			case "openrouter":
				cfg.Embedder.Model = "openai/text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://openrouter.ai/api/v1"
				// OpenRouter: leave Dimensions nil to use model's native dimensions
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
				fmt.Print("Qdrant endpoint [localhost]: ")
				endpoint, _ := reader.ReadString('\n')
				endpoint = strings.TrimSpace(endpoint)
				if endpoint == "" {
					endpoint = "localhost"
				}
				cfg.Store.Qdrant.Endpoint = endpoint

				fmt.Print("Qdrant port [6334]: ")
				port, _ := reader.ReadString('\n')
				port = strings.TrimSpace(port)
				if port == "" {
					cfg.Store.Qdrant.Port = 6334
				} else {
					var portInt int
					_, err := fmt.Sscanf(port, "%d", &portInt)
					if err != nil {
						return fmt.Errorf("invalid port number: %w", err)
					}
					cfg.Store.Qdrant.Port = portInt
				}

				fmt.Print("Use TLS? (y/n) [n]: ")
				useTLS, _ := reader.ReadString('\n')
				useTLS = strings.TrimSpace(strings.ToLower(useTLS))
				cfg.Store.Qdrant.UseTLS = useTLS == "y" || useTLS == "yes"

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
	} else if !skipPrompts {
		// Non-interactive with flags
		if initProvider != "" {
			cfg.Embedder.Provider = initProvider
			// Apply provider-specific settings
			switch initProvider {
			case "lmstudio":
				cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "http://127.0.0.1:1234"
				dim := lmStudioEmbeddingDimensions
				cfg.Embedder.Dimensions = &dim
			case "openai":
				cfg.Embedder.Model = "text-embedding-3-small"
				cfg.Embedder.Endpoint = "https://api.openai.com/v1"
				cfg.Embedder.Dimensions = nil
			case "synthetic":
				cfg.Embedder.Model = "hf:nomic-ai/nomic-embed-text-v1.5"
				cfg.Embedder.Endpoint = "https://api.synthetic.new/openai/v1"
				dim := 768
				cfg.Embedder.Dimensions = &dim
			case "openrouter":
				cfg.Embedder.Endpoint = "https://openrouter.ai/api/v1"
				cfg.Embedder.Dimensions = nil
				// Use provided model flag or default
				switch initModel {
				case "text-embedding-3-large":
					cfg.Embedder.Model = "openai/text-embedding-3-large"
				case "qwen3-embedding-8b":
					cfg.Embedder.Model = "qwen/qwen3-embedding-8b"
				default:
					cfg.Embedder.Model = "openai/text-embedding-3-small"
				}
			}
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
	case "synthetic":
		fmt.Println("\nMake sure SYNTHETIC_API_KEY or OPENAI_API_KEY is set in your environment.")
		fmt.Println("  Get your free API key at: https://api.synthetic.new")
	case "openrouter":
		fmt.Println("\nMake sure OPENROUTER_API_KEY or OPENAI_API_KEY is set in your environment.")
		fmt.Println("  Get your API key at: https://openrouter.ai/keys")
	}

	return nil
}
