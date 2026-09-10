package store

import (
	"context"
	"testing"
)

func TestGOBStoreGetCompleteDocument(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		doc    *Document
		chunks []Chunk
		want   bool
	}{
		{name: "absent"},
		{name: "zero IDs", doc: &Document{Path: "a.go"}},
		{
			name: "one of two missing",
			doc:  &Document{Path: "a.go", ChunkIDs: []string{"c1", "c2"}},
			chunks: []Chunk{
				{ID: "c1", FilePath: "a.go", Vector: []float32{1}},
			},
		},
		{
			name:   "wrong file",
			doc:    &Document{Path: "a.go", ChunkIDs: []string{"c1"}},
			chunks: []Chunk{{ID: "c1", FilePath: "other/a.go", Vector: []float32{1}}},
		},
		{
			name:   "cross-project identifier",
			doc:    &Document{Path: "a.go", ChunkIDs: []string{"workspace/two/a.go_0"}},
			chunks: []Chunk{{ID: "workspace/two/a.go_0", FilePath: "workspace/two/a.go", Vector: []float32{1}}},
		},
		{
			name:   "empty vector",
			doc:    &Document{Path: "a.go", ChunkIDs: []string{"c1"}},
			chunks: []Chunk{{ID: "c1", FilePath: "a.go"}},
		},
		{
			name: "complete",
			doc:  &Document{Path: "a.go", ChunkIDs: []string{"c1", "c2"}},
			chunks: []Chunk{
				{ID: "c1", FilePath: "a.go", Vector: []float32{1}},
				{ID: "c2", FilePath: "a.go", Vector: []float32{2}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := NewGOBStore(t.TempDir() + "/index.gob")
			if err := st.SaveChunks(ctx, tt.chunks); err != nil {
				t.Fatal(err)
			}
			if tt.doc != nil {
				if err := st.SaveDocument(ctx, *tt.doc); err != nil {
					t.Fatal(err)
				}
			}
			got, err := st.GetCompleteDocument(ctx, "a.go")
			if err != nil || (got != nil) != tt.want {
				t.Fatalf("GetCompleteDocument() = %+v, %v; want present=%v", got, err, tt.want)
			}
			if tt.doc != nil {
				metadata, err := st.GetDocument(ctx, "a.go")
				if err != nil || metadata == nil {
					t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
				}
			}
			if got != nil {
				got.ChunkIDs[0] = "mutated"
				again, _ := st.GetCompleteDocument(ctx, "a.go")
				if again.ChunkIDs[0] != "c1" {
					t.Fatalf("returned chunk IDs alias store: %v", again.ChunkIDs)
				}
			}
		})
	}
}

