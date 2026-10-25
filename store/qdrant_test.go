package store

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestQdrantFileHashPayload(t *testing.T) {
	s := &QdrantStore{}
	for _, hash := range []string{"", strings.Repeat("a", 64)} {
		chunk := Chunk{FilePath: "file.go", FileHash: hash, Hash: "chunk-hash", ContentHash: "content-hash"}
		payload, err := s.buildChunkPayload(chunk)
		if err != nil {
			t.Fatal(err)
		}
		value, present := payload["file_hash"]
		if present != (hash != "") || value.GetStringValue() != hash {
			t.Fatalf("file_hash = %v, present=%v, want %q", value, present, hash)
		}
		parsed := s.parseChunkPayload(payload)
		if parsed.FileHash != hash || parsed.Hash != chunk.Hash || parsed.ContentHash != chunk.ContentHash {
			t.Fatalf("hashes did not round-trip: %+v", parsed)
		}
	}
}

type fileHashPointsServer struct {
	qdrant.UnimplementedPointsServer
	scroll func(*qdrant.ScrollPoints) (*qdrant.ScrollResponse, error)
}

func (s *fileHashPointsServer) Scroll(_ context.Context, request *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
	return s.scroll(request)
}

func TestQdrantGetDocumentFileHash(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hashes   []string
		wantHash string
		fail     bool
	}{
		{"missing file", nil, "", false},
		{"legacy points", []string{"", ""}, "", false},
		{"file hash", []string{"file-hash", "file-hash"}, "file-hash", false},
		{"hash on later point", []string{"", "file-hash"}, "file-hash", false},
		{"scroll error", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			qdrant.RegisterPointsServer(server, &fileHashPointsServer{scroll: func(request *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
				if request.GetCollectionName() != "test_grepai_filehash" || request.GetLimit() != 1000 ||
					!proto.Equal(request.GetFilter(), &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewMatch("file_path", "file.go")}}) ||
					!proto.Equal(request.GetWithPayload(), qdrant.NewWithPayloadInclude("file_path", "file_hash")) {
					t.Errorf("unexpected scroll request: %v", request)
				}
				if tc.fail {
					return nil, status.Error(codes.Unavailable, "test scroll failure")
				}
				points := make([]*qdrant.RetrievedPoint, len(tc.hashes))
				for i, hash := range tc.hashes {
					payload := map[string]*qdrant.Value{"hash": mustCreateValue(t, "not-a-file-hash")}
					if hash != "" {
						payload["file_hash"] = mustCreateValue(t, hash)
					}
					points[i] = &qdrant.RetrievedPoint{Id: qdrant.NewIDNum(uint64(i + 1)), Payload: payload}
				}
				return &qdrant.ScrollResponse{Result: points}, nil
			}})
			t.Cleanup(server.Stop)
			go func() {
				if err := server.Serve(listener); err != nil {
					t.Errorf("serve: %v", err)
				}
			}()
			client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, SkipCompatibilityCheck: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Error(err)
				}
			})
			s := &QdrantStore{client: client, collectionName: "test_grepai_filehash"}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			doc, err := s.GetDocument(ctx, "file.go")
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "test scroll failure") || doc != nil {
					t.Fatalf("doc=%v err=%v", doc, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.hashes) == 0 {
				if doc != nil {
					t.Fatalf("missing file returned %+v", doc)
				}
				return
			}
			wantIDs := make([]string, len(tc.hashes))
			for i := range wantIDs {
				wantIDs[i] = qdrant.NewIDNum(uint64(i + 1)).String()
			}
			if doc == nil || doc.Path != "file.go" || doc.Hash != tc.wantHash || !reflect.DeepEqual(doc.ChunkIDs, wantIDs) {
				t.Fatalf("unexpected document: %+v", doc)
			}
		})
	}
}

