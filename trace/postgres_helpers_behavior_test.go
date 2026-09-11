package trace

import (
	"context"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

type schemaExecutorStub struct {
	calls  int
	failAt int
}

func (s *schemaExecutorStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	s.calls++
	if s.failAt > 0 && s.calls == s.failAt {
		return pgconn.CommandTag{}, errors.New("DDL failed")
	}
	return pgconn.NewCommandTag("CREATE TABLE"), nil
}

func TestExecuteSymbolSchemaQueriesSuccessAndFailures(t *testing.T) {
	plan := symbolSchemaPlan("test_schema", symbolSchemaInventory{
		tables: make(map[string]schemaTable), indexes: make(map[string]schemaIndex),
	}, symbolSchemaFresh)
	success := &schemaExecutorStub{}

	if err := executeSymbolSchemaQueries(context.Background(), success, plan, nil); err != nil {
		t.Fatal(err)
	}
	if success.calls != len(plan) {
		t.Fatalf("executed %d DDL statements, want %d", success.calls, len(plan))
	}

	hookCalls := 0
	err := executeSymbolSchemaQueries(context.Background(), &schemaExecutorStub{}, plan, func(int, string) error {
		hookCalls++
		return errors.New("hook failed")
	})
	if err == nil || hookCalls != 1 {
		t.Fatalf("hook failure = %v, calls=%d", err, hookCalls)
	}
	databaseFailure := &schemaExecutorStub{failAt: 2}
	if err := executeSymbolSchemaQueries(context.Background(), databaseFailure, plan, nil); err == nil || databaseFailure.calls != 2 {
		t.Fatalf("database failure = %v, calls=%d", err, databaseFailure.calls)
	}
}

func TestMigrationFileAndFingerprintHelpers(t *testing.T) {
	// Given an exact source snapshot.
	path := filepath.Join(t.TempDir(), "symbols.gob")
	if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := fingerprintSource(path)
	if err != nil {
		t.Fatal(err)
	}
	size := fingerprint.size
	status := migrationStatus{state: "completed", sourceDigest: fingerprint.digest, sourceSize: &size}

	// When the marker matches, verification succeeds.
	if err := verifyCompletedSource(status, fingerprint); err != nil {
		t.Fatal(err)
	}
	exists, err := fileExists(path)
	if err != nil || !exists {
		t.Fatalf("existing source = %v, %v", exists, err)
	}
	missing, err := fileExists(path + ".missing")
	if err != nil || missing {
		t.Fatalf("missing source = %v, %v", missing, err)
	}

	// Then missing or mismatched marker data fails closed.
	if err := verifyCompletedSource(migrationStatus{}, fingerprint); err == nil {
		t.Fatal("missing fingerprint accepted")
	}
	wrong := sourceFingerprint{digest: append([]byte(nil), fingerprint.digest...), size: fingerprint.size}
	wrong.digest[0] ^= 0xff
	if err := verifyCompletedSource(status, wrong); err == nil {
		t.Fatal("mismatched fingerprint accepted")
	}
	if _, err := fingerprintSource(path + ".missing"); err == nil {
		t.Fatal("missing source fingerprint succeeded")
	}
	exists, err = fileExists(filepath.Join(path, "child"))
	if err != nil || exists {
		t.Fatalf("child of file = %v, %v; want false,nil", exists, err)
	}
}

func TestArchiveMigratedGOBSuccessAndFailure(t *testing.T) {
	// Given a source and an old replaceable backup.
	path := filepath.Join(t.TempDir(), "symbols.gob")
	if err := os.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".migrated.bak", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When archived, the new snapshot replaces the backup.
	if err := archiveMigratedGOB(path); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path + ".migrated.bak"); err != nil || string(data) != "new" {
		t.Fatalf("archive = %q, %v", data, err)
	}

	// Then a missing source reports an actionable failure.
	if err := archiveMigratedGOB(path); err == nil {
		t.Fatal("missing source archive succeeded")
	}
}

func TestLoadLockedGOBSnapshotNormalizesNilCollections(t *testing.T) {
	// Given a valid legacy snapshot with omitted maps and slices.
	path := filepath.Join(t.TempDir(), "symbols.gob")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gob.NewEncoder(file).Encode(gobSymbolData{}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	// When decoded, all runtime collections are usable.
	store, loaded, err := loadLockedGOBSymbolSnapshot(path)
	if err != nil || !loaded {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if store.index.Symbols == nil || store.index.References == nil || store.index.CallGraph == nil || store.fileIndex == nil || store.fileContentHashes == nil || store.fileExtractorVersions == nil {
		t.Fatal("legacy nil collections were not normalized")
	}
}

func TestWaitForMigrationWriterCanceledContextWrapsActiveError(t *testing.T) {
	// Given a project whose writer lock is contended by a second handle.
	root := t.TempDir()
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	_, busyErr := fileutil.AcquireProjectWriterLock(root)
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	var activeErr *fileutil.ProjectWriterActiveError
	if !errors.As(busyErr, &activeErr) {
		t.Fatalf("second acquire error = %T %v, want *ProjectWriterActiveError", busyErr, busyErr)
	}

	// When the wait observes an already-canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = waitForMigrationWriter(ctx, activeErr)

	// Then both the context cause and the typed busy cause are retained.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForMigrationWriter() error = %v, want errors.Is(context.Canceled)", err)
	}
	if !errors.As(err, &activeErr) {
		t.Fatalf("waitForMigrationWriter() error = %v, want typed writer-active cause", err)
	}
}
