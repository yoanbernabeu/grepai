package store

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePointsServer struct {
	qdrant.UnimplementedPointsServer
	mu       sync.Mutex
	requests []*qdrant.ScrollPoints
	scroll   func(context.Context, *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error)
}

func (s *fakePointsServer) Scroll(ctx context.Context, req *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	return s.scroll(ctx, req)
}

func newQdrantListDocumentsTestStore(t *testing.T, server *fakePointsServer) *QdrantStore {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	qdrant.RegisterPointsServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	port := listener.Addr().(*net.TCPAddr).Port
	client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: port, PoolSize: 1, SkipCompatibilityCheck: true})
	if err != nil {
		grpcServer.Stop()
		_ = listener.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		grpcServer.Stop()
		_ = listener.Close()
	})
	return &QdrantStore{client: client, collectionName: "tenant-project"}
}

func qdrantListPoint(t *testing.T, id uint64, path string) *qdrant.RetrievedPoint {
	t.Helper()
	return &qdrant.RetrievedPoint{Id: qdrant.NewIDNum(id), Payload: map[string]*qdrant.Value{"file_path": mustCreateValue(t, path)}}
}

func TestQdrantListDocumentsPaginatesAndDeduplicates(t *testing.T) {
	pages := map[uint64]*qdrant.ScrollResponse{
		0: {Result: []*qdrant.RetrievedPoint{qdrantListPoint(t, 1, "first.go"), qdrantListPoint(t, 2, "duplicate.go")}, NextPageOffset: qdrant.NewIDNum(2)},
		2: {Result: []*qdrant.RetrievedPoint{qdrantListPoint(t, 3, "second.go"), qdrantListPoint(t, 4, "duplicate.go")}, NextPageOffset: qdrant.NewIDNum(4)},
		4: {Result: []*qdrant.RetrievedPoint{qdrantListPoint(t, 5, "third.go")}},
	}
	server := &fakePointsServer{}
	server.scroll = func(_ context.Context, req *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
		return pages[req.GetOffset().GetNum()], nil
	}
	store := newQdrantListDocumentsTestStore(t, server)
	paths, err := store.ListDocuments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	want := []string{"duplicate.go", "first.go", "second.go", "third.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(server.requests))
	}
}

func TestQdrantListDocumentsPageErrorReturnsNoPartialInventory(t *testing.T) {
	server := &fakePointsServer{}
	server.scroll = func(_ context.Context, req *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
		if req.Offset == nil {
			return &qdrant.ScrollResponse{Result: []*qdrant.RetrievedPoint{qdrantListPoint(t, 1, "partial.go")}, NextPageOffset: qdrant.NewIDNum(1)}, nil
		}
		return nil, status.Error(codes.Internal, "later page failed")
	}
	store := newQdrantListDocumentsTestStore(t, server)
	paths, err := store.ListDocuments(context.Background())
	if err == nil || status.Code(errors.Unwrap(err)) != codes.Internal {
		t.Fatalf("error = %v, want wrapped Internal", err)
	}
	if paths != nil {
		t.Fatalf("paths = %v, want nil", paths)
	}
}

func TestQdrantListDocumentsHonorsCanceledContext(t *testing.T) {
	server := &fakePointsServer{scroll: func(ctx context.Context, _ *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	store := newQdrantListDocumentsTestStore(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	paths, err := store.ListDocuments(ctx)
	if !errors.Is(err, context.Canceled) || paths != nil {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
}
