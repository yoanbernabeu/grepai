package trace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const readWriteRefsSQL = `SELECT ` + refColumns + ` FROM refs WHERE project_id=$1 AND symbol_name=$2 AND ref_type=ANY($3::text[]) ORDER BY file,line,ordinal`

// LookupCalleeResult reads the target, callee references, and callee
// definitions from one repeatable-read snapshot.
func (s *PostgresSymbolStore) LookupCalleeResult(ctx context.Context, symbolName, _ string) (result CalleeLookupResult, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, fmt.Errorf("failed to begin callee result snapshot transaction: %w", err)
	}
	defer rollbackReadResult(tx, "callee result", &err)()
	refs, err := s.lookupCallees(ctx, tx, symbolName)
	if err != nil {
		return result, err
	}
	symbols, err := s.lookupSymbolsBatch(ctx, tx, referenceDefinitionNames(symbolName, refs, false))
	if err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("failed to commit callee result snapshot transaction: %w", err)
	}
	return CalleeLookupResult{Symbols: symbols, References: refs}, nil
}

// LookupRefsResult reads both access kinds and their caller definitions from
// one repeatable-read snapshot.
func (s *PostgresSymbolStore) LookupRefsResult(ctx context.Context, symbolName string) (result RefsLookupResult, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, fmt.Errorf("failed to begin refs result snapshot transaction: %w", err)
	}
	defer rollbackReadResult(tx, "refs result", &err)()
	refs, err := s.lookupReadWriteRefs(ctx, tx, symbolName)
	if err != nil {
		return result, err
	}
	symbols, err := s.lookupSymbolsBatch(ctx, tx, referenceDefinitionNames("", refs, true))
	if err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("failed to commit refs result snapshot transaction: %w", err)
	}
	return RefsLookupResult{Symbols: symbols, References: refs}, nil
}

func (s *PostgresSymbolStore) lookupReadWriteRefs(ctx context.Context, q postgresQuerier, symbolName string) ([]Reference, error) {
	rows, err := q.Query(ctx, readWriteRefsSQL, identityBytes(s.projectID), identityBytes(symbolName), []string{RefKindRead, RefKindWrite})
	if err != nil {
		return nil, fmt.Errorf("failed to lookup read/write references: %w", err)
	}
	defer rows.Close()
	return scanRefs(rows)
}

func rollbackReadResult(tx pgx.Tx, label string, resultErr *error) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			*resultErr = errors.Join(*resultErr, fmt.Errorf("failed to rollback %s snapshot transaction: %w", label, err))
		}
	}
}
