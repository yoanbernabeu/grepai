package trace

import (
	"context"
	"errors"
	"testing"
)

type readinessStubStore struct {
	SymbolStore
	stats      *SymbolStats
	statsErr   error
	statsCalls int
}

func (s *readinessStubStore) GetStats(context.Context) (*SymbolStats, error) {
	s.statsCalls++
	return s.stats, s.statsErr
}

// counterStubStore is a store that additionally implements SymbolCounter so
// its GetStats usage can be observed alongside the counter path.
type counterStubStore struct {
	readinessStubStore
	count      int
	countErr   error
	countCalls int
}

func (s *counterStubStore) CountSymbols(ctx context.Context) (int, error) {
	s.countCalls++
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.count, s.countErr
}

// counterOnlyStore must never fall through to GetStats: full stats aggregate
// refs/edges/files/sizes, which is exactly what readiness callers must avoid.
type counterOnlyStore struct {
	counterStubStore
}

func (*counterOnlyStore) GetStats(context.Context) (*SymbolStats, error) {
	panic("GetStats must not be called when CountSymbols is available")
}

// The Postgres store already satisfies the optional seam (CountSymbols in
// trace/postgres_lookup.go) and the GOB store must keep its stats fallback.
var _ SymbolCounter = (*PostgresSymbolStore)(nil)

func TestCountSymbolsForReadinessUsesCounterWithoutFullStats(t *testing.T) {
	store := &counterOnlyStore{counterStubStore{count: 42}}
	total, err := CountSymbolsForReadiness(context.Background(), store)
	if err != nil || total != 42 {
		t.Fatalf("total=%d err=%v", total, err)
	}
	if store.countCalls != 1 {
		t.Fatalf("countCalls=%d", store.countCalls)
	}
}

func TestCountSymbolsForReadinessCounterErrorIsNotRetriedViaStats(t *testing.T) {
	sentinel := errors.New("count boom")
	store := &counterOnlyStore{counterStubStore{readinessStubStore: readinessStubStore{}, count: 7, countErr: sentinel}}
	_, err := CountSymbolsForReadiness(context.Background(), store)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want wrapped sentinel", err)
	}
	if store.countCalls != 1 || store.statsCalls != 0 {
		t.Fatalf("countCalls=%d statsCalls=%d", store.countCalls, store.statsCalls)
	}
}

func TestCountSymbolsForReadinessCounterHonorsCanceledContext(t *testing.T) {
	store := &counterOnlyStore{counterStubStore{count: 9}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	total, err := CountSymbolsForReadiness(ctx, store)
	if !errors.Is(err, context.Canceled) || total != 0 {
		t.Fatalf("total=%d err=%v", total, err)
	}
}

func TestCountSymbolsForReadinessFallsBackToStatsForNonCounterStores(t *testing.T) {
	store := &readinessStubStore{stats: &SymbolStats{TotalSymbols: 5, TotalReferences: 99}}
	total, err := CountSymbolsForReadiness(context.Background(), store)
	if err != nil || total != 5 {
		t.Fatalf("total=%d err=%v", total, err)
	}
	if store.statsCalls != 1 {
		t.Fatalf("statsCalls=%d", store.statsCalls)
	}
}

func TestCountSymbolsForReadinessFallsBackPropagatesStatsError(t *testing.T) {
	sentinel := errors.New("stats boom")
	store := &readinessStubStore{statsErr: sentinel}
	_, err := CountSymbolsForReadiness(context.Background(), store)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v", err)
	}
}

func TestGOBSymbolStoreKeepsReadinessViaStats(t *testing.T) {
	// GOB stores are not counter-capable and must not become so implicitly:
	// readiness keeps using their in-memory GetStats, so persisted-index
	// semantics are unchanged.
	store := SymbolStore(NewGOBSymbolStore(t.TempDir() + "/symbols.gob"))
	t.Cleanup(func() { _ = store.Close() })
	if _, ok := store.(SymbolCounter); ok {
		t.Fatal("GOBSymbolStore unexpectedly implements SymbolCounter")
	}
	ctx := context.Background()
	if total, err := CountSymbolsForReadiness(ctx, store); err != nil || total != 0 {
		t.Fatalf("empty GOB total=%d err=%v", total, err)
	}
	if err := store.SaveFile(ctx, "a.go", []Symbol{{Name: "A", File: "a.go"}}, []Reference{{SymbolName: "A", File: "a.go"}}); err != nil {
		t.Fatal(err)
	}
	total, err := CountSymbolsForReadiness(ctx, store)
	if err != nil || total != 1 {
		t.Fatalf("GOB total=%d err=%v", total, err)
	}
}
