package trace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

// PostgresSymbolStore stores symbol and trace data incrementally in Postgres.
type PostgresSymbolStore struct {
	pool               *pgxpool.Pool
	schema             string
	projectID          string
	projectRoot        string
	migrationBatchHook func(int) error
	mutationHook       func(string, string) error
	schemaDDLHook      func(int, string) error
}

// SaveFile requires Load to have activated this project in the database before
// the first mutation.
func (s *PostgresSymbolStore) SaveFile(ctx context.Context, filePath string, symbols []Symbol, refs []Reference) error {
	return s.SaveFileWithContentHash(ctx, filePath, "", symbols, refs)
}

// SaveFileWithContentHash requires Load to have activated this project in the
// database before the first mutation.
func (s *PostgresSymbolStore) SaveFileWithContentHash(ctx context.Context, filePath, contentHash string, symbols []Symbol, refs []Reference) error {
	return s.saveFile(ctx, filePath, contentHash, nil, symbols, refs)
}

// SaveFileWithSignature requires the same database activation as SaveFile.
func (s *PostgresSymbolStore) SaveFileWithSignature(ctx context.Context, filePath, contentHash, extractorVersion string, symbols []Symbol, refs []Reference) error {
	return s.saveFile(ctx, filePath, contentHash, &extractorVersion, symbols, refs)
}

func (s *PostgresSymbolStore) saveFile(ctx context.Context, filePath, contentHash string, extractorVersion *string, symbols []Symbol, refs []Reference) (retErr error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin symbol file transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to rollback symbol file transaction: %w", rollbackErr))
		}
	}()
	if err := s.requireProjectActivation(ctx, tx, "save"); err != nil {
		return err
	}
	if err := s.lockFileMutation(ctx, tx, "save", filePath); err != nil {
		return err
	}
	if err := s.saveFileTx(ctx, tx, filePath, contentHash, extractorVersion, symbols, refs); err != nil {
		return err
	}
	if err := s.recordProjectMutation(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit symbol file transaction: %w", err)
	}
	return nil
}

func (s *PostgresSymbolStore) saveFileTx(ctx context.Context, tx pgx.Tx, filePath, contentHash string, extractorVersion *string, symbols []Symbol, refs []Reference) error {
	version := ""
	if extractorVersion != nil {
		version = *extractorVersion
	} else {
		err := tx.QueryRow(ctx, `SELECT extractor_version FROM symbol_files WHERE project_id=$1 AND path=$2`, identityBytes(s.projectID), identityBytes(filePath)).Scan(&version)
		if err != nil && err != pgx.ErrNoRows {
			return fmt.Errorf("failed to read existing extractor version: %w", err)
		}
	}
	if _, err := s.deleteFileTx(ctx, tx, filePath); err != nil {
		return err
	}
	rows := buildPostgresFileRows(s.projectID, filePath, contentHash, version, symbols, refs, time.Now().UTC())
	return copyPostgresRows(ctx, tx, rows)
}

func identityBytes(s string) []byte { return []byte(s) }

// sanUTF8 sanitizes non-identity display values for Postgres TEXT columns.
func sanUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}

// DeleteFile requires Load to have activated this project in the database.
func (s *PostgresSymbolStore) DeleteFile(ctx context.Context, filePath string) (retErr error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin delete transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to rollback delete transaction: %w", rollbackErr))
		}
	}()
	if err := s.requireProjectActivation(ctx, tx, "delete"); err != nil {
		return err
	}
	if err := s.lockFileMutation(ctx, tx, "delete", filePath); err != nil {
		return err
	}
	deleted, err := s.deleteFileTx(ctx, tx, filePath)
	if err != nil {
		return err
	}
	if deleted {
		if err := s.recordProjectMutation(ctx, tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit delete transaction: %w", err)
	}
	return nil
}

func (s *PostgresSymbolStore) deleteFileTx(ctx context.Context, tx pgx.Tx, filePath string) (bool, error) {
	deleted := false
	for _, query := range []string{`DELETE FROM symbols WHERE project_id=$1 AND file=$2`, `DELETE FROM refs WHERE project_id=$1 AND file=$2`, `DELETE FROM call_edges WHERE project_id=$1 AND file=$2`, `DELETE FROM symbol_files WHERE project_id=$1 AND path=$2`} {
		tag, err := tx.Exec(ctx, query, identityBytes(s.projectID), identityBytes(filePath))
		if err != nil {
			return false, fmt.Errorf("failed to delete symbol file data: %w", err)
		}
		deleted = deleted || tag.RowsAffected() != 0
	}
	return deleted, nil
}

