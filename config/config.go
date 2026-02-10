package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/git"
	"gopkg.in/yaml.v3"
)

const (
	ConfigDir           = ".grepai"
	ConfigFileName      = "config.yaml"
	IndexFileName       = "index.gob"
	SymbolIndexFileName = "symbols.gob"
	RPGIndexFileName    = "rpg.gob"

	// RPG default configuration values.
	DefaultRPGDriftThreshold       = 0.35
	DefaultRPGMaxTraversalDepth    = 3
	DefaultRPGLLMTimeoutMs         = 8000
	DefaultRPGFeatureMode          = "local"
	DefaultRPGFeatureGroupStrategy = "sample"

	// Watch defaults for RPG realtime updates.
	DefaultWatchRPGPersistIntervalMs      = 1000
	DefaultWatchRPGDerivedDebounceMs      = 300
	DefaultWatchRPGFullReconcileIntervalS = 300
	DefaultWatchRPGMaxDirtyFilesPerBatch  = 128
)

type Config struct {
	Version           int            `yaml:"version"`
	Embedder          EmbedderConfig `yaml:"embedder"`
	Store             StoreConfig    `yaml:"store"`
	Chunking          ChunkingConfig `yaml:"chunking"`
	Watch             WatchConfig    `yaml:"watch"`
	Search            SearchConfig   `yaml:"search"`
	Trace             TraceConfig    `yaml:"trace"`
	RPG               RPGConfig      `yaml:"rpg"`
	Update            UpdateConfig   `yaml:"update"`
	Ignore            []string       `yaml:"ignore"`
	ExternalGitignore string         `yaml:"external_gitignore,omitempty"`
}

// UpdateConfig holds auto-update settings
type UpdateConfig struct {
	CheckOnStartup bool `yaml:"check_on_startup"` // Check for updates when running commands
}

type SearchConfig struct {
	Boost  BoostConfig  `yaml:"boost"`
	Hybrid HybridConfig `yaml:"hybrid"`
}

type HybridConfig struct {
	Enabled bool    `yaml:"enabled"`
	K       float32 `yaml:"k"` // RRF constant (default: 60)
}

type BoostConfig struct {
	Enabled   bool        `yaml:"enabled"`
	Penalties []BoostRule `yaml:"penalties"`
	Bonuses   []BoostRule `yaml:"bonuses"`
}

type BoostRule struct {
	Pattern string  `yaml:"pattern"`
	Factor  float32 `yaml:"factor"`
}

type EmbedderConfig struct {
	Provider    string `yaml:"provider"` // ollama | lmstudio | openai
	Model       string `yaml:"model"`
	Endpoint    string `yaml:"endpoint,omitempty"`
	APIKey      string `yaml:"api_key,omitempty"`
	Dimensions  *int   `yaml:"dimensions,omitempty"`
	Parallelism int    `yaml:"parallelism"` // Number of parallel workers for batch embedding (default: 4)
}

// GetDimensions returns the configured dimensions or a default value.
// For OpenAI, defaults to 1536 (text-embedding-3-small).
// For Ollama/LMStudio, defaults to 768 (nomic-embed-text).
func (e *EmbedderConfig) GetDimensions() int {
	if e.Dimensions != nil {
		return *e.Dimensions
	}
	switch e.Provider {
	case "openai":
		return 1536
	default:
		return 768
	}
}

