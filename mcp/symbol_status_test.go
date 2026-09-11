package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/yoanbernabeu/grepai/trace"
)

type closeCountingSymbolStore struct {
	trace.SymbolStore
	loadErr  error
	statsErr error
	closes   int
}

func (s *closeCountingSymbolStore) Load(context.Context) error { return s.loadErr }
func (s *closeCountingSymbolStore) GetStats(context.Context) (*trace.SymbolStats, error) {
	return &trace.SymbolStats{TotalSymbols: 3}, s.statsErr
}
func (s *closeCountingSymbolStore) Close() error { s.closes++; return nil }

type metadataSymbolStore struct {
	trace.SymbolStore
	loadErr    error
	count      int
	countErr   error
	countCalls int
	closes     int
}

func (s *metadataSymbolStore) Load(context.Context) error { return s.loadErr }
func (s *metadataSymbolStore) CountSymbols(ctx context.Context) (int, error) {
	s.countCalls++
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.count, s.countErr
}
func (*metadataSymbolStore) GetStats(context.Context) (*trace.SymbolStats, error) {
	panic("GetStats must not be called when CountSymbols is available")
}
func (s *metadataSymbolStore) Close() error { s.closes++; return nil }

func TestReadSymbolStatusClosesStoreOnLoadFailure(t *testing.T) {
	store := &closeCountingSymbolStore{loadErr: errors.New("load failed")}
	ready, total := readAndCloseSymbolStatus(context.Background(), store)
	if ready || total != 0 || store.closes != 1 {
		t.Fatalf("ready=%v total=%d closes=%d", ready, total, store.closes)
	}
}

func TestReadSymbolStatusClosesStoreOnStatsFailure(t *testing.T) {
	store := &closeCountingSymbolStore{statsErr: errors.New("stats failed")}
	ready, total := readAndCloseSymbolStatus(context.Background(), store)
	if ready || total != 0 || store.closes != 1 {
		t.Fatalf("ready=%v total=%d closes=%d", ready, total, store.closes)
	}
}

func TestReadSymbolStatusReturnsReadyAndClosesOnSuccess(t *testing.T) {
	store := &closeCountingSymbolStore{}
	ready, total := readAndCloseSymbolStatus(context.Background(), store)
	if !ready || total != 3 || store.closes != 1 {
		t.Fatalf("ready=%v total=%d closes=%d", ready, total, store.closes)
	}
}

func TestReadSymbolStatusPrefersMetadataCounter(t *testing.T) {
	store := &metadataSymbolStore{count: 7}
	ready, total := readAndCloseSymbolStatus(context.Background(), store)
	if !ready || total != 7 || store.countCalls != 1 || store.closes != 1 {
		t.Fatalf("ready=%v total=%d countCalls=%d closes=%d", ready, total, store.countCalls, store.closes)
	}
}

func TestReadSymbolStatusCounterFailuresAreNotReadyAndClose(t *testing.T) {
	tests := []struct {
		name  string
		ctx   func() context.Context
		store *metadataSymbolStore
	}{
		{name: "zero", ctx: context.Background, store: &metadataSymbolStore{}},
		{name: "error", ctx: context.Background, store: &metadataSymbolStore{countErr: errors.New("count failed")}},
		{name: "canceled", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, store: &metadataSymbolStore{count: 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, total := readAndCloseSymbolStatus(tt.ctx(), tt.store)
			if ready || total != 0 || tt.store.countCalls != 1 || tt.store.closes != 1 {
				t.Fatalf("ready=%v total=%d countCalls=%d closes=%d", ready, total, tt.store.countCalls, tt.store.closes)
			}
		})
	}
}

func TestReadSymbolStatusLoadFailureSkipsMetadataCounter(t *testing.T) {
	store := &metadataSymbolStore{loadErr: errors.New("migration failed"), count: 5}
	ready, total := readAndCloseSymbolStatus(context.Background(), store)
	if ready || total != 0 || store.countCalls != 0 || store.closes != 1 {
		t.Fatalf("ready=%v total=%d countCalls=%d closes=%d", ready, total, store.countCalls, store.closes)
	}
}
