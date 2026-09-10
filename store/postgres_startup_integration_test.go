package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var metadataSchemaCounter atomic.Uint64

func newMetadataPostgresStore(t *testing.T, tracer pgx.QueryTracer) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("grepai_metadata_%d_%d", os.Getpid(), metadataSchemaCounter.Add(1))
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+identifier+` CASCADE`)
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ", public"
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	st := &PostgresStore{pool: pool, projectID: "metadata-project", dimensions: 3}
	if err := st.ensureSchema(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

type startupQueryCounter struct {
	mu    sync.Mutex
	count int
}

func (q *startupQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.mu.Lock()
	q.count++
	q.mu.Unlock()
	return ctx
}
func (*startupQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (q *startupQueryCounter) reset()                                                        { q.mu.Lock(); q.count = 0; q.mu.Unlock() }
func (q *startupQueryCounter) calls() int                                                    { q.mu.Lock(); defer q.mu.Unlock(); return q.count }

func TestPostgresListDocumentMetadataUsesOneQueryAndTenantScope(t *testing.T) {
	tracer := &startupQueryCounter{}
	st := newMetadataPostgresStore(t, tracer)
	other := &PostgresStore{pool: st.pool, projectID: "other", dimensions: 3}
	ctx := context.Background()
	for i := range 40 {
		chunks := []string{"chunk"}
		if i == 2 {
			chunks = []string{}
		}
		if err := st.SaveDocument(ctx, Document{Path: fmt.Sprintf("file-%02d.go", i), Hash: "hash", ModTime: time.Now(), ChunkIDs: chunks}); err != nil {
			t.Fatal(err)
		}
	}
	if err := other.SaveDocument(ctx, Document{Path: "foreign.go", Hash: "foreign", ModTime: time.Now(), ChunkIDs: []string{"chunk"}}); err != nil {
		t.Fatal(err)
	}
	tracer.reset()
	metadata, err := st.ListDocumentMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 40 || tracer.calls() != 1 {
		t.Fatalf("metadata=%d queries=%d", len(metadata), tracer.calls())
	}
	for _, m := range metadata {
		if m.Path == "foreign.go" {
			t.Fatal("foreign project metadata leaked")
		}
		if m.Path == "file-02.go" && m.HasChunks {
			t.Fatal("zero-chunk document reported chunks")
		}
	}
}

func TestPostgresExactModTimeRoundTripAndCAS(t *testing.T) {
	st := newMetadataPostgresStore(t, nil)
	ctx := context.Background()
	want := time.Unix(1_700_000_000, 123456789)
	if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "hash", ModTime: want, HasExactModTime: true, ChunkIDs: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := st.GetDocument(ctx, "a.go")
	if err != nil || doc == nil || !doc.HasExactModTime || !doc.ModTime.Equal(want) {
		t.Fatalf("doc=%+v err=%v", doc, err)
	}
	updatedTime := want.Add(time.Second)
	if updated, err := st.RefreshDocumentModTime(ctx, "a.go", "hash", updatedTime); err != nil || !updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	if updated, err := st.RefreshDocumentModTime(ctx, "a.go", "wrong", updatedTime.Add(time.Second)); err != nil || updated {
		t.Fatalf("wrong hash updated=%v err=%v", updated, err)
	}
}

var _ pgx.QueryTracer = (*startupQueryCounter)(nil)