type StoreConfig struct {
	Backend  string         `yaml:"backend"` // gob | postgres | qdrant
	Postgres PostgresConfig `yaml:"postgres,omitempty"`
	Qdrant   QdrantConfig   `yaml:"qdrant,omitempty"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type QdrantConfig struct {
	Endpoint   string `yaml:"endpoint"`             // e.g., "http://localhost" or "localhost"
	Port       int    `yaml:"port,omitempty"`       // e.g., 6333
	Collection string `yaml:"collection,omitempty"` // Optional, defaults from project path
	APIKey     string `yaml:"api_key,omitempty"`    // Optional, for Qdrant Cloud
	UseTLS     bool   `yaml:"use_tls,omitempty"`    // Enable TLS (for Qdrant Cloud)
}

type ChunkingConfig struct {
	Size    int `yaml:"size"`
	Overlap int `yaml:"overlap"`
}

type WatchConfig struct {
	DebounceMs                  int       `yaml:"debounce_ms"`
	LastIndexTime               time.Time `yaml:"last_index_time,omitempty"`
	RPGPersistIntervalMs        int       `yaml:"rpg_persist_interval_ms,omitempty"`
	RPGDerivedDebounceMs        int       `yaml:"rpg_derived_debounce_ms,omitempty"`
	RPGFullReconcileIntervalSec int       `yaml:"rpg_full_reconcile_interval_sec,omitempty"`
	RPGMaxDirtyFilesPerBatch    int       `yaml:"rpg_max_dirty_files_per_batch,omitempty"`
}

type TraceConfig struct {
	Mode             string   `yaml:"mode"`              // fast or precise
	EnabledLanguages []string `yaml:"enabled_languages"` // File extensions to index
	ExcludePatterns  []string `yaml:"exclude_patterns"`  // Patterns to exclude
}

type RPGConfig struct {
	Enabled              bool    `yaml:"enabled"`
	StorePath            string  `yaml:"store_path,omitempty"`
	FeatureMode          string  `yaml:"feature_mode"` // local | hybrid | llm
	DriftThreshold       float64 `yaml:"drift_threshold"`
	MaxTraversalDepth    int     `yaml:"max_traversal_depth"`
	LLMProvider          string  `yaml:"llm_provider,omitempty"`
	LLMModel             string  `yaml:"llm_model,omitempty"`
	LLMEndpoint          string  `yaml:"llm_endpoint,omitempty"`
	LLMAPIKey            string  `yaml:"llm_api_key,omitempty"`
	LLMTimeoutMs         int     `yaml:"llm_timeout_ms,omitempty"`
	FeatureGroupStrategy string  `yaml:"feature_group_strategy,omitempty"`
}

// ValidateRPGConfig checks RPG configuration values for validity.
func ValidateRPGConfig(cfg RPGConfig) error {
	if cfg.DriftThreshold < 0.0 || cfg.DriftThreshold > 1.0 {
		return fmt.Errorf("rpg.drift_threshold must be between 0.0 and 1.0, got %.2f", cfg.DriftThreshold)
	}
	if cfg.MaxTraversalDepth < 1 || cfg.MaxTraversalDepth > 10 {
		return fmt.Errorf("rpg.max_traversal_depth must be between 1 and 10, got %d", cfg.MaxTraversalDepth)
	}
	switch cfg.FeatureMode {
	case "local", "hybrid", "llm":
		// valid
	default:
		return fmt.Errorf("rpg.feature_mode must be one of: local, hybrid, llm; got %q", cfg.FeatureMode)
	}
	switch cfg.FeatureGroupStrategy {
	case "sample", "split":
		// valid
	default:
		return fmt.Errorf("rpg.feature_group_strategy must be one of: sample, split; got %q", cfg.FeatureGroupStrategy)
	}
	return nil
}

// ValidateWatchConfig checks watch configuration values for validity.
func ValidateWatchConfig(cfg WatchConfig) error {
	if cfg.RPGPersistIntervalMs < 200 {
		return fmt.Errorf("watch.rpg_persist_interval_ms must be >= 200, got %d", cfg.RPGPersistIntervalMs)
	}
	if cfg.RPGDerivedDebounceMs < 100 {
		return fmt.Errorf("watch.rpg_derived_debounce_ms must be >= 100, got %d", cfg.RPGDerivedDebounceMs)
	}
	if cfg.RPGFullReconcileIntervalSec < 30 {
		return fmt.Errorf("watch.rpg_full_reconcile_interval_sec must be >= 30, got %d", cfg.RPGFullReconcileIntervalSec)
	}
	if cfg.RPGMaxDirtyFilesPerBatch < 1 {
		return fmt.Errorf("watch.rpg_max_dirty_files_per_batch must be >= 1, got %d", cfg.RPGMaxDirtyFilesPerBatch)
	}
	return nil
}

func DefaultConfig() *Config {
	defaultDim := 768
	return &Config{
		Version: 1,
		Embedder: EmbedderConfig{
			Provider:   "ollama",
			Model:      "nomic-embed-text",
			Endpoint:   "http://localhost:11434",
			Dimensions: &defaultDim,
			// Parallelism intentionally omitted - only applies to OpenAI
		},
		Store: StoreConfig{
			Backend: "gob",
		},
		Chunking: ChunkingConfig{
			Size:    512,
			Overlap: 50,
		},
		Watch: WatchConfig{
			DebounceMs:                  500,
			RPGPersistIntervalMs:        DefaultWatchRPGPersistIntervalMs,
			RPGDerivedDebounceMs:        DefaultWatchRPGDerivedDebounceMs,
			RPGFullReconcileIntervalSec: DefaultWatchRPGFullReconcileIntervalS,
			RPGMaxDirtyFilesPerBatch:    DefaultWatchRPGMaxDirtyFilesPerBatch,
		},
		Search: SearchConfig{
			Hybrid: HybridConfig{
				Enabled: false,
				K:       60,
			},
			Boost: BoostConfig{
				Enabled: true,
				Penalties: []BoostRule{
					// Test files (multi-language)
					{Pattern: "/tests/", Factor: 0.5},
					{Pattern: "/test/", Factor: 0.5},
					{Pattern: "__tests__", Factor: 0.5},
					{Pattern: "_test.", Factor: 0.5},
					{Pattern: ".test.", Factor: 0.5},
					{Pattern: ".spec.", Factor: 0.5},
					{Pattern: "test_", Factor: 0.5},
					// Mocks
					{Pattern: "/mocks/", Factor: 0.4},
					{Pattern: "/mock/", Factor: 0.4},
					{Pattern: ".mock.", Factor: 0.4},
					// Fixtures & test data
					{Pattern: "/fixtures/", Factor: 0.4},
					{Pattern: "/testdata/", Factor: 0.4},
					// Generated code
					{Pattern: "/generated/", Factor: 0.4},
					{Pattern: ".generated.", Factor: 0.4},
					{Pattern: ".gen.", Factor: 0.4},
					// Documentation
					{Pattern: ".md", Factor: 0.6},
					{Pattern: "/docs/", Factor: 0.6},
				},
				Bonuses: []BoostRule{
					// Entry points (multi-language)
					{Pattern: "/src/", Factor: 1.1},
					{Pattern: "/lib/", Factor: 1.1},
					{Pattern: "/app/", Factor: 1.1},
				},
			},
		},
		Trace: TraceConfig{
			Mode: "fast",
			EnabledLanguages: []string{
				".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php",
				".c", ".h", ".cpp", ".hpp", ".cc", ".cxx",
				".rs", ".zig", ".cs", ".java",
				".pas", ".dpr", // Pascal/Delphi
			},
			ExcludePatterns: []string{
				"*_test.go",
				"*.spec.ts",
				"*.spec.js",
				"*.test.ts",
				"*.test.js",
				"__tests__/*",
			},
		},
		RPG: RPGConfig{
			Enabled:              false,
			FeatureMode:          DefaultRPGFeatureMode,
			DriftThreshold:       DefaultRPGDriftThreshold,
			MaxTraversalDepth:    DefaultRPGMaxTraversalDepth,
			LLMProvider:          "ollama",
			LLMModel:             "",
			LLMEndpoint:          "http://localhost:11434/v1",
			LLMTimeoutMs:         DefaultRPGLLMTimeoutMs,
			FeatureGroupStrategy: DefaultRPGFeatureGroupStrategy,
		},
		Update: UpdateConfig{
			CheckOnStartup: false, // Opt-in by default for privacy
		},
		Ignore: []string{
			".git",
			".grepai",
			"node_modules",
			"vendor",
			"bin",
			"dist",
			"__pycache__",
			".venv",
			"venv",
			".idea",
			".vscode",
			"target",
			".zig-cache",
			"zig-out",
			"qdrant_storage",
		},
	}
}

func GetConfigDir(projectRoot string) string {
	return filepath.Join(projectRoot, ConfigDir)
}

func GetConfigPath(projectRoot string) string {
	return filepath.Join(GetConfigDir(projectRoot), ConfigFileName)
}

func GetIndexPath(projectRoot string) string {
	return filepath.Join(GetConfigDir(projectRoot), IndexFileName)
}

func GetSymbolIndexPath(projectRoot string) string {
	return filepath.Join(GetConfigDir(projectRoot), SymbolIndexFileName)
}

func GetRPGIndexPath(projectRoot string) string {
	return filepath.Join(GetConfigDir(projectRoot), RPGIndexFileName)
}

func Load(projectRoot string) (*Config, error) {
	configPath := GetConfigPath(projectRoot)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for missing values (backward compatibility)
	cfg.applyDefaults()

	// Validate watch timing configuration
	if err := ValidateWatchConfig(cfg.Watch); err != nil {
		return nil, fmt.Errorf("invalid watch configuration: %w", err)
	}

	// Validate RPG config when enabled
	if cfg.RPG.Enabled {
		if err := ValidateRPGConfig(cfg.RPG); err != nil {
			return nil, fmt.Errorf("invalid RPG configuration: %w", err)
		}
	}

	return &cfg, nil
}

// applyDefaults fills in missing configuration values with sensible defaults.
// This ensures backward compatibility with older config files that may not
// have newer fields like dimensions or endpoint.
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()

	// Embedder defaults
	if c.Embedder.Endpoint == "" {
		switch c.Embedder.Provider {
		case "ollama":
			c.Embedder.Endpoint = "http://localhost:11434"
		case "lmstudio":
			c.Embedder.Endpoint = "http://127.0.0.1:1234"
		case "openai":
			c.Embedder.Endpoint = "https://api.openai.com/v1"
		default:
			c.Embedder.Endpoint = defaults.Embedder.Endpoint
		}
	}

	// Only set default dimensions for local embedders (Ollama, LMStudio).
	// For OpenAI, leave nil to let the API use the model's native dimensions.
	if c.Embedder.Dimensions == nil {
		switch c.Embedder.Provider {
		case "ollama":
			dim := 768 // nomic-embed-text default
			c.Embedder.Dimensions = &dim
		case "lmstudio":
			dim := 768 // nomic default
			c.Embedder.Dimensions = &dim
		}
	}

	// Parallelism default (only used by OpenAI embedder)
	if c.Embedder.Parallelism <= 0 {
		c.Embedder.Parallelism = 4
	}

	// Chunking defaults
	if c.Chunking.Size == 0 {
		c.Chunking.Size = defaults.Chunking.Size
	}
	if c.Chunking.Overlap == 0 {
		c.Chunking.Overlap = defaults.Chunking.Overlap
	}

	// Watch defaults
	if c.Watch.DebounceMs == 0 {
		c.Watch.DebounceMs = defaults.Watch.DebounceMs
	}
	if c.Watch.RPGPersistIntervalMs == 0 {
		c.Watch.RPGPersistIntervalMs = defaults.Watch.RPGPersistIntervalMs
	}
	if c.Watch.RPGDerivedDebounceMs == 0 {
		c.Watch.RPGDerivedDebounceMs = defaults.Watch.RPGDerivedDebounceMs
	}
	if c.Watch.RPGFullReconcileIntervalSec == 0 {
		c.Watch.RPGFullReconcileIntervalSec = defaults.Watch.RPGFullReconcileIntervalSec
	}
	if c.Watch.RPGMaxDirtyFilesPerBatch == 0 {
		c.Watch.RPGMaxDirtyFilesPerBatch = defaults.Watch.RPGMaxDirtyFilesPerBatch
	}

	// Qdrant defaults
	if c.Store.Backend == "qdrant" && c.Store.Qdrant.Port <= 0 {
		c.Store.Qdrant.Port = 6334
	}

	// RPG defaults
	if c.RPG.FeatureMode == "" {
		c.RPG.FeatureMode = DefaultRPGFeatureMode
	}
	if c.RPG.DriftThreshold == 0 {
		c.RPG.DriftThreshold = DefaultRPGDriftThreshold
	}
	if c.RPG.MaxTraversalDepth <= 0 {
		c.RPG.MaxTraversalDepth = DefaultRPGMaxTraversalDepth
	}
	if c.RPG.LLMProvider == "" {
		c.RPG.LLMProvider = "ollama"
	}
	// LLMModel intentionally left empty when unset — user must configure
	// explicitly. The watch/mcp code falls back to the local extractor when
	// LLMModel is empty.
	if c.RPG.LLMEndpoint == "" {
		c.RPG.LLMEndpoint = "http://localhost:11434/v1"
	}
	if c.RPG.LLMTimeoutMs <= 0 {
		c.RPG.LLMTimeoutMs = DefaultRPGLLMTimeoutMs
	}
	if c.RPG.FeatureGroupStrategy == "" {
		c.RPG.FeatureGroupStrategy = DefaultRPGFeatureGroupStrategy
	}
}

func (c *Config) Save(projectRoot string) error {
	configDir := GetConfigDir(projectRoot)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := GetConfigPath(projectRoot)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func Exists(projectRoot string) bool {
	configPath := GetConfigPath(projectRoot)
	_, err := os.Stat(configPath)
	return err == nil
}

func FindProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Resolve symlinks to handle symlinked directories
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	dir := cwd
	for {
		if Exists(dir) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Git worktree fallback: if we're in a linked worktree and the main
	// worktree has .grepai/, auto-initialize a local copy for isolation.
	// Each worktree gets its own config + index so search/watch operate
	// on the worktree's own files.
	gitInfo, gitErr := git.Detect(cwd)
	if gitErr == nil && gitInfo.IsWorktree && Exists(gitInfo.MainWorktree) {
		if err := autoInitFromMainWorktree(gitInfo.GitRoot, gitInfo.MainWorktree); err == nil {
			return gitInfo.GitRoot, nil
		}
	}

	return "", fmt.Errorf("no grepai project found (run 'grepai init' first)")
}

// AutoInitWorktree creates a local .grepai/ in worktreeRoot by copying config and
// index files from mainWorktree. This is used by watch to auto-init linked worktrees.
func AutoInitWorktree(worktreeRoot, mainWorktree string) error {
	return autoInitFromMainWorktree(worktreeRoot, mainWorktree)
}

// autoInitFromMainWorktree creates a local .grepai/ in the worktree by copying
// config and index files from the main worktree. This enables zero-config usage:
// search and trace work immediately with the main worktree's index as a seed,
// and watch will incrementally update for worktree-specific changes.
func autoInitFromMainWorktree(worktreeRoot, mainWorktree string) error {
	localGrepai := filepath.Join(worktreeRoot, ".grepai")
	if err := os.MkdirAll(localGrepai, 0755); err != nil {
		return err
	}

	mainGrepai := filepath.Join(mainWorktree, ".grepai")

	// Copy config.yaml (required)
	srcConfig := filepath.Join(mainGrepai, "config.yaml")
	dstConfig := filepath.Join(localGrepai, "config.yaml")
	if err := copyFileIfExists(srcConfig, dstConfig); err != nil {
		os.RemoveAll(localGrepai)
		return err
	}
	// Verify config.yaml was actually copied (it's required)
	if _, err := os.Stat(dstConfig); os.IsNotExist(err) {
		os.RemoveAll(localGrepai)
		return fmt.Errorf("config.yaml not found in main worktree: %s", srcConfig)
	}

	// Copy index.gob as seed (search works immediately)
	_ = copyFileIfExists(
		filepath.Join(mainGrepai, "index.gob"),
		filepath.Join(localGrepai, "index.gob"),
	)

	// Copy symbols.gob as seed (trace works immediately)
	_ = copyFileIfExists(
		filepath.Join(mainGrepai, "symbols.gob"),
		filepath.Join(localGrepai, "symbols.gob"),
	)

	// Ensure .grepai/ is in .gitignore
	ensureGitignoreEntry(worktreeRoot, ".grepai/")

	return nil
}

// copyFileIfExists copies src to dst if src exists. Returns error only if src
// exists but copy fails. Returns nil if src doesn't exist.
func copyFileIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// ensureGitignoreEntry adds an entry to .gitignore if not already present.
func ensureGitignoreEntry(dir, entry string) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		// Check if entry already exists
		for _, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) == entry || strings.TrimSpace(line) == strings.TrimSuffix(entry, "/") {
				return
			}
		}
	}
	// Append entry
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return
		}
	}
	if _, err := f.WriteString(entry + "\n"); err != nil {
		return
	}
}

// FindProjectRootWithGit extends FindProjectRoot with git worktree awareness.
// It first tries the standard .grepai/ directory walk. If found, it also returns
// git worktree info if available. If .grepai/ is not found locally but we're in
// a git worktree, it checks the main worktree for .grepai/config.yaml.
//
// Returns:
//   - projectRoot: the directory containing .grepai/
//   - gitInfo: git worktree detection info (nil if not in a git repo)
//   - err: error if neither local nor main worktree has .grepai/
func FindProjectRootWithGit() (string, *git.DetectInfo, error) {
	// Try standard FindProjectRoot first
	projectRoot, findErr := FindProjectRoot()

	// Get current directory for git detection
	cwd, err := os.Getwd()
	if err != nil {
		if findErr == nil {
			return projectRoot, nil, nil
		}
		return "", nil, findErr
	}

	// Resolve symlinks (same as FindProjectRoot does)
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		if findErr == nil {
			return projectRoot, nil, nil
		}
		return "", nil, findErr
	}

	// Try to detect git info
	gitInfo, gitErr := git.Detect(cwd)
	if gitErr != nil {
		// Not in a git repo - return whatever FindProjectRoot returned
		if findErr == nil {
			return projectRoot, nil, nil
		}
		return "", nil, findErr
	}

	// If we found .grepai/ locally, return it with git info
	if findErr == nil {
		return projectRoot, gitInfo, nil
	}

	// .grepai/ not found locally, but we're in a git repo
	// If we're in a linked worktree, check if main worktree has .grepai/
	if gitInfo.IsWorktree && Exists(gitInfo.MainWorktree) {
		return gitInfo.MainWorktree, gitInfo, nil
	}

	// No .grepai/ found anywhere
	return "", gitInfo, findErr
}
