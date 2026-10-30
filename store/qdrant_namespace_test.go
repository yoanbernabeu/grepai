package store

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/protobuf/proto"
)

func TestQdrantProjectNamespace(t *testing.T) {
	ns := func(collection, root string) string {
		t.Helper()
		return QdrantProjectNamespace(collection, root)
	}
	if got := ns("", "/work/a"); got != "" {
		t.Fatalf("derived collection got namespace %q, want none", got)
	}
	tempDir := func() string {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return dir
	}
	plain := tempDir()
	if got := ns("shared", plain); got != plain {
		t.Fatalf("explicit collection outside git got namespace %q, want the project root %q", got, plain)
	}
	// A symlinked root (as an MCP root parameter may be) names the same
	// project as its resolved path, which is what the CLI uses.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := ns("shared", link+string(filepath.Separator)); got != plain {
		t.Fatalf("symlinked root got namespace %q, want %q", got, plain)
	}
	// Without git on PATH, the path is used (git's absence is not fatal).
	t.Run("git not installed", func(t *testing.T) {
		t.Setenv("PATH", "")
		dir := tempDir()
		if got := ns("shared", dir); got != dir {
			t.Fatalf("namespace without git = %q, want %q", got, dir)
		}
	})
	// In a repository with an origin remote, the namespace is the repository,
	// whatever the checkout path, and holds no credential.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := tempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://token@github.com/Org/App.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if got := ns("shared", repo); got != "github.com/Org/App" {
		t.Fatalf("git project got namespace %q, want github.com/Org/App", got)
	}
	if got := ns("", repo); got != "" {
		t.Fatalf("derived collection got namespace %q", got)
	}
}

// Collections without a namespace must keep the point IDs and payloads of
// earlier versions so that existing indexes are reused as they are.
func TestQdrantLegacyLayoutUnchanged(t *testing.T) {
	legacy := &QdrantStore{}
	want := uuid.NewSHA1(uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), []byte("README.md_0"))
	if got := legacy.getUUIDForChunk("README.md_0"); got != want {
		t.Fatalf("legacy point ID changed: got %v, want %v", got, want)
	}
	payload, err := legacy.buildChunkPayload(Chunk{FilePath: "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := payload[qdrantNamespaceField]; ok {
		t.Fatal("legacy payload gained a project namespace")
	}
	if f := legacy.filter(); f != nil {
		t.Fatalf("legacy unfiltered read got filter %v", f)
	}
	want1 := &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewMatch("file_path", "README.md")}}
	if f := legacy.filter(qdrant.NewMatch("file_path", "README.md")); !proto.Equal(f, want1) {
		t.Fatalf("legacy filter = %v, want %v", f, want1)
	}
	if f := legacy.filterWithLegacy(qdrant.NewMatch("file_path", "README.md")); !proto.Equal(f, want1) {
		t.Fatalf("legacy delete filter = %v, want %v", f, want1)
	}
}

