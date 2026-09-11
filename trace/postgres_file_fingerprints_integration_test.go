package trace

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresListFileFingerprintsSingleSnapshot(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationSymbolStore(t, "fingerprint-snapshot", t.TempDir())
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
	got, err := store.ListFileFingerprints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("snapshot size = %d, want 3 (got %#v)", len(got), got)
	}
	wantFull := FileFingerprint{ContentHash: "hash-full", ExtractorVersion: "v-full", HasContentHash: true, HasExtractorVersion: true}
	if got["full.go"] != wantFull {
		t.Fatalf("full.go = %#v, want %#v", got["full.go"], wantFull)
	}
	wantHashOnly := FileFingerprint{ContentHash: "hash-orphan", HasContentHash: true}
	hashOnly, hasHashOnly := got["hash-only.go"]
	bare, hasBare := got["bare.go"]
	if !hasHashOnly || !hasBare || hashOnly != wantHashOnly || bare != (FileFingerprint{}) {
		t.Fatalf("legacy snapshots = %#v", got)
	}
}

func TestPostgresListFileFingerprintsLegacyEmptyStringFingerprint(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationSymbolStore(t, "fingerprint-legacy", t.TempDir())
	truncateSymbolTables(t, store)
	if err := store.SaveFile(ctx, "legacy.go", []Symbol{{Name: "Legacy", File: "legacy.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFileFingerprints(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	legacy, present := got["legacy.go"]
	if len(got) != 1 || !present || legacy != (FileFingerprint{}) {
		t.Fatalf("legacy snapshot = %#v", got)
	}
}

func TestPostgresListFileFingerprintsTenantIsolation(t *testing.T) {
	ctx := context.Background()
	one := newIntegrationSymbolStore(t, "fingerprint-tenant-one", t.TempDir())
	truncateSymbolTables(t, one)
	two := newIntegrationSymbolStore(t, "fingerprint-tenant-two", t.TempDir())
	if err := two.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := one.SaveFileWithContentHash(ctx, "same.go", "hash-one", []Symbol{{Name: "OnlyOne", File: "same.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := two.SaveFileWithContentHash(ctx, "same.go", "hash-two", []Symbol{{Name: "OnlyTwo", File: "same.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	oneFingerprints, err := one.ListFileFingerprints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	twoFingerprints, err := two.ListFileFingerprints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(oneFingerprints) != 1 || len(twoFingerprints) != 1 || oneFingerprints["same.go"].ContentHash != "hash-one" || twoFingerprints["same.go"].ContentHash != "hash-two" {
		t.Fatalf("cross-tenant fingerprints leaked: %#v / %#v", oneFingerprints, twoFingerprints)
	}
}

func TestPostgresListFileFingerprintsErrors(t *testing.T) {
	t.Run("closed pool", func(t *testing.T) {
		store := newIntegrationSymbolStore(t, "fingerprint-closed-pool", t.TempDir())
		store.pool.Close()
		if _, err := store.ListFileFingerprints(context.Background()); err == nil {
			t.Fatal("closed pool unexpectedly succeeded")
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		store := newIntegrationSymbolStore(t, "fingerprint-cancel", t.TempDir())
		truncateSymbolTables(t, store)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.ListFileFingerprints(ctx); err == nil {
			t.Fatal("canceled context unexpectedly succeeded")
		}
	})
}

func TestPostgresListFileFingerprintsLargeSnapshotUsesSingleQuery(t *testing.T) {
	poolConfig := isolatedSymbolSchemaConfig(t)
	tracer := &symbolFileQueryTracer{}
	poolConfig.ConnConfig.Tracer = tracer
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), poolConfig, "fingerprint-query-count", t.TempDir())
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
	got, err := store.ListFileFingerprints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != files || tracer.selects != 1 {
		t.Fatalf("snapshot size = %d, SELECTs = %d", len(got), tracer.selects)
	}
}

type symbolFileQueryTracer struct {
	mu      sync.Mutex
	selects int
}

func (t *symbolFileQueryTracer) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.selects = 0
}

func (t *symbolFileQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	if upper := strings.ToUpper(data.SQL); strings.Contains(upper, "SELECT") && strings.Contains(upper, "FROM SYMBOL_FILES") {
		t.selects++
	}
	return ctx
}

func (*symbolFileQueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

var _ pgx.QueryTracer = (*symbolFileQueryTracer)(nil)
