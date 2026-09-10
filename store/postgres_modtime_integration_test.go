package store

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type modTimeMigrationTracer struct{ alters atomic.Int64 }

func (m *modTimeMigrationTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "ALTER TABLE documents ADD COLUMN mod_time_ns") {
		m.alters.Add(1)
	}
	return ctx
}

func (*modTimeMigrationTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresDocumentExactModTimeRoundTripAndLegacyWriterInvalidation(t *testing.T) {
	st := newMetadataPostgresStore(t, nil)
	ctx := context.Background()
	want := time.Unix(1_700_000_000, 123456789)
	doc := Document{Path: "a.go", Hash: "hash", ModTime: want, HasExactModTime: true, ChunkIDs: []string{"c1"}}
	if err := st.SaveDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetDocument(ctx, doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasExactModTime || !got.ModTime.Equal(want) {
		t.Fatalf("round trip = %v exact=%v", got.ModTime, got.HasExactModTime)
	}
	legacyWrite := want.Add(2 * time.Second)
	if _, err := st.pool.Exec(ctx, `UPDATE documents SET mod_time=$1 WHERE project_id=$2 AND path=$3`, legacyWrite, st.projectID, doc.Path); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetDocument(ctx, doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasExactModTime || got.ModTime.Equal(want) {
		t.Fatalf("legacy write = %v exact=%v", got.ModTime, got.HasExactModTime)
	}
}

func TestPostgresRefreshDocumentModTimeIsChunkSafeAndProjectScoped(t *testing.T) {
	st := newMetadataPostgresStore(t, nil)
	ctx := context.Background()
	other := &PostgresStore{pool: st.pool, projectID: "other-project", dimensions: st.dimensions}
	base := time.Unix(1_700_000_000, 1)
	for _, target := range []*PostgresStore{st, other} {
		if err := target.SaveDocument(ctx, Document{Path: "a.go", Hash: "hash", ModTime: base, HasExactModTime: true, ChunkIDs: []string{"c1"}}); err != nil {
			t.Fatal(err)
		}
	}
	want := base.Add(987654321 * time.Nanosecond)
	updated, err := st.RefreshDocumentModTime(ctx, "a.go", "hash", want)
	if err != nil || !updated {
		t.Fatalf("refresh = %v, %v", updated, err)
	}
	mine, err := st.GetDocument(ctx, "a.go")
	if err != nil || mine == nil {
		t.Fatalf("read refreshed document: doc=%v err=%v", mine, err)
	}
	foreign, err := other.GetDocument(ctx, "a.go")
	if err != nil || foreign == nil {
		t.Fatalf("read foreign document: doc=%v err=%v", foreign, err)
	}
	if !mine.ModTime.Equal(want) || len(mine.ChunkIDs) != 1 || mine.ChunkIDs[0] != "c1" {
		t.Fatalf("local document changed unexpectedly: %+v", mine)
	}
	if !foreign.ModTime.Equal(base) {
		t.Fatalf("foreign project timestamp changed: %v", foreign.ModTime)
	}
	if updated, err := st.RefreshDocumentModTime(ctx, "a.go", "wrong", want.Add(time.Second)); err != nil || updated {
		t.Fatalf("wrong-hash refresh = %v, %v", updated, err)
	}
	if err := st.SaveDocument(ctx, Document{Path: "empty.go", Hash: "hash", ModTime: base, HasExactModTime: true, ChunkIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if updated, err := st.RefreshDocumentModTime(ctx, "empty.go", "hash", want); err != nil || updated {
		t.Fatalf("zero-chunk refresh = %v, %v", updated, err)
	}
}

func TestPostgresModTimeMigrationIsGuardedUnderConcurrentInitializers(t *testing.T) {
	tracer := &modTimeMigrationTracer{}
	st := newMetadataPostgresStore(t, tracer)
	ctx := context.Background()
	if _, err := st.pool.Exec(ctx, `ALTER TABLE documents DROP COLUMN mod_time_ns`); err != nil {
		t.Fatal(err)
	}
	tracer.alters.Store(0)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- st.ensureDocumentModTimeColumn(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := tracer.alters.Load(); got != 1 {
		t.Fatalf("ALTER executions = %d, want 1", got)
	}
}

var _ pgx.QueryTracer = (*modTimeMigrationTracer)(nil)