func TestQdrantFileHashIntegration(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6333", time.Second)
	if err != nil {
		t.Skipf("local Qdrant unavailable: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: 6334})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	const collection = "test_grepai_filehash"
	exists, err := client.CollectionExists(ctx, collection)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("test collection already exists; refusing to change an unowned collection")
	}
	if err := client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collection,
		VectorsConfig:  qdrant.NewVectorsConfig(&qdrant.VectorParams{Size: 3, Distance: qdrant.Distance_Cosine}),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := client.DeleteCollection(cleanupCtx, collection); err != nil {
			t.Error(err)
		}
	})
	s := &QdrantStore{client: client, collectionName: collection}
	var points []*qdrant.PointStruct
	for _, fixture := range []struct {
		path, hash string
		count      int
	}{
		{"current.go", "whole-file-hash", 2},
		{"legacy.go", "", 1},
		{"large.go", "large-file-hash", 1001},
	} {
		for i := 0; i < fixture.count; i++ {
			chunk := Chunk{ID: fmt.Sprintf("%s_%d", fixture.path, i), FilePath: fixture.path, FileHash: fixture.hash, Hash: "chunk-hash"}
			payload, err := s.buildChunkPayload(chunk)
			if err != nil {
				t.Fatal(err)
			}
			points = append(points, &qdrant.PointStruct{Id: qdrant.NewID(s.getUUIDForChunk(chunk.ID).String()), Payload: payload, Vectors: qdrant.NewVectors(1, 0, 0)})
		}
	}
	// Wait for application, not just acknowledgement, before reading the fixtures.
	if _, err := client.Upsert(ctx, &qdrant.UpsertPoints{CollectionName: collection, Wait: qdrant.PtrOf(true), Points: points}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, hash string
		count      int
	}{
		{"missing.go", "", 0}, {"current.go", "whole-file-hash", 2},
		{"legacy.go", "", 1}, {"large.go", "large-file-hash", 1000},
	} {
		doc, err := s.GetDocument(ctx, tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if tc.count == 0 {
			if doc != nil {
				t.Fatalf("missing file: %+v", doc)
			}
		} else if doc == nil || doc.Path != tc.path || doc.Hash != tc.hash || len(doc.ChunkIDs) != tc.count {
			t.Fatalf("%s: unexpected document %+v", tc.path, doc)
		}
	}
}

func mustCreateValue(t *testing.T, value interface{}) *qdrant.Value {
	t.Helper()
	val, err := qdrant.NewValue(value)
	if err != nil {
		t.Fatalf("failed to create qdrant value: %v", err)
	}
	return val
}

