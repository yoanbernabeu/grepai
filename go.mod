module github.com/yoanbernabeu/grepai

go 1.24.2

require (
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/kenshaw/snaker v0.4.3 // indirect
	github.com/mark3labs/mcp-go v0.43.2
	github.com/pgvector/pgvector-go v0.3.0
	github.com/qdrant/go-client v1.16.2
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

// Exclude the separate javascript submodule to use the one from the main module
exclude github.com/smacker/go-tree-sitter/javascript v0.0.1

// Exclude the separate csharp submodule to use the one from the main module
exclude github.com/smacker/go-tree-sitter/csharp v0.0.0-20240827094217-dd81d9e9be82
