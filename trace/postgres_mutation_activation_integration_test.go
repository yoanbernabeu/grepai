package trace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func requireProjectDataRows(t *testing.T, store *PostgresSymbolStore, want int) {
	t.Helper()
	var count int
	err := store.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM symbol_files WHERE project_id=$1)+(SELECT count(*) FROM symbols WHERE project_id=$1)+(SELECT count(*) FROM refs WHERE project_id=$1)+(SELECT count(*) FROM call_edges WHERE project_id=$1)`, identityBytes(store.projectID)).Scan(&count)
	if err != nil || count != want {
		t.Fatalf("project data rows=%d, want %d: %v", count, want, err)
	}
}

func TestPostgresMutationRequiresDatabaseActivation(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	root := t.TempDir()
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "activation", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	err = store.SaveFile(context.Background(), "blocked.go", []Symbol{{Name: "Blocked", File: "blocked.go", Line: 1}}, nil)
	if !errors.Is(err, ErrPostgresSymbolStoreActivationRequired) || !strings.Contains(err.Error(), "Load") {
		t.Fatalf("SaveFile error=%T %v", err, err)
	}
	var activationErr *PostgresSymbolStoreActivationRequiredError
	if !errors.As(err, &activationErr) || activationErr.Operation != "save" {
		t.Fatalf("SaveFile typed error=%#v", activationErr)
	}
	requireProjectDataRows(t, store, 0)

	err = store.DeleteFile(context.Background(), "blocked.go")
	if !errors.Is(err, ErrPostgresSymbolStoreActivationRequired) {
		t.Fatalf("DeleteFile error=%T %v", err, err)
	}
	requireProjectDataRows(t, store, 0)

	if err := store.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(context.Background(), "active.go", []Symbol{{Name: "Active", File: "active.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "activation", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	if err := reopened.SaveFile(context.Background(), "second.go", []Symbol{{Name: "Second", File: "second.go", Line: 1}}, nil); err != nil {
		t.Fatalf("database-activated project required another Load: %v", err)
	}
	if err := reopened.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresMutationAfterLegacyGOBLoad(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	root := t.TempDir()
	writeMigrationGOB(t, root, 1)
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "activation-gob", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(context.Background(), "after.go", []Symbol{{Name: "After", File: "after.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "activation-gob", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	if err := reopened.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.LookupSymbol(context.Background(), "After"); err != nil || len(got) != 1 {
		t.Fatalf("post-migration mutation=%#v, %v", got, err)
	}
}

func TestPostgresMutationActivationLockPreventsMarkerRemoval(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	tx, err := store.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := store.requireProjectActivation(context.Background(), tx, "save"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = store.pool.Exec(ctx, `DELETE FROM symbol_migrations WHERE project_id=$1`, identityBytes(store.projectID))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("marker removal while mutation holds activation lock = %v", err)
	}
	stateCtx, stateCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stateCancel()
	_, err = store.pool.Exec(stateCtx, `UPDATE symbol_migrations SET state='migrating' WHERE project_id=$1`, identityBytes(store.projectID))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("activation state change while mutation holds lock = %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(context.Background(), `DELETE FROM symbol_migrations WHERE project_id=$1`, identityBytes(store.projectID)); err != nil {
		t.Fatalf("marker removal after mutation transaction: %v", err)
	}
}