func TestQdrantNamespacedLayout(t *testing.T) {
	a := &QdrantStore{namespace: "/work/a"}
	b := &QdrantStore{namespace: "/work/b"}
	legacy := &QdrantStore{}
	if a.getUUIDForChunk("README.md_0") == b.getUUIDForChunk("README.md_0") {
		t.Fatal("two projects got the same point ID for README.md_0")
	}
	if a.getUUIDForChunk("README.md_0") == legacy.getUUIDForChunk("README.md_0") {
		t.Fatal("namespaced point ID collides with the legacy one")
	}
	payload, err := a.buildChunkPayload(Chunk{FilePath: "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got := payload[qdrantNamespaceField].GetStringValue(); got != "/work/a" {
		t.Fatalf("payload namespace = %q", got)
	}
	ns := qdrant.NewMatch(qdrantNamespaceField, "/work/a")
	if f, want := a.filter(), (&qdrant.Filter{Must: []*qdrant.Condition{ns}}); !proto.Equal(f, want) {
		t.Fatalf("filter = %v, want %v", f, want)
	}
	path := qdrant.NewMatch("file_path", "README.md")
	want := &qdrant.Filter{Must: []*qdrant.Condition{path}, Should: []*qdrant.Condition{ns, qdrant.NewIsEmpty(qdrantNamespaceField)}}
	if f := a.filterWithLegacy(path); !proto.Equal(f, want) {
		t.Fatalf("filterWithLegacy = %v, want %v", f, want)
	}
}

// qdrantTestClient connects to a local Qdrant (gRPC port from
// GREPAI_QDRANT_TEST_GRPC_PORT, 6334 by default) or skips the test.
func qdrantTestClient(t *testing.T) (*qdrant.Client, int) {
	t.Helper()
	port := 6334
	if v := os.Getenv("GREPAI_QDRANT_TEST_GRPC_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		port = p
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		t.Skipf("local Qdrant unavailable: %v", err)
	}
	_ = conn.Close()
	client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, port
}

// TestQdrantSharedCollectionIsolation reproduces #247 on a real Qdrant: two
// projects indexing the same explicit collection must not overwrite or delete
// each other's points, and an index written before namespaces existed must be
// reused and replaced rather than left behind.
func TestQdrantSharedCollectionIsolation(t *testing.T) {
	client, port := qdrantTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	collection := fmt.Sprintf("test_grepai_247_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = client.DeleteCollection(cleanupCtx, collection)
	})
	openStore := func(namespace string, legacyReadFallback bool) *QdrantStore {
		s, err := NewQdrantStoreWithNamespace(ctx, "localhost", port, false, collection, "", 3, namespace, legacyReadFallback)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	open := func(namespace string) *QdrantStore { return openStore(namespace, false) }
	save := func(s *QdrantStore, path, content string, vec ...float32) {
		t.Helper()
		if err := s.DeleteByFile(ctx, path); err != nil {
			t.Fatal(err)
		}
		chunk := Chunk{ID: path + "_0", FilePath: path, Content: content, Hash: content, ContentHash: "ch-" + content, FileHash: "fh-" + content, Vector: vec, UpdatedAt: time.Now()}
		if err := s.SaveChunks(ctx, []Chunk{chunk}); err != nil {
			t.Fatal(err)
		}
	}
	docs := func(s *QdrantStore) []string {
		t.Helper()
		paths, err := s.ListDocuments(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(paths)
		return paths
	}
	contents := func(s *QdrantStore, path string) []string {
		t.Helper()
		chunks, err := s.GetChunksForFile(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range chunks {
			out = append(out, c.Content)
		}
		return out
	}

	// An earlier release indexed project A into the explicit collection.
	legacy := open("")
	save(legacy, "README.md", "victim", 1, 0, 0)
	save(legacy, "victim_only.go", "secret", 0, 1, 0)
	save(legacy, "deleted.go", "gone", 0, 0, 1)

	a := open("/work/a")
	if got := docs(a); len(got) != 0 {
		t.Fatalf("legacy points are visible to the namespaced store: %v", got)
	}
	// Right after upgrading, a search-only store still reads the old index.
	reader := openStore("/work/a", true)
	if results, err := reader.Search(ctx, []float32{0, 1, 0}, 1, SearchOptions{}); err != nil || len(results) != 1 || results[0].Chunk.Content != "secret" {
		t.Fatalf("fallback search = %+v, %v; want the legacy victim_only.go chunk", results, err)
	}
	if stats, err := reader.GetStats(ctx); err != nil || stats.TotalFiles != 3 || stats.TotalChunks != 3 {
		t.Fatalf("fallback stats = %+v, %v", stats, err)
	}
	// Re-indexing A reuses the stored vector, then replaces the legacy point.
	vec, found, err := a.LookupByContentHash(ctx, "ch-victim", "vic")
	if err != nil || !found || len(vec) != 3 || vec[0] != 1 {
		t.Fatalf("legacy vector not reused: vec=%v found=%v err=%v", vec, found, err)
	}
	// After an embedding model change, legacy vectors must not be reused.
	rebuilt := open("/work/a")
	rebuilt.DisableLegacyVectorReuse()
	if _, found, err := rebuilt.LookupByContentHash(ctx, "ch-victim", "vic"); found || err != nil {
		t.Fatalf("legacy vector reused after a model change: found=%v err=%v", found, err)
	}
	save(a, "README.md", "victim", vec...)
	save(a, "victim_only.go", "secret", 0, 1, 0)
	// Once the project has points of its own, the fallback is off.
	reader = openStore("/work/a", true)
	if stats, err := reader.GetStats(ctx); err != nil || stats.TotalFiles != 2 {
		t.Fatalf("stats after migration = %+v, %v; want only the namespaced files", stats, err)
	}

	// Project B indexes the same collection with its own README.md.
	b := open("/work/b")
	if _, found, _ := b.LookupByContentHash(ctx, "ch-victim", "vic"); found {
		t.Fatal("project B reused project A's namespaced vector")
	}
	save(b, "README.md", "attacker", 1, 1, 0)
	if err := b.DeleteByFile(ctx, "victim_only.go"); err != nil { // B never had it
		t.Fatal(err)
	}

	if got := contents(a, "README.md"); len(got) != 1 || got[0] != "victim" {
		t.Fatalf("project A README.md = %v, want [victim]", got)
	}
	if got := contents(b, "README.md"); len(got) != 1 || got[0] != "attacker" {
		t.Fatalf("project B README.md = %v, want [attacker]", got)
	}
	if got := docs(a); fmt.Sprint(got) != "[README.md victim_only.go]" {
		t.Fatalf("project A documents = %v", got)
	}
	if got := docs(b); fmt.Sprint(got) != "[README.md]" {
		t.Fatalf("project B documents = %v", got)
	}
	results, err := b.Search(ctx, []float32{1, 0, 0}, 10, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.Content != "attacker" {
		t.Fatalf("project B search returned %+v", results)
	}
	stats, err := a.GetStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalChunks != 2 || stats.TotalFiles != 2 {
		t.Fatalf("project A stats = %+v", stats)
	}
	// Legacy vectors serve the indexer's initial scan; once it disables the
	// reuse after that scan, leftovers (possibly from another model) are not
	// picked up any more.
	afterScan := open("/work/a")
	if _, found, err := afterScan.LookupByContentHash(ctx, "ch-gone", "go"); !found || err != nil {
		t.Fatalf("initial scan did not reuse the legacy vector: found=%v err=%v", found, err)
	}
	afterScan.DisableLegacyVectorReuse()
	if _, found, err := afterScan.LookupByContentHash(ctx, "ch-gone", "go"); found || err != nil {
		t.Fatalf("legacy vector reused after the initial scan: found=%v err=%v", found, err)
	}
	// Only the legacy point of the file deleted since remains.
	remaining, err := client.Count(ctx, &qdrant.CountPoints{CollectionName: collection, Exact: qdrant.PtrOf(true),
		Filter: &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewIsEmpty(qdrantNamespaceField)}}})
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("%d legacy points left, want 1 (deleted.go)", remaining)
	}
}
