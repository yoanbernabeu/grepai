package trace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// LookupCallerResult reads the target, caller references, and caller
// definitions from one repeatable-read snapshot.
func (s *PostgresSymbolStore) LookupCallerResult(ctx context.Context, symbolName string) (result CallerLookupResult, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CallerLookupResult{}, fmt.Errorf("failed to begin caller snapshot transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("failed to rollback caller snapshot transaction: %w", rollbackErr))
		}
	}()

	refs, err := s.lookupRefs(ctx, tx, symbolName, RefKindCall)
	if err != nil {
		return CallerLookupResult{}, err
	}
	names := make([]string, 0, len(refs)+1)
	names = append(names, symbolName)
	for _, ref := range refs {
		names = append(names, ref.CallerName)
	}
	symbols, err := s.lookupSymbolsBatch(ctx, tx, names)
	if err != nil {
		return CallerLookupResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CallerLookupResult{}, fmt.Errorf("failed to commit caller snapshot transaction: %w", err)
	}
	return CallerLookupResult{Symbols: symbols, References: refs}, nil
}
