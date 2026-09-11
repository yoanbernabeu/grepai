package trace

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type callerResultFallbackStore struct {
	SymbolStore
	refs       []Reference
	symbols    map[string][]Symbol
	callerRuns int
	batchRuns  int
	batchNames []string
}

func (s *callerResultFallbackStore) LookupCallers(context.Context, string) ([]Reference, error) {
	s.callerRuns++
	return s.refs, nil
}

func (s *callerResultFallbackStore) LookupSymbolsBatch(_ context.Context, names []string) (map[string][]Symbol, error) {
	s.batchRuns++
	s.batchNames = append([]string(nil), names...)
	return s.symbols, nil
}

type callerResultCapableStore struct {
	*callerResultFallbackStore
	result CallerLookupResult
	err    error
	runs   int
}

func (s *callerResultCapableStore) LookupCallerResult(context.Context, string) (CallerLookupResult, error) {
	s.runs++
	return s.result, s.err
}

func TestLookupCallerResultDoesNotReturnPartialCapabilityDataOnError(t *testing.T) {
	wantErr := errors.New("snapshot failed")
	store := &callerResultCapableStore{
		callerResultFallbackStore: &callerResultFallbackStore{},
		result:                    CallerLookupResult{Symbols: map[string][]Symbol{"Target": {{Name: "stale"}}}},
		err:                       wantErr,
	}

	got, err := LookupCallerResult(context.Background(), store, "Target")
	if !errors.Is(err, wantErr) || got.Symbols != nil || got.References != nil {
		t.Fatalf("partial result = %#v, error = %v", got, err)
	}
	if store.runs != 1 || store.callerRuns != 0 || store.batchRuns != 0 {
		t.Fatalf("compound error fell back: %d/%d/%d", store.runs, store.callerRuns, store.batchRuns)
	}
}

func TestLookupCallerResultPrefersCompoundCapability(t *testing.T) {
	want := CallerLookupResult{Symbols: map[string][]Symbol{"Target": {{Name: "Target"}}}}
	store := &callerResultCapableStore{callerResultFallbackStore: &callerResultFallbackStore{}, result: want}

	got, err := LookupCallerResult(context.Background(), store, "Target")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %#v, %v; want %#v", got, err, want)
	}
	if store.runs != 1 || store.callerRuns != 0 || store.batchRuns != 0 {
		t.Fatalf("compound/fallback calls = %d/%d/%d", store.runs, store.callerRuns, store.batchRuns)
	}
}

func TestLookupCallerResultFallbackBatchesTargetAndUniqueCallers(t *testing.T) {
	refs := []Reference{{CallerName: "Shared"}, {CallerName: "Shared"}, {CallerName: "Other"}}
	store := &callerResultFallbackStore{refs: refs, symbols: map[string][]Symbol{
		"Target": {{Name: "Target", File: "target.go"}},
		"Shared": {{Name: "Shared", File: "shared.go"}},
	}}

	got, err := LookupCallerResult(context.Background(), store, "Target")
	if err != nil {
		t.Fatal(err)
	}
	if store.callerRuns != 1 || store.batchRuns != 1 {
		t.Fatalf("caller/batch calls = %d/%d", store.callerRuns, store.batchRuns)
	}
	if want := []string{"Target", "Shared", "Other"}; !reflect.DeepEqual(store.batchNames, want) {
		t.Fatalf("batch names = %v, want %v", store.batchNames, want)
	}
	if !reflect.DeepEqual(got.References, refs) || got.Symbols["Target"][0].File != "target.go" {
		t.Fatalf("fallback result = %#v", got)
	}
}
