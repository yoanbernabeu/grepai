package cli

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/yoanbernabeu/grepai/trace"
)

type projectWriterLockHeldSymbolLoader interface {
	LoadWithProjectWriterLockHeld(context.Context) error
}

// requiredSymbolStoreInitError marks symbol store factory and PostgreSQL
// load/migration failures that must abort workspace startup: continuing
// would publish readiness with the affected project silently omitted.
type requiredSymbolStoreInitError struct {
	cause error
}

func (e *requiredSymbolStoreInitError) Error() string { return e.cause.Error() }
func (e *requiredSymbolStoreInitError) Unwrap() error { return e.cause }

func isRequiredSymbolStoreInitError(err error) bool {
	var reqErr *requiredSymbolStoreInitError
	return errors.As(err, &reqErr)
}

func runAfterWatcherSymbolLoad(ctx context.Context, backend, project string, symbolStore trace.SymbolStore, next func() error) error {
	load := symbolStore.Load
	if backend == "postgres" {
		if lockHeldLoader, ok := symbolStore.(projectWriterLockHeldSymbolLoader); ok {
			load = lockHeldLoader.LoadWithProjectWriterLockHeld
		}
	}
	if err := load(ctx); err != nil {
		if backend == "postgres" {
			return &requiredSymbolStoreInitError{
				cause: fmt.Errorf("failed to load Postgres symbol index for %s: %w", project, err),
			}
		}
		log.Printf("Warning: failed to load symbol index for %s: %v", project, err)
	}
	if next == nil {
		return nil
	}
	err := next()
	if err != nil && backend == "postgres" && !isRequiredSymbolStoreInitError(err) {
		// A failed Postgres initial scan leaves the index incomplete; like a
		// load/migration failure it must abort startup, not drop the project.
		return &requiredSymbolStoreInitError{cause: err}
	}
	return err
}

func initializeWorkspaceSymbolStore(ctx context.Context, backend, project string, symbolStore trace.SymbolStore, scan func() error) error {
	if err := runAfterWatcherSymbolLoad(ctx, backend, project, symbolStore, scan); err != nil {
		return errors.Join(err, symbolStore.Close())
	}
	return nil
}