// TestSanitizeCollectionName tests the exported SanitizeCollectionName function
func TestSanitizeCollectionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "/Users/test/project", "_Users_test_project"},
		{"nested path", "/home/user/src/github.com/repo", "_home_user_src_github.com_repo"},
		{"root path", "/", "_"},
		{"multiple slashes", "///", "___"},
		{"no slashes", "myproject", "myproject"},
		{"windows drive path", "C:\\Users\\test\\project", "C__Users_test_project"},
		{"windows drive colon", "C:", "C_"},
		{"windows nested path", "C:\\Users\\test\\dev\\grepai", "C__Users_test_dev_grepai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCollectionName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestParseHost tests the parseHost function with various endpoint formats
func TestParseHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"http scheme", "http://localhost", "localhost"},
		{"https scheme", "https://qdrant.io", "qdrant.io"},
		{"with port", "localhost:6334", "localhost"},
		{"http with port", "http://localhost:6334", "localhost"},
		{"https with port", "https://qdrant.io:443", "qdrant.io"},
		{"with path", "http://localhost/v1", "localhost"},
		{"with port and path", "http://localhost:6334/v1", "localhost"},
		{"IP address", "192.168.1.1", "192.168.1.1"},
		{"complex URL", "https://qdrant-cluster.qdrant.io:6334/v1/collections", "qdrant-cluster.qdrant.io"},
		{"just hostname", "localhost", "localhost"},
		{"just IP", "127.0.0.1", "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHost(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestGetUUIDForChunk tests that UUID generation is deterministic for the same chunk ID
func TestGetUUIDForChunk(t *testing.T) {
	store := &QdrantStore{}

	chunkID := "test-file.go:10-20"
	uuid1 := store.getUUIDForChunk(chunkID)
	uuid2 := store.getUUIDForChunk(chunkID)

	if uuid1 != uuid2 {
		t.Errorf("expected same UUID for same chunk ID, got %v and %v", uuid1, uuid2)
	}

	// Different chunk IDs should produce different UUIDs
	uuid3 := store.getUUIDForChunk("other-file.go:5-15")
	if uuid1 == uuid3 {
		t.Errorf("expected different UUIDs for different chunk IDs")
	}

	// Verify it's a valid UUID
	if _, err := uuid.Parse(uuid1.String()); err != nil {
		t.Errorf("generated invalid UUID: %v", err)
	}
}

// TestParseChunkPayload tests parsing of Qdrant point payloads
func TestParseChunkPayload(t *testing.T) {
	store := &QdrantStore{}

	now := time.Now().UTC()

	tests := []struct {
		name     string
		payload  map[string]*qdrant.Value
		expected *Chunk
	}{
		{
			name: "complete payload",
			payload: map[string]*qdrant.Value{
				"file_path":  mustCreateValue(t, "test.go"),
				"start_line": mustCreateValue(t, int64(10)),
				"end_line":   mustCreateValue(t, int64(20)),
				"content":    mustCreateValue(t, "test content"),
				"hash":       mustCreateValue(t, "abc123"),
				"updated_at": mustCreateValue(t, now.Format(time.RFC3339)),
			},
			expected: &Chunk{
				FilePath:  "test.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "test content",
				Hash:      "abc123",
				UpdatedAt: now,
			},
		},
		{
			name: "minimal payload",
			payload: map[string]*qdrant.Value{
				"file_path":  mustCreateValue(t, "test.go"),
				"start_line": mustCreateValue(t, int64(1)),
				"end_line":   mustCreateValue(t, int64(10)),
			},
			expected: &Chunk{
				FilePath:  "test.go",
				StartLine: 1,
				EndLine:   10,
				Content:   "",
				Hash:      "",
				UpdatedAt: time.Time{},
			},
		},
		{
			name:    "empty payload",
			payload: map[string]*qdrant.Value{},
			expected: &Chunk{
				FilePath:  "",
				StartLine: 0,
				EndLine:   0,
				Content:   "",
				Hash:      "",
				UpdatedAt: time.Time{},
			},
		},
		{
			name: "partial payload with missing fields",
			payload: map[string]*qdrant.Value{
				"file_path": mustCreateValue(t, "test.go"),
				"content":   mustCreateValue(t, "some content"),
			},
			expected: &Chunk{
				FilePath:  "test.go",
				StartLine: 0,
				EndLine:   0,
				Content:   "some content",
				Hash:      "",
				UpdatedAt: time.Time{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.parseChunkPayload(tt.payload)

			if result.FilePath != tt.expected.FilePath {
				t.Errorf("expected FilePath %s, got %s", tt.expected.FilePath, result.FilePath)
			}
			if result.StartLine != tt.expected.StartLine {
				t.Errorf("expected StartLine %d, got %d", tt.expected.StartLine, result.StartLine)
			}
			if result.EndLine != tt.expected.EndLine {
				t.Errorf("expected EndLine %d, got %d", tt.expected.EndLine, result.EndLine)
			}
			if result.Content != tt.expected.Content {
				t.Errorf("expected Content %s, got %s", tt.expected.Content, result.Content)
			}
			if result.Hash != tt.expected.Hash {
				t.Errorf("expected Hash %s, got %s", tt.expected.Hash, result.Hash)
			}
			// Allow small time difference due to parsing
			if !result.UpdatedAt.IsZero() && !tt.expected.UpdatedAt.IsZero() {
				if result.UpdatedAt.Sub(tt.expected.UpdatedAt).Abs() > time.Second {
					t.Errorf("expected UpdatedAt %v, got %v", tt.expected.UpdatedAt, result.UpdatedAt)
				}
			}
		})
	}
}

// TestBuildChunkPayload tests building of Qdrant payloads from chunks
func TestBuildChunkPayload(t *testing.T) {
	store := &QdrantStore{}

	now := time.Now().UTC()

	tests := []struct {
		name    string
		chunk   Chunk
		wantErr bool
	}{
		{
			name: "valid chunk",
			chunk: Chunk{
				ID:        "test-id",
				FilePath:  "test.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "test content",
				Hash:      "abc123",
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "minimal chunk",
			chunk: Chunk{
				ID:        "test-id",
				FilePath:  "test.go",
				StartLine: 0,
				EndLine:   0,
				Content:   "",
				Hash:      "",
				UpdatedAt: time.Time{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := store.buildChunkPayload(tt.chunk)

			if (err != nil) != tt.wantErr {
				t.Errorf("buildChunkPayload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if val, ok := payload["file_path"]; !ok {
					t.Error("expected file_path in payload")
				} else if val.GetStringValue() != tt.chunk.FilePath {
					t.Errorf("expected file_path %s, got %s", tt.chunk.FilePath, val.GetStringValue())
				}

				if val, ok := payload["start_line"]; !ok {
					t.Error("expected start_line in payload")
				} else if val.GetIntegerValue() != int64(tt.chunk.StartLine) {
					t.Errorf("expected start_line %d, got %d", tt.chunk.StartLine, val.GetIntegerValue())
				}

				if val, ok := payload["end_line"]; !ok {
					t.Error("expected end_line in payload")
				} else if val.GetIntegerValue() != int64(tt.chunk.EndLine) {
					t.Errorf("expected end_line %d, got %d", tt.chunk.EndLine, val.GetIntegerValue())
				}

				if val, ok := payload["content"]; !ok {
					t.Error("expected content in payload")
				} else if val.GetStringValue() != tt.chunk.Content {
					t.Errorf("expected content %s, got %s", tt.chunk.Content, val.GetStringValue())
				}

				if val, ok := payload["hash"]; !ok {
					t.Error("expected hash in payload")
				} else if val.GetStringValue() != tt.chunk.Hash {
					t.Errorf("expected hash %s, got %s", tt.chunk.Hash, val.GetStringValue())
				}
			}
		})
	}
}

// TestSearch_InvalidLimit tests that Search returns error for invalid limits
func TestSearch_InvalidLimit(t *testing.T) {
	store := &QdrantStore{
		client:         nil, // Not used for validation
		collectionName: "test",
		dimensions:     768,
	}

	tests := []struct {
		name  string
		limit int
		want  string
	}{
		{"zero limit", 0, "limit must be positive"},
		{"negative limit", -1, "limit must be positive"},
		{"negative large limit", -100, "limit must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Search(context.Background(), []float32{0.1, 0.2}, tt.limit, SearchOptions{})
			if err == nil {
				t.Error("expected error, got nil")
			} else if err.Error()[:22] != tt.want {
				t.Errorf("expected error message to start with %s, got %s", tt.want, err.Error())
			}
		})
	}
}

// TestSaveChunks_EmptySlice tests that SaveChunks handles empty chunks slice
func TestSaveChunks_EmptySlice(t *testing.T) {
	store := &QdrantStore{
		client:         nil, // Not used for empty slice
		collectionName: "test",
		dimensions:     768,
	}

	err := store.SaveChunks(context.Background(), []Chunk{})
	if err != nil {
		t.Errorf("expected no error for empty chunks, got %v", err)
	}
}

// TestQdrantStore_StructFields verifies struct has all expected fields
func TestQdrantStore_StructFields(t *testing.T) {
	store := &QdrantStore{
		collectionName: "test-collection",
		dimensions:     768,
		apiKey:         "test-key",
	}

	// Use all fields to avoid unused variable warnings
	if store.collectionName != "test-collection" || store.dimensions != 768 || store.apiKey != "test-key" {
		t.Errorf("expected collectionName 'test-collection', dimensions 768, and apiKey 'test-key'")
	}
}

// TestQdrantStore_CollectionNameSanitization verifies / \ : replacement with _
func TestQdrantStore_CollectionNameSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "/Users/test/project", "_Users_test_project"},
		{"nested path", "/home/user/src/github.com/repo", "_home_user_src_github.com_repo"},
		{"root path", "/", "_"},
		{"multiple slashes", "///", "___"},
		{"no slashes", "myproject", "myproject"},
		{"windows drive path", "C:\\Users\\test\\project", "C__Users_test_project"},
		{"windows drive colon", "C:", "C_"},
		{"windows nested path", "C:\\Users\\test\\dev\\grepai", "C__Users_test_dev_grepai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeCollectionName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestQdrantStore_DimensionsField verifies dimension field values
func TestQdrantStore_DimensionsField(t *testing.T) {
	tests := []struct {
		name       string
		dimensions int
	}{
		{"nomic-embed-text", 768},
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &QdrantStore{
				collectionName: "test",
				dimensions:     tt.dimensions,
			}

			// Verify dimensions is accessible and correct
			if store.dimensions != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, store.dimensions)
			}
			if store.collectionName != "test" {
				t.Errorf("expected collectionName 'test', got %s", store.collectionName)
			}
		})
	}
}

// TestQdrantStore_ConfigurationVariants tests different config combinations
func TestQdrantStore_ConfigurationVariants(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		collection string
		apiKey     string
		dimensions int
	}{
		{
			name:       "local qdrant",
			endpoint:   "http://localhost:6333",
			collection: "myproject",
			apiKey:     "",
			dimensions: 768,
		},
		{
			name:       "qdrant cloud",
			endpoint:   "https://cloud.qdrant.io",
			collection: "project",
			apiKey:     "secret-key",
			dimensions: 1536,
		},
		{
			name:       "custom endpoint",
			endpoint:   "http://192.168.1.100:6333",
			collection: "test_repo",
			apiKey:     "",
			dimensions: 768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &QdrantStore{
				collectionName: tt.collection,
				dimensions:     tt.dimensions,
				apiKey:         tt.apiKey,
			}

			// Verify all fields are accessible and correct
			if store.collectionName != tt.collection {
				t.Errorf("expected collectionName %s, got %s", tt.collection, store.collectionName)
			}
			if store.dimensions != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, store.dimensions)
			}
			if store.apiKey != tt.apiKey {
				t.Errorf("expected apiKey %s, got %s", tt.apiKey, store.apiKey)
			}
		})
	}
}
