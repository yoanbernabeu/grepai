package trace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const currentSymbolSchemaVersion = 3
const symbolSchemaVersionQuery = `SELECT value FROM symbol_store_meta WHERE key='schema_version'`

// ErrSymbolSchemaVersionTooNew marks a symbol store whose stored schema
// version is newer than this build understands.
var ErrSymbolSchemaVersionTooNew = errors.New("symbol store schema version is newer than this build supports")

type symbolSchemaVersionReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readSymbolSchemaVersion(ctx context.Context, reader symbolSchemaVersionReader) (version int, tableMissing bool, err error) {
	err = reader.QueryRow(ctx, symbolSchemaVersionQuery).Scan(&version)
	if err == nil {
		return version, false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return 0, true, nil
	}
	return 0, false, fmt.Errorf("failed to read symbol schema version: %w", err)
}

func schemaAdvisoryKey() (int32, int32) {
	return advisoryKey("grepai:symbol-schema")
}

// checkSymbolSchemaVersion prevents an older binary from downgrading a newer
// schema's version marker or running incompatible DDL against it.
func checkSymbolSchemaVersion(version int) error {
	if version > currentSymbolSchemaVersion {
		return fmt.Errorf("%w: stored %d, supported %d", ErrSymbolSchemaVersionTooNew, version, currentSymbolSchemaVersion)
	}
	return nil
}

func (s *PostgresSymbolStore) ensureSchema(ctx context.Context) (retErr error) {
	version, _, err := readSymbolSchemaVersion(ctx, s.pool)
	if err != nil {
		return err
	}
	if err := checkSymbolSchemaVersion(version); err != nil {
		return err
	}
	if version == currentSymbolSchemaVersion {
		current, err := s.validateCurrentSymbolSchema(ctx)
		if err != nil {
			return err
		}
		if current {
			return nil
		}
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire symbol schema connection: %w", err)
	}
	defer conn.Release()
	key1, key2 := schemaAdvisoryKey()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1,$2)`, key1, key2); err != nil {
		return fmt.Errorf("failed to acquire symbol schema advisory lock: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, releaseSchemaAdvisoryLock(conn, key1, key2)) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin symbol schema transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to rollback symbol schema transaction: %w", rollbackErr))
		}
	}()

	inv, err := inspectSymbolSchema(ctx, tx, s.schema)
	if err != nil {
		return err
	}
	markerPresent, version, err := inspectSchemaMarker(ctx, tx, inv)
	if err != nil {
		return err
	}
	if err := checkSymbolSchemaVersion(version); err != nil {
		return err
	}
	if version == currentSymbolSchemaVersion {
		if err := inv.validateCurrentLayout(); err != nil {
			return err
		}
		if inv.hasAllIndexes() {
			return tx.Commit(ctx)
		}
	}
	ownership, err := classifySymbolSchema(inv, markerPresent, version)
	if err != nil {
		return err
	}
	if ownership != symbolSchemaFresh {
		if err := lockOwnedSymbolTables(ctx, tx, s.schema, inv, symbolMigrationNeedsDDL(inv)); err != nil {
			return err
		}
		inv, err = inspectSymbolSchema(ctx, tx, s.schema)
		if err != nil {
			return err
		}
		markerPresent, version, err = inspectSchemaMarker(ctx, tx, inv)
		if err != nil {
			return err
		}
		if err := checkSymbolSchemaVersion(version); err != nil {
			return err
		}
		if version == currentSymbolSchemaVersion {
			if err := inv.validateCurrentLayout(); err != nil {
				return err
			}
			if inv.hasAllIndexes() {
				return tx.Commit(ctx)
			}
		}
		ownership, err = classifySymbolSchema(inv, markerPresent, version)
		if err != nil {
			return err
		}
	}
	queries := symbolSchemaPlan(s.schema, inv, ownership)
	if version == currentSymbolSchemaVersion {
		queries = missingIndexQueries(s.schema, inv)
	}
	if err := executeSymbolSchemaQueries(ctx, tx, queries, s.schemaDDLHook); err != nil {
		return err
	}
	meta := pgx.Identifier{s.schema, "symbol_store_meta"}.Sanitize()
	if _, err := tx.Exec(ctx, `INSERT INTO `+meta+`(key,value) VALUES('schema_version',$1) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`, currentSymbolSchemaVersion); err != nil {
		return fmt.Errorf("failed to record symbol schema version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit symbol schema transaction: %w", err)
	}
	return nil
}

func (s *PostgresSymbolStore) validateCurrentSymbolSchema(ctx context.Context) (current bool, retErr error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return false, fmt.Errorf("failed to begin current symbol schema validation: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to rollback current symbol schema validation: %w", rollbackErr))
		}
	}()
	inv, err := inspectSymbolSchema(ctx, tx, s.schema)
	if err != nil {
		return false, err
	}
	markerPresent, version, err := inspectSchemaMarker(ctx, tx, inv)
	if err != nil {
		return false, err
	}
	if err := checkSymbolSchemaVersion(version); err != nil {
		return false, err
	}
	if !markerPresent || version != currentSymbolSchemaVersion {
		return false, nil
	}
	if err := inv.validateCurrentLayout(); err != nil {
		return false, err
	}
	if !inv.hasAllIndexes() {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit current symbol schema validation: %w", err)
	}
	return true, nil
}

func inspectSchemaMarker(ctx context.Context, tx pgx.Tx, inv symbolSchemaInventory) (bool, int, error) {
	if _, ok := inv.tables["symbol_store_meta"]; !ok {
		return false, 0, nil
	}
	if err := validateTable("symbol_store_meta", inv.tables["symbol_store_meta"], [][]columnRule{metaShape}, []string{"key"}); err != nil {
		return false, 0, err
	}
	var version int
	err := tx.QueryRow(ctx, symbolSchemaVersionQuery).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to read symbol schema version: %w", err)
	}
	return true, version, nil
}

// symbolTableLockOrder follows the mutation hierarchy: project metadata first,
// then data tables in their exact write order. Metadata upgrades therefore
// wait for old and new writers before holding any data-table lock.
var symbolTableLockOrder = []string{
	"symbol_migrations", "symbols", "refs", "call_edges", "symbol_files", "symbol_store_meta",
}

func lockOwnedSymbolTables(ctx context.Context, tx pgx.Tx, schema string, inv symbolSchemaInventory, migrationMetadataExclusive bool) error {
	for _, name := range symbolTableLockOrder {
		if _, ok := inv.tables[name]; !ok {
			continue
		}
		mode := "SHARE ROW EXCLUSIVE"
		if name == "symbol_migrations" {
			mode = "ROW SHARE"
			if migrationMetadataExclusive {
				mode = "ACCESS EXCLUSIVE"
			}
		}
		if _, err := tx.Exec(ctx, `LOCK TABLE `+pgx.Identifier{schema, name}.Sanitize()+` IN `+mode+` MODE`); err != nil {
			return fmt.Errorf("failed to lock owned symbol table %q: %w", name, err)
		}
	}
	return nil
}

func releaseSchemaAdvisoryLock(conn *pgxpool.Conn, key1, key2 int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1,$2)`, key1, key2).Scan(&unlocked); err != nil {
		return fmt.Errorf("failed to release symbol schema advisory lock: %w", err)
	}
	if !unlocked {
		return fmt.Errorf("symbol schema advisory lock was not held during release")
	}
	return nil
}
