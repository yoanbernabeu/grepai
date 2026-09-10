package store

import (
	"context"
	"testing"
	"time"
)

func TestPostgresGetCompleteDocumentValidation(t *testing.T) {
	st := newMetadataPostgresStore(t, nil)
	ctx := context.Background()
	other := &PostgresStore{pool: st.pool, projectID: "other-project", dimensions: 3}
	now := time.Now().UTC()

	saveChunk := func(target *PostgresStore, id, path string) {
		t.Helper()
		if err := target.SaveChunks(ctx, []Chunk{
			{ID: id, FilePath: path, Vector: []float32{1, 2, 3}, UpdatedAt: now},
		}); err != nil {
			t.Fatal(err)
		}
	}
	saveDoc := func(path string, ids ...string) {
		t.Helper()
		if ids == nil {
			ids = []string{}
		}
		if err := st.SaveDocument(ctx, Document{Path: path, ModTime: now, ChunkIDs: ids}); err != nil {
			t.Fatal(err)
		}
	}

	saveDoc("zero.go")
	saveChunk(st, "present", "partial.go")
	saveDoc("partial.go", "present", "missing")
	saveChunk(st, "wrong-file", "owner.go")
	saveDoc("wrong.go", "wrong-file")
	saveChunk(other, "foreign", "cross.go")
	saveDoc("cross.go", "foreign")
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO chunks (id, project_id, file_path, start_line, end_line, content, vector, hash, updated_at)
		VALUES ('null-vector', $1, 'null.go', 1, 1, '', NULL, '', $2)`, st.projectID, now); err != nil {
		t.Fatal(err)
	}
	saveDoc("null.go", "null-vector")
	saveChunk(st, "complete-1", "complete.go")
	saveChunk(st, "complete-2", "complete.go")
	saveDoc("complete.go", "complete-1", "complete-2")

	for _, path := range []string{"absent.go", "zero.go", "partial.go", "wrong.go", "cross.go", "null.go"} {
		t.Run(path, func(t *testing.T) {
			got, err := st.GetCompleteDocument(ctx, path)
			if err != nil || got != nil {
				t.Fatalf("GetCompleteDocument(%q) = %+v, %v; want nil, nil", path, got, err)
			}
			if path != "absent.go" {
				metadata, err := st.GetDocument(ctx, path)
				if err != nil || metadata == nil {
					t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
				}
			}
		})
	}

	got, err := st.GetCompleteDocument(ctx, "complete.go")
	if err != nil || got == nil || len(got.ChunkIDs) != 2 {
		t.Fatalf("complete document = %+v, %v", got, err)
	}
	got.ChunkIDs[0] = "mutated"
	again, err := st.GetCompleteDocument(ctx, "complete.go")
	if err != nil || again == nil || again.ChunkIDs[0] != "complete-1" {
		t.Fatalf("chunk IDs were not detached: %+v, %v", again, err)
	}
}

func TestPostgresGetCompleteDocumentWithPrefixValidation(t *testing.T) {
	st := newMetadataPostgresStore(t, nil)
	ctx := context.Background()
	other := &PostgresStore{pool: st.pool, projectID: "other-project", dimensions: 3}
	now := time.Now().UTC()
	const prefix = "ws/proj"

	saveChunk := func(target *PostgresStore, id, path string) {
		t.Helper()
		if err := target.SaveChunks(ctx, []Chunk{
			{ID: id, FilePath: path, Vector: []float32{1, 2, 3}, UpdatedAt: now},
		}); err != nil {
			t.Fatal(err)
		}
	}
	saveNullChunk := func(id, path string) {
		t.Helper()
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO chunks (id, project_id, file_path, start_line, end_line, content, vector, hash, updated_at)
			VALUES ($1, $2, $3, 1, 1, '', NULL, '', $4)`, id, st.projectID, path, now); err != nil {
			t.Fatal(err)
		}
	}
	saveDoc := func(path string, ids ...string) {
		t.Helper()
		if err := st.SaveDocument(ctx, Document{Path: path, ModTime: now, ChunkIDs: ids}); err != nil {
			t.Fatal(err)
		}
	}

	// Complete: raw references resolve to prefixed chunks written by the
	// prefixing wrapper.
	saveChunk(st, "ws/proj/a.go_0", "ws/proj/a.go")
	saveDoc("ws/proj/a.go", "a.go_0")
	// Complete: mixed raw and already-prefixed references.
	saveChunk(st, "ws/proj/b.go_0", "ws/proj/b.go")
	saveChunk(st, "ws/proj/b.go_1", "ws/proj/b.go")
	saveDoc("ws/proj/b.go", "b.go_0", "ws/proj/b.go_1")
	// Complete: an exact-ID row for the wrong file must not block the valid
	// prefixed mapping.
	saveChunk(st, "c.go_0", "elsewhere/c.go")
	saveChunk(st, "ws/proj/c.go_0", "ws/proj/c.go")
	saveDoc("ws/proj/c.go", "c.go_0")
	// Complete: an exact-ID row with a NULL vector must not reject the valid
	// prefixed mapping (per-reference nested NOT EXISTS semantics).
	saveNullChunk("d.go_0", "ws/proj/d.go")
	saveChunk(st, "ws/proj/d.go_0", "ws/proj/d.go")
	saveDoc("ws/proj/d.go", "d.go_0")
	// Incomplete: only a foreign project holds the prefixed chunk.
	saveChunk(other, "ws/proj/e.go_0", "ws/proj/e.go")
	saveDoc("ws/proj/e.go", "e.go_0")
	// Incomplete: prefixed chunk belongs to a different file.
	saveChunk(st, "ws/proj/f.go_0", "ws/proj/other.go")
	saveDoc("ws/proj/f.go", "f.go_0")
	// Incomplete: prefixed chunk has a NULL vector.
	saveNullChunk("ws/proj/g.go_0", "ws/proj/g.go")
	saveDoc("ws/proj/g.go", "g.go_0")
	// Incomplete: an ID merely ending in the reference tail is not a match.
	saveChunk(st, "xtail-ws/proj/h.go_0", "ws/proj/h.go")
	saveDoc("ws/proj/h.go", "h.go_0")
	// Empty prefix matches exact IDs only.
	saveChunk(st, "ws/proj/i.go_0", "ws/proj/i.go")
	saveDoc("ws/proj/i.go", "i.go_0")
	// Empty prefix keeps backslash references literal (no normalization).
	saveChunk(st, "sub/o.go_0", "sub/o.go")
	saveDoc("sub/o.go", `sub\o.go_0`)
	// Complete: a legacy Windows raw reference (native backslashes) resolves
	// to the slash-normalized prefixed chunk the writer stored.
	saveChunk(st, "ws/proj/src/j.go_0", "ws/proj/src/j.go")
	saveDoc("ws/proj/src/j.go", `src\j.go_0`)
	// Complete: mixed raw and already-prefixed Windows references; the full
	// reference normalizes without adding the prefix a second time.
	saveChunk(st, "ws/proj/src/p.go_0", "ws/proj/src/p.go")
	saveChunk(st, "ws/proj/src/p.go_1", "ws/proj/src/p.go")
	saveDoc("ws/proj/src/p.go", `src\p.go_0`, `ws\proj\src\p.go_1`)
	// Incomplete: a normalized full reference owned by another file is
	// rejected; namespace widening never relaxes the same-file check.
	saveChunk(st, "ws/proj/q.go_0", "ws/proj/q.go")
	saveDoc("ws/proj/r.go", `ws\proj\q.go_0`)
	// Complete: a host whose name literally contains backslashes resolves
	// through the literal prefixed candidate.
	saveChunk(st, `ws/proj/l\m.go_0`, `ws/proj/l\m.go`)
	saveDoc(`ws/proj/l\m.go`, `l\m.go_0`)
	// Incomplete: the normalized candidate matches an ID owned by another
	// file; namespace widening never relaxes the same-file check.
	saveChunk(st, "ws/proj/sub/k.go_0", "ws/proj/sub/k.go")
	saveDoc("ws/proj/k.go", `sub\k.go_0`)

	for _, path := range []string{"ws/proj/a.go", "ws/proj/b.go", "ws/proj/c.go", "ws/proj/d.go", "ws/proj/src/j.go", `ws/proj/l\m.go`, "ws/proj/src/p.go"} {
		t.Run("complete/"+path, func(t *testing.T) {
			got, err := st.GetCompleteDocumentWithPrefix(ctx, path, prefix)
			if err != nil || got == nil || got.Path != path {
				t.Fatalf("GetCompleteDocumentWithPrefix(%q) = %+v, %v; want complete", path, got, err)
			}
		})
	}
	for _, path := range []string{"ws/proj/e.go", "ws/proj/f.go", "ws/proj/g.go", "ws/proj/h.go", "ws/proj/k.go", "ws/proj/r.go"} {
		t.Run("incomplete/"+path, func(t *testing.T) {
			got, err := st.GetCompleteDocumentWithPrefix(ctx, path, prefix)
			if err != nil || got != nil {
				t.Fatalf("GetCompleteDocumentWithPrefix(%q) = %+v, %v; want nil, nil", path, got, err)
			}
		})
	}
	t.Run("empty prefix exact only", func(t *testing.T) {
		got, err := st.GetCompleteDocumentWithPrefix(ctx, "ws/proj/i.go", "")
		if err != nil || got != nil {
			t.Fatalf("GetCompleteDocumentWithPrefix(empty prefix) = %+v, %v; want nil, nil", got, err)
		}
	})
	t.Run("empty prefix literal backslash", func(t *testing.T) {
		got, err := st.GetCompleteDocumentWithPrefix(ctx, "sub/o.go", "")
		if err != nil || got != nil {
			t.Fatalf("GetCompleteDocumentWithPrefix(empty prefix) = %+v, %v; want nil, nil", got, err)
		}
	})

	// Document metadata stays intact regardless of completeness verdicts.
	for _, path := range []string{"ws/proj/a.go", "ws/proj/e.go", "ws/proj/i.go"} {
		metadata, err := st.GetDocument(ctx, path)
		if err != nil || metadata == nil {
			t.Fatalf("GetDocument(%q) metadata changed: %+v, %v", path, metadata, err)
		}
	}

	// Returned chunk IDs are detached from the stored document.
	got, err := st.GetCompleteDocumentWithPrefix(ctx, "ws/proj/a.go", prefix)
	if err != nil || got == nil {
		t.Fatalf("complete document = %+v, %v", got, err)
	}
	got.ChunkIDs[0] = "mutated"
	again, err := st.GetCompleteDocumentWithPrefix(ctx, "ws/proj/a.go", prefix)
	if err != nil || again == nil || again.ChunkIDs[0] != "a.go_0" {
		t.Fatalf("chunk IDs were not detached: %+v, %v", again, err)
	}
}