func (s *PostgresSymbolStore) IsFileIndexed(filePath string) bool {
	var exists bool
	err := s.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM symbol_files WHERE project_id=$1 AND path=$2)`, identityBytes(s.projectID), identityBytes(filePath)).Scan(&exists)
	return err == nil && exists
}

func (s *PostgresSymbolStore) GetFileContentHash(filePath string) (string, bool) {
	var value string
	err := s.pool.QueryRow(context.Background(), `SELECT content_hash FROM symbol_files WHERE project_id=$1 AND path=$2 AND content_hash<>''`, identityBytes(s.projectID), identityBytes(filePath)).Scan(&value)
	return value, err == nil
}

func (s *PostgresSymbolStore) GetFileExtractorVersion(filePath string) (string, bool) {
	var value string
	err := s.pool.QueryRow(context.Background(), `SELECT extractor_version FROM symbol_files WHERE project_id=$1 AND path=$2 AND extractor_version<>''`, identityBytes(s.projectID), identityBytes(filePath)).Scan(&value)
	return value, err == nil
}

const migrationWriterWaitInterval = 10 * time.Millisecond

// migrationWriterWaitBudget bounds how long Load waits for a contending
// project writer to complete the GOB-to-Postgres migration when the caller's
// context carries no deadline of its own. It bounds only the wait for the
// writer lock / migration marker, never the GOB import once the lock is
// acquired.
const migrationWriterWaitBudget = 30 * time.Second

func (s *PostgresSymbolStore) Load(ctx context.Context) (retErr error) {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}
	// waitCtx bounds only contention on the project writer lock / migration
	// marker; it equals ctx until the first lock rejection. The default
	// budget arms lazily on that first contention — never at Load entry — so
	// schema checks/upgrades and the initial fast-path reads never consume
	// it. Callers with their own deadline keep it; deadline-free callers
	// (e.g. CLI Loads passing context.Background) get a finite default so a
	// stale GOB watcher holding the lifetime lock can never hang Load: such
	// a watcher can no longer publish the Postgres migration marker. Once
	// armed, marker re-checks also run under waitCtx, so a stalled marker
	// query cannot bypass the bound. The GOB import after lock acquisition
	// always runs under the caller's original ctx, never under waitCtx.
	waitCtx := ctx
	budgetArmed := false
	internalBudget := false
	// lastActiveErr retains the most recent contention cause so that a
	// budget expiry observed by a marker re-check (not just by the wait
	// timer) still surfaces the typed active-writer error.
	var lastActiveErr *fileutil.ProjectWriterActiveError
	for {
		completed, err := s.migrationCompletedWithoutGOB(waitCtx)
		if err != nil {
			if internalBudget && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
				return migrationWaitBudgetError(lastActiveErr, err)
			}
			return err
		}
		if completed {
			return nil
		}
		writerLock, err := fileutil.AcquireProjectWriterLock(s.projectRoot)
		if err == nil {
			defer func() { retErr = errors.Join(retErr, writerLock.Close()) }()
			return s.migrateGOBIfNeeded(ctx)
		}
		var activeErr *fileutil.ProjectWriterActiveError
		if !errors.As(err, &activeErr) {
			return fmt.Errorf("failed to acquire project writer lock for Postgres symbol migration: %w", err)
		}
		lastActiveErr = activeErr
		// Another writer holds the project lock: a concurrent initial
		// migration or a live watcher. Wait — bounded by waitCtx — for it to
		// finish the migration or release the lock, then re-check completion
		// so readers never queue behind a lifelong watcher once migration is
		// done, and concurrent Loads serialize instead of failing fast.
		if !budgetArmed {
			budgetArmed = true
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				var waitCancel context.CancelFunc
				waitCtx, waitCancel = context.WithTimeout(ctx, migrationWriterWaitBudget)
				defer waitCancel()
				internalBudget = true
			}
		}
		if err := waitForMigrationWriter(waitCtx, activeErr); err != nil {
			if internalBudget && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
				return migrationWaitBudgetError(activeErr, err)
			}
			return err
		}
	}
}

// migrationWaitBudgetError reports exhaustion of the internal default
// writer-wait budget, whether the expiry was observed by the wait timer or
// by a bounded marker re-check. It preserves the typed active-writer cause
// and the context cause, and tells CLI users how to clear the stale watcher.
func migrationWaitBudgetError(activeErr *fileutil.ProjectWriterActiveError, cause error) error {
	return fmt.Errorf("gave up after %s waiting for the active project writer to finish the Postgres symbol migration; stop or restart the prior grepai watcher holding %s, then retry: %w: %w", migrationWriterWaitBudget, activeErr.LockPath, activeErr, cause)
}

// waitForMigrationWriter blocks one retry interval, mirroring the bounded
// wait in fileutil.AcquireProjectWriterLockContext, so the caller can
// re-observe migration completion between lock attempts.
func waitForMigrationWriter(ctx context.Context, activeErr *fileutil.ProjectWriterActiveError) error {
	timer := time.NewTimer(migrationWriterWaitInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", activeErr, ctx.Err())
	case <-timer.C:
		return nil
	}
}

// LoadWithProjectWriterLockHeld loads migration state while the caller holds
// the project writer lock for the lifetime of this operation.
func (s *PostgresSymbolStore) LoadWithProjectWriterLockHeld(ctx context.Context) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}
	return s.migrateGOBIfNeeded(ctx)
}

// Persist is intentionally a no-op: every Postgres mutation is committed by
// SaveFile*/DeleteFile, so periodic persistence must not rewrite the index.
func (s *PostgresSymbolStore) Persist(context.Context) error { return nil }

func (s *PostgresSymbolStore) Close() error { s.pool.Close(); return nil }

var _ SymbolStore = (*PostgresSymbolStore)(nil)
