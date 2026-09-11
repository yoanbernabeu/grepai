package trace

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/config"
)

// NewSymbolStore creates the configured symbol store for a project.
func NewSymbolStore(ctx context.Context, cfg *config.Config, projectRoot string) (SymbolStore, error) {
	return NewSymbolStoreWithWorkspace(ctx, cfg, projectRoot, nil)
}

// NewSymbolStoreWithWorkspace creates a symbol store with optional workspace
// store settings used as a Postgres DSN fallback.
func NewSymbolStoreWithWorkspace(ctx context.Context, cfg *config.Config, projectRoot string, workspaceStore *config.StoreConfig) (SymbolStore, error) {
	backend, err := symbolStoreBackend(cfg.Trace.StoreBackend)
	if err != nil {
		return nil, err
	}
	if backend == "gob" {
		return NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot)), nil
	}

	dsn, source := resolveSymbolPostgresDSN(cfg, workspaceStore)
	log.Printf("trace: using Postgres DSN from %s", source)
	canonicalRoot, err := canonicalSymbolPostgresRoot(projectRoot)
	if err != nil {
		return nil, err
	}
	return NewPostgresSymbolStore(ctx, dsn, canonicalRoot, canonicalRoot)
}

// canonicalSymbolPostgresRoot matches the canonical project identity used by
// workspace writers. Existing rows written under a legacy symlink alias are
// not migrated automatically; selecting an alias migration policy remains an
// explicit owner operation rather than risking an unexpected namespace merge.
func canonicalSymbolPostgresRoot(projectRoot string) (string, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to make trace postgres project root absolute: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve trace postgres project root: %w", err)
	}
	return filepath.Clean(canonicalRoot), nil
}

func symbolStoreBackend(backend string) (string, error) {
	if backend == "" {
		return "gob", nil
	}
	if backend != "gob" && backend != "postgres" {
		return "", fmt.Errorf("unknown trace storage backend: %s", backend)
	}
	return backend, nil
}

func resolveSymbolPostgresDSN(cfg *config.Config, workspaceStore *config.StoreConfig) (string, string) {
	if cfg.Trace.Postgres.DSN != "" {
		return cfg.Trace.Postgres.DSN, "trace.postgres.dsn"
	}
	if workspaceStore != nil && workspaceStore.Postgres.DSN != "" {
		return workspaceStore.Postgres.DSN, "workspace store.postgres.dsn"
	}
	if cfg.Store.Postgres.DSN != "" {
		return cfg.Store.Postgres.DSN, "project store.postgres.dsn"
	}
	return config.DefaultPostgresDSN, "default Postgres DSN"
}
