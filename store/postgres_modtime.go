package store

import (
	"context"
	"fmt"
	"time"
)

const documentModTimeColumnQuery = `SELECT EXISTS (
	SELECT 1 FROM pg_attribute
	WHERE attrelid = 'documents'::regclass AND attname = 'mod_time_ns' AND NOT attisdropped
)`

func (s *PostgresStore) ensureDocumentModTimeColumn(ctx context.Context) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, documentModTimeColumnQuery).Scan(&exists); err != nil {
		return fmt.Errorf("check documents.mod_time_ns: %w", err)
	}
	if exists {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin documents timestamp migration: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback(context.Background())
		}
	}()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(0x677265706169216)); err != nil {
		return fmt.Errorf("lock documents timestamp migration: %w", err)
	}
	if err := tx.QueryRow(ctx, documentModTimeColumnQuery).Scan(&exists); err != nil {
		return fmt.Errorf("recheck documents.mod_time_ns: %w", err)
	}
	if !exists {
		if _, err := tx.Exec(ctx, `ALTER TABLE documents ADD COLUMN mod_time_ns BIGINT NULL`); err != nil {
			return fmt.Errorf("add documents.mod_time_ns: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit documents timestamp migration: %w", err)
	}
	rollback = false
	return nil
}

func (s *PostgresStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	ns, ok := exactModTimeNanos(modTime)
	if !ok {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `UPDATE documents SET mod_time=$1, mod_time_ns=$2
		WHERE project_id=$3 AND path=$4 AND hash=$5 AND COALESCE(cardinality(chunk_ids),0)>0`, postgresCompatibilityModTime(modTime), ns, s.projectID, path, expectedHash)
	if err != nil {
		return false, fmt.Errorf("refresh document mod time: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

var _ DocumentModTimeRefresher = (*PostgresStore)(nil)
