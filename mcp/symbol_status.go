package mcp

import (
	"context"
	"errors"
	"os"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

// loadStatusProjectConfig loads a workspace project's config for status
// reporting. Defaults apply only when the config is genuinely absent: a
// workspace project can carry a populated symbol index predating any local
// config file. Any other failure — malformed, unreadable, or denied
// traversal — surfaces unchanged rather than silently defaulting.
func loadStatusProjectConfig(projectRoot string) (*config.Config, error) {
	cfg, err := config.Load(projectRoot)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return config.DefaultConfig(), nil
	}
	return nil, err
}

func readAndCloseSymbolStatus(ctx context.Context, symbolStore trace.SymbolStore) (bool, int) {
	defer symbolStore.Close()
	if err := symbolStore.Load(ctx); err != nil {
		return false, 0
	}
	total, err := trace.CountSymbolsForReadiness(ctx, symbolStore)
	if err != nil || total == 0 {
		return false, 0
	}
	return true, total
}
