package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	ConfigDir      = ".grepai"
	ConfigFileName = "config.yaml"
	IndexFileName  = "index.gob"
)

type Config struct {
	Version  int            `yaml:"version"`
	Embedder EmbedderConfig `yaml:"embedder"`
	Store    StoreConfig    `yaml:"store"`
	Chunking ChunkingConfig `yaml:"chunking"`
	Watch    WatchConfig    `yaml:"watch"`
	Ignore   []string       `yaml:"ignore"`
}

type EmbedderConfig struct {
	Provider string `yaml:"provider"` // ollama | lmstudio | openai
	Model    string `yaml:"model"`
	Endpoint string `yaml:"endpoint,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
}

type StoreConfig struct {
	Backend  string         `yaml:"backend"` // gob | postgres
	Postgres PostgresConfig `yaml:"postgres,omitempty"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type ChunkingConfig struct {
	Size    int `yaml:"size"`
	Overlap int `yaml:"overlap"`
}

type WatchConfig struct {
	DebounceMs int `yaml:"debounce_ms"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		Embedder: EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
			Endpoint: "http://localhost:11434",
		},
		Store: StoreConfig{
			Backend: "gob",
		},
		Chunking: ChunkingConfig{
			Size:    512,
			Overlap: 50,
		},
		Watch: WatchConfig{
			DebounceMs: 500,
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

	return &cfg, nil
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

	return "", fmt.Errorf("no grepai project found (run 'grepai init' first)")
}
