package trace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type migrationStatus struct {
	state        string
	sourceDigest []byte
	sourceSize   *int64
}

type sourceFingerprint struct {
	digest []byte
	size   int64
}

func releaseMigrationAdvisoryLock(conn *pgxpool.Conn, key1, key2 int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1,$2)`, key1, key2).Scan(&unlocked); err != nil {
		return fmt.Errorf("failed to release symbol migration advisory lock: %w", err)
	}
	if !unlocked {
		return fmt.Errorf("symbol migration advisory lock was not held during release")
	}
	return nil
}

func (s *PostgresSymbolStore) migrationState(ctx context.Context, conn *pgxpool.Conn) (migrationStatus, error) {
	var status migrationStatus
	var size pgtype.Int8
	err := conn.QueryRow(ctx, `SELECT state,source_digest,source_size FROM symbol_migrations WHERE project_id=$1`, identityBytes(s.projectID)).Scan(&status.state, &status.sourceDigest, &size)
	if err == pgx.ErrNoRows {
		return migrationStatus{}, nil
	}
	if err != nil {
		return migrationStatus{}, fmt.Errorf("failed to read symbol migration state: %w", err)
	}
	if size.Valid {
		status.sourceSize = &size.Int64
	}
	return status, nil
}

func (s *PostgresSymbolStore) projectDataRows(ctx context.Context, conn *pgxpool.Conn) (int64, error) {
	var rows int64
	err := conn.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM symbols WHERE project_id=$1)+(SELECT COUNT(*) FROM refs WHERE project_id=$1)+(SELECT COUNT(*) FROM call_edges WHERE project_id=$1)+(SELECT COUNT(*) FROM symbol_files WHERE project_id=$1)`, identityBytes(s.projectID)).Scan(&rows)
	if err != nil {
		return 0, fmt.Errorf("failed to check Postgres symbol migration eligibility: %w", err)
	}
	return rows, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	// Go reports both ENOENT and Windows ERROR_PATH_NOT_FOUND as ErrNotExist.
	// ENOTDIR means a non-directory was used as a directory; for a source path
	// inspection this is still "not present", and must not fail the check.
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect GOB symbol index: %w", err)
}

func fingerprintSource(path string) (fingerprint sourceFingerprint, retErr error) {
	file, err := os.Open(path)
	if err != nil {
		return sourceFingerprint{}, fmt.Errorf("failed to open GOB symbol source for fingerprinting: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return sourceFingerprint{}, fmt.Errorf("failed to fingerprint GOB symbol source: %w", err)
	}
	return sourceFingerprint{digest: hash.Sum(nil), size: size}, nil
}

func verifyCompletedSource(status migrationStatus, fingerprint sourceFingerprint) error {
	if len(status.sourceDigest) != sha256.Size || status.sourceSize == nil {
		return fmt.Errorf("completed Postgres symbol migration has no source fingerprint; refusing to archive residual GOB")
	}
	if !bytes.Equal(status.sourceDigest, fingerprint.digest) || *status.sourceSize != fingerprint.size {
		return fmt.Errorf("residual GOB differs from the completed Postgres symbol migration source; refusing to archive unimported data")
	}
	return nil
}

func archiveMigratedGOB(path string) error {
	backup := path + ".migrated.bak"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Postgres symbol migration completed, but old backup could not be removed: %w", err)
	}
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("Postgres symbol migration completed, but source GOB could not be archived as %s: %w", backup, err)
	}
	return nil
}
