package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

// TestProjectPrefixStore_RPGLinksChunksInWorkspace reproduces #368: in a
// workspace, the RPG encoder asks the prefix store for a file's chunks with the
// relative paths that ListDocuments and the scanner return. Those lookups must
// hit the prefixed rows of the shared store, otherwise symbols are never
// linked to their chunks.
func TestProjectPrefixStore_RPGLinksChunksInWorkspace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	shared := &pathKeyedStore{}
	wrapped := &projectPrefixStore{
		store:         shared,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   filepath.Join(dir, "proj"),
	}

	// Another project of the same workspace owns a file with the same
	// relative path: it must never be returned for this project.
	other := &projectPrefixStore{store: shared, workspaceName: "ws", projectName: "other", projectPath: filepath.Join(dir, "other")}
	if err := other.SaveChunks(ctx, []store.Chunk{{ID: "main.go_0", FilePath: "main.go", StartLine: 1, EndLine: 20}}); err != nil {
		t.Fatalf("SaveChunks(other) failed: %v", err)
	}

	// Saved like the indexer does: relative paths and IDs, the wrapper adds
	// the prefix.
	chunks := []store.Chunk{
		{ID: "main.go_0", FilePath: "main.go", StartLine: 1, EndLine: 10},
		{ID: "main.go_1", FilePath: "main.go", StartLine: 11, EndLine: 20},
	}
	if err := wrapped.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := wrapped.SaveDocument(ctx, store.Document{Path: "main.go", Hash: "h", ModTime: time.Now(), ChunkIDs: []string{"main.go_0", "main.go_1"}}); err != nil {
		t.Fatalf("SaveDocument failed: %v", err)
	}

	got, err := wrapped.GetChunksForFile(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetChunksForFile failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetChunksForFile(relative) returned %d chunks, want 2", len(got))
	}
	for i, c := range got {
		if c.FilePath != "main.go" || c.ID != chunks[i].ID {
			t.Errorf("chunk %d = (%q, %q), want the project-relative (%q, %q)", i, c.ID, c.FilePath, chunks[i].ID, "main.go")
		}
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(dir, "symbols.gob"))
	if err := symbolStore.SaveFile(ctx, "main.go", []trace.Symbol{{Name: "Foo", Kind: trace.KindFunction, File: "main.go", Line: 1, EndLine: 20, Language: "go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	rpgStore := rpg.NewGOBRPGStore(filepath.Join(dir, "rpg.gob"))
	encoder := rpg.NewRPGEncoder(rpgStore, rpg.NewLocalExtractor(), wrapped.projectPath, rpg.RPGEncoderConfig{DriftThreshold: 0.35})

	// Initial index (BuildFull) and a later watch event (LinkChunksForFile)
	// must both link the symbol to the two chunks, without accumulating
	// chunk nodes across events.
	if err := encoder.BuildFull(ctx, symbolStore, wrapped, nil); err != nil {
		t.Fatalf("BuildFull failed: %v", err)
	}
	assertChunkLinks(t, "BuildFull", rpgStore.GetGraph(), 2)

	for i := 0; i < 2; i++ {
		linked, err := wrapped.GetChunksForFile(ctx, "main.go")
		if err != nil {
			t.Fatalf("GetChunksForFile failed: %v", err)
		}
		if err := encoder.LinkChunksForFile(ctx, "main.go", linked); err != nil {
			t.Fatalf("LinkChunksForFile failed: %v", err)
		}
		assertChunkLinks(t, "LinkChunksForFile", rpgStore.GetGraph(), 2)
	}
}

func assertChunkLinks(t *testing.T, step string, g *rpg.Graph, want int) {
	t.Helper()
	stats := g.Stats()
	if got := stats.NodesByKind[rpg.KindChunk]; got != want {
		t.Errorf("%s: %d chunk nodes, want %d", step, got, want)
	}
	if got := stats.EdgesByType[rpg.EdgeMapsToChunk]; got != want {
		t.Errorf("%s: %d maps_to_chunk edges, want %d", step, got, want)
	}
	for _, id := range []string{"main.go_0", "main.go_1"} {
		n := g.GetNode(rpg.MakeNodeID(rpg.KindChunk, id))
		if n == nil || n.Path != "main.go" {
			t.Errorf("%s: chunk node %s = %+v, want path %q", step, id, n, "main.go")
		}
	}
}

// pathKeyedStore stands for the shared workspace backends (Postgres, Qdrant):
// it returns a file's chunks by matching the stored file path.
type pathKeyedStore struct {
	mockVectorStore
	chunks []store.Chunk
	docs   []string
}

func (s *pathKeyedStore) SaveChunks(_ context.Context, chunks []store.Chunk) error {
	s.chunks = append(s.chunks, chunks...)
	return nil
}

func (s *pathKeyedStore) SaveDocument(_ context.Context, doc store.Document) error {
	s.docs = append(s.docs, doc.Path)
	return nil
}

func (s *pathKeyedStore) ListDocuments(context.Context) ([]string, error) {
	return s.docs, nil
}

func (s *pathKeyedStore) GetChunksForFile(_ context.Context, filePath string) ([]store.Chunk, error) {
	var out []store.Chunk
	for _, c := range s.chunks {
		if c.FilePath == filePath {
			out = append(out, c)
		}
	}
	return out, nil
}
