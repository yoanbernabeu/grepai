package trace

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeBareStore implements only the required SymbolStore interface and counts
// graph enumeration attempts made while detecting unsupported snapshots.
type fakeBareStore struct {
	callEdgesCalls int
}

func (fakeBareStore) SaveFile(context.Context, string, []Symbol, []Reference) error {
	return nil
}
func (fakeBareStore) SaveFileWithContentHash(context.Context, string, string, []Symbol, []Reference) error {
	return nil
}
func (fakeBareStore) SaveFileWithSignature(context.Context, string, string, string, []Symbol, []Reference) error {
	return nil
}
func (fakeBareStore) DeleteFile(context.Context, string) error { return nil }
func (fakeBareStore) IsFileIndexed(string) bool                { return false }
func (fakeBareStore) LookupSymbol(context.Context, string) ([]Symbol, error) {
	return []Symbol{}, nil
}
func (fakeBareStore) LookupSymbolsBatch(context.Context, []string) (map[string][]Symbol, error) {
	return map[string][]Symbol{}, nil
}
func (fakeBareStore) LookupCallers(context.Context, string) ([]Reference, error) {
	return []Reference{}, nil
}
func (fakeBareStore) LookupCallees(context.Context, string, string) ([]Reference, error) {
	return []Reference{}, nil
}
func (fakeBareStore) LookupReaders(context.Context, string) ([]Reference, error) {
	return []Reference{}, nil
}
func (fakeBareStore) LookupWriters(context.Context, string) ([]Reference, error) {
	return []Reference{}, nil
}
func (fakeBareStore) GetCallGraph(context.Context, string, int) (*CallGraph, error) {
	return nil, nil
}
func (fakeBareStore) Load(context.Context) error    { return nil }
func (fakeBareStore) Persist(context.Context) error { return nil }
func (fakeBareStore) GetSymbolsForFile(context.Context, string) ([]Symbol, error) {
	return []Symbol{}, nil
}

func (f *fakeBareStore) GetCallEdges(context.Context) ([]CallEdge, error) {
	f.callEdgesCalls++
	return []CallEdge{}, nil
}
func (fakeBareStore) Close() error { return nil }
func (fakeBareStore) GetStats(context.Context) (*SymbolStats, error) {
	return &SymbolStats{}, nil
}

var _ SymbolStore = (*fakeBareStore)(nil)

// fakeEdgeStore implements SymbolStore by embedding the required interface and
// overriding GetCallEdges to control fallback enumeration.
type fakeEdgeStore struct {
	SymbolStore
	edges []CallEdge
}

func (f *fakeEdgeStore) GetCallEdges(context.Context) ([]CallEdge, error) {
	return append([]CallEdge(nil), f.edges...), nil
}

// fakeSourceStore implements the optional FileFingerprintSource directly,
// on top of a call-edge enumerated store.
type fakeSourceStore struct {
	fakeEdgeStore
	mu        sync.Mutex
	snapshot  map[string]FileFingerprint
	calls     int
	ctxPassed context.Context
	err       error
}

func (f *fakeSourceStore) ListFileFingerprints(ctx context.Context) (map[string]FileFingerprint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.ctxPassed = ctx
	if f.err != nil {
		return nil, f.err
	}
	snapshot := make(map[string]FileFingerprint, len(f.snapshot))
	for path, fingerprint := range f.snapshot {
		snapshot[path] = fingerprint
	}
	return snapshot, nil
}

func TestLoadFileFingerprintsUsesOptionalSourceWithoutFallback(t *testing.T) {
	// Given a store whose FileFingerprintSource succeeds while its call
	// edges describe a different, unrelated file.
	bare := &fakeBareStore{}
	source := &fakeSourceStore{
		fakeEdgeStore: fakeEdgeStore{SymbolStore: bare, edges: []CallEdge{{Caller: "A", Callee: "B", File: "unrelated.go"}}},
		snapshot: map[string]FileFingerprint{
			"orphan.go": {ContentHash: "hash-orphan", ExtractorVersion: "v2", HasContentHash: true, HasExtractorVersion: true},
		},
	}

	// When fingerprints load, the optional source wins and fallback
	// enumeration never runs.
	got, err := LoadFileFingerprints(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1", source.calls)
	}
	if len(got) != 1 || got["orphan.go"].ContentHash != "hash-orphan" || got["orphan.go"].ExtractorVersion != "v2" {
		t.Fatalf("snapshot = %#v", got)
	}
	if _, ok := got["unrelated.go"]; ok {
		t.Fatal("fallback enumeration ran even though the optional source succeeded")
	}
	if bare.callEdgesCalls != 0 {
		t.Fatalf("GetCallEdges calls = %d, want 0", bare.callEdgesCalls)
	}
}

func TestLoadFileFingerprintsPreservesSourceErrors(t *testing.T) {
	// Given a source that fails.
	wantErr := errors.New("source failed")
	source := &fakeSourceStore{err: wantErr}

	// When fingerprints load, the error passes through without silent
	// fallback to enumeration.
	_, err := LoadFileFingerprints(context.Background(), source)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1", source.calls)
	}
}

func TestLoadFileFingerprintsPassesContextToSource(t *testing.T) {
	// Given a canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := &fakeSourceStore{snapshot: map[string]FileFingerprint{}}

	// When fingerprints load, the caller context reaches the source.
	_, _ = LoadFileFingerprints(ctx, source)
	if source.ctxPassed != ctx {
		t.Fatal("caller context was not passed to ListFileFingerprints")
	}
}

func TestLoadFileFingerprintsUnsupportedDoesNotInspectCallGraph(t *testing.T) {
	store := &fakeBareStore{}

	got, err := LoadFileFingerprints(context.Background(), store)
	if got != nil || !errors.Is(err, ErrFileFingerprintsUnsupported) {
		t.Fatalf("snapshot = %#v, error = %v", got, err)
	}
	if store.callEdgesCalls != 0 {
		t.Fatalf("GetCallEdges calls = %d, want 0", store.callEdgesCalls)
	}
}
