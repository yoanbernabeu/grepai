package trace

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestPostgresListIndexedFilesIncludesZeroSymbolFiles(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationSymbolStore(t, "inventory-zero-symbols", t.TempDir())
	truncateSymbolTables(t, store)
	if err := store.SaveFileWithSignature(ctx, "full.go", "hash-full", "v-full", []Symbol{{Name: "Full", File: "full.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFileWithContentHash(ctx, "hash-only.go", "hash-orphan", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(ctx, "bare.go", nil, nil); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bare.go", "full.go", "hash-only.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("indexed files = %#v, want %#v", got, want)
	}

	// Empty-symbol files must survive as inventory: they carry no symbols
	// or references, so a symbol-derived enumeration would miss them.
	for _, path := range []string{"bare.go", "hash-only.go"} {
		if syms, err := store.GetSymbolsForFile(ctx, path); err != nil || len(syms) != 0 {
			t.Fatalf("fixture %q unexpectedly has symbols: %#v, %v", path, syms, err)
		}
	}
}

func TestPostgresListIndexedFilesEmptyProject(t *testing.T) {
	store := newIntegrationSymbolStore(t, "inventory-empty", t.TempDir())
	truncateSymbolTables(t, store)
	got, err := store.ListIndexedFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty project inventory = %#v, want empty non-nil slice", got)
	}
}

func TestPostgresListIndexedFilesDetachedSnapshot(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationSymbolStore(t, "inventory-detached", t.TempDir())
	truncateSymbolTables(t, store)
	if err := store.SaveFile(ctx, "a.go", []Symbol{{Name: "A", File: "a.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Later mutations (add and delete) must not alter the snapshot the
	// caller already holds.
	if err := store.SaveFile(ctx, "b.go", []Symbol{{Name: "B", File: "b.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteFile(ctx, "a.go"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, []string{"a.go"}) {
		t.Fatalf("snapshot mutated after store changes: %#v", snapshot)
	}

	current, err := store.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, []string{"b.go"}) {
		t.Fatalf("current inventory = %#v, want [b.go]", current)
	}
}

func TestPostgresListIndexedFilesTenantIsolation(t *testing.T) {
	ctx := context.Background()
	one := newIntegrationSymbolStore(t, "inventory-tenant-one", t.TempDir())
	truncateSymbolTables(t, one)
	two := newIntegrationSymbolStore(t, "inventory-tenant-two", t.TempDir())
	if err := two.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := one.SaveFile(ctx, "same.go", []Symbol{{Name: "OnlyOne", File: "same.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	// A zero-symbol file must not leak across tenants either.
	if err := one.SaveFile(ctx, "orphan-only-one.go", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := two.SaveFile(ctx, "same.go", []Symbol{{Name: "OnlyTwo", File: "same.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}

	oneFiles, err := one.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	twoFiles, err := two.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(oneFiles, []string{"orphan-only-one.go", "same.go"}) {
		t.Fatalf("tenant one inventory leaked: %#v", oneFiles)
	}
	if !reflect.DeepEqual(twoFiles, []string{"same.go"}) {
		t.Fatalf("tenant two inventory leaked: %#v", twoFiles)
	}
}

func TestPostgresListIndexedFilesErrors(t *testing.T) {
	t.Run("closed pool", func(t *testing.T) {
		store := newIntegrationSymbolStore(t, "inventory-closed-pool", t.TempDir())
		store.pool.Close()
		if _, err := store.ListIndexedFiles(context.Background()); err == nil {
			t.Fatal("closed pool unexpectedly succeeded")
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		store := newIntegrationSymbolStore(t, "inventory-cancel", t.TempDir())
		truncateSymbolTables(t, store)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.ListIndexedFiles(ctx); err == nil {
			t.Fatal("canceled context unexpectedly succeeded")
		}
	})
}

func TestPostgresListIndexedFilesSingleQuery(t *testing.T) {
	poolConfig := isolatedSymbolSchemaConfig(t)
	tracer := &symbolFileQueryTracer{}
	poolConfig.ConnConfig.Tracer = tracer
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), poolConfig, "inventory-query-count", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	const files = 60
	for i := 0; i < files; i++ {
		path := fmt.Sprintf("pkg/file%04d.go", i)
		if err := store.SaveFileWithContentHash(context.Background(), path, fmt.Sprintf("hash-%d", i), []Symbol{{Name: fmt.Sprintf("S%d", i), File: path, Line: 1}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	tracer.reset()
	got, err := store.ListIndexedFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != files || tracer.selects != 1 {
		t.Fatalf("inventory size = %d, SELECTs = %d", len(got), tracer.selects)
	}
}

func TestPostgresListIndexedFilesLosslessIdentityBytes(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationSymbolStore(t, "inventory-identity-bytes", t.TempDir())
	truncateSymbolTables(t, store)
	paths := []string{"src/\xff.go", "src/\xfe.go", "plain.go"}
	for _, path := range paths {
		if err := store.SaveFile(ctx, path, []Symbol{{Name: "Thing", File: path, Line: 1}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListIndexedFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"plain.go", "src/\xfe.go", "src/\xff.go"}) {
		t.Fatalf("lossless identity inventory = %#v", got)
	}
}