func TestGOBStoreGetCompleteDocumentWithPrefix(t *testing.T) {
	ctx := context.Background()
	const (
		docPath = "ws/proj/src/a.go"
		prefix  = "ws/proj"
		rawID   = "src/a.go_0"
		fullID  = "ws/proj/src/a.go_0"
	)
	validPrefixed := Chunk{ID: fullID, FilePath: docPath, Vector: []float32{1}}
	tests := []struct {
		name   string
		ids    []string
		prefix string
		chunks []Chunk
		want   bool
	}{
		{
			name:   "raw references match prefixed chunks",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{validPrefixed},
			want:   true,
		},
		{
			name:   "mixed raw and full references",
			ids:    []string{rawID, "ws/proj/src/a.go_1"},
			prefix: prefix,
			chunks: []Chunk{
				validPrefixed,
				{ID: "ws/proj/src/a.go_1", FilePath: docPath, Vector: []float32{2}},
			},
			want: true,
		},
		{
			name:   "exact collision wrong file still uses valid prefixed",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{
				validPrefixed,
				{ID: rawID, FilePath: "src/other.go", Vector: []float32{9}},
			},
			want: true,
		},
		{
			name:   "exact collision empty vector still uses valid prefixed",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{
				validPrefixed,
				{ID: rawID, FilePath: docPath},
			},
			want: true,
		},
		{
			name:   "prefixed chunk wrong file",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{{ID: fullID, FilePath: "ws/proj/src/other.go", Vector: []float32{1}}},
		},
		{
			name:   "prefixed chunk empty vector",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{{ID: fullID, FilePath: docPath}},
		},
		{
			name:   "other prefix suffix tail not accepted",
			ids:    []string{rawID},
			prefix: prefix,
			chunks: []Chunk{{ID: "tail-" + fullID, FilePath: docPath, Vector: []float32{1}}},
		},
		{
			name:   "empty prefix matches exact only",
			ids:    []string{rawID},
			chunks: []Chunk{validPrefixed},
		},
		{
			name:   "exact match without prefix",
			ids:    []string{rawID},
			chunks: []Chunk{{ID: rawID, FilePath: docPath, Vector: []float32{1}}},
			want:   true,
		},
		{
			// Legacy Windows metadata recorded raw references with native
			// backslash separators while the writer stored slash-normalized
			// prefixed IDs; the prefixed candidate resolves them.
			name:   "windows raw reference resolves to slash-stored prefixed chunk",
			ids:    []string{`src\a.go_0`},
			prefix: prefix,
			chunks: []Chunk{validPrefixed},
			want:   true,
		},
		{
			// Already-prefixed Windows references normalize without adding the
			// prefix a second time, alongside raw Windows references.
			name:   "mixed raw and full windows references",
			ids:    []string{`src\a.go_0`, `ws\proj\src\a.go_1`},
			prefix: prefix,
			chunks: []Chunk{
				validPrefixed,
				{ID: "ws/proj/src/a.go_1", FilePath: docPath, Vector: []float32{2}},
			},
			want: true,
		},
		{
			// A normalized full reference still requires the same owning file.
			name:   "full windows reference wrong owner rejected",
			ids:    []string{`ws\proj\other.go_0`},
			prefix: prefix,
			chunks: []Chunk{{ID: "ws/proj/other.go_0", FilePath: "ws/proj/other.go", Vector: []float32{1}}},
		},
		{
			// Normalization only widens the namespace lookup: an ID match via
			// the normalized candidate still requires the same owning file.
			name:   "windows reference normalized candidate wrong file",
			ids:    []string{`sub\b.go_0`},
			prefix: prefix,
			chunks: []Chunk{{ID: "ws/proj/sub/b.go_0", FilePath: "ws/proj/sub/b.go", Vector: []float32{1}}},
		},
		{
			// Hosts whose names literally contain backslashes keep resolving
			// through the literal prefixed candidate.
			name:   "literal backslash reference resolves to literal stored chunk",
			ids:    []string{`src\a.go_0`},
			prefix: prefix,
			chunks: []Chunk{{ID: `ws/proj/src\a.go_0`, FilePath: docPath, Vector: []float32{1}}},
			want:   true,
		},
		{
			// An empty prefix never rewrites the reference, so a backslash
			// reference does not match a slash-spelled stored ID.
			name:   "empty prefix keeps windows reference literal",
			ids:    []string{`src\a.go_0`},
			chunks: []Chunk{{ID: rawID, FilePath: docPath, Vector: []float32{1}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := NewGOBStore(t.TempDir() + "/index.gob")
			if err := st.SaveChunks(ctx, tt.chunks); err != nil {
				t.Fatal(err)
			}
			doc := Document{Path: docPath, Hash: "hash", ChunkIDs: tt.ids}
			if err := st.SaveDocument(ctx, doc); err != nil {
				t.Fatal(err)
			}
			got, err := st.GetCompleteDocumentWithPrefix(ctx, docPath, tt.prefix)
			if err != nil || (got != nil) != tt.want {
				t.Fatalf("GetCompleteDocumentWithPrefix() = %+v, %v; want present=%v", got, err, tt.want)
			}
			metadata, err := st.GetDocument(ctx, docPath)
			if err != nil || metadata == nil || metadata.Hash != "hash" {
				t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
			}
			if got != nil {
				if got.Path != docPath {
					t.Fatalf("document path = %q", got.Path)
				}
				got.ChunkIDs[0] = "mutated"
				again, _ := st.GetCompleteDocumentWithPrefix(ctx, docPath, tt.prefix)
				if again.ChunkIDs[0] != tt.ids[0] {
					t.Fatalf("returned chunk IDs alias store: %v", again.ChunkIDs)
				}
			}
		})
	}
}

func TestGOBStoreGetCompleteDocumentAfterReload(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/index.gob"
	st := NewGOBStore(path)
	if err := st.SaveChunks(ctx, []Chunk{{ID: "c1", FilePath: "a.go", Vector: []float32{1}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDocument(ctx, Document{Path: "a.go", ChunkIDs: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Persist(ctx); err != nil {
		t.Fatal(err)
	}
	reloaded := NewGOBStore(path)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := reloaded.GetCompleteDocument(ctx, "a.go"); err != nil || got == nil {
		t.Fatalf("reloaded document = %+v, %v", got, err)
	}
}
