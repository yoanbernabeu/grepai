package trace

import (
	"context"
	"reflect"
	"testing"
)

type referenceResultFallbackStore struct {
	SymbolStore
	callees, readers, writers []Reference
	symbols                   map[string][]Symbol
	calleeCalls               int
	readerCalls               int
	writerCalls               int
	batchCalls                int
	batchNames                []string
}

func (s *referenceResultFallbackStore) LookupCallees(context.Context, string, string) ([]Reference, error) {
	s.calleeCalls++
	return s.callees, nil
}
func (s *referenceResultFallbackStore) LookupReaders(context.Context, string) ([]Reference, error) {
	s.readerCalls++
	return s.readers, nil
}
func (s *referenceResultFallbackStore) LookupWriters(context.Context, string) ([]Reference, error) {
	s.writerCalls++
	return s.writers, nil
}
func (s *referenceResultFallbackStore) LookupSymbolsBatch(_ context.Context, names []string) (map[string][]Symbol, error) {
	s.batchCalls++
	s.batchNames = append([]string(nil), names...)
	return s.symbols, nil
}

type compoundReferenceResultStore struct {
	*referenceResultFallbackStore
	calleeResult CalleeLookupResult
	refsResult   RefsLookupResult
	calleeRuns   int
	refsRuns     int
}

func (s *compoundReferenceResultStore) LookupCalleeResult(context.Context, string, string) (CalleeLookupResult, error) {
	s.calleeRuns++
	return s.calleeResult, nil
}
func (s *compoundReferenceResultStore) LookupRefsResult(context.Context, string) (RefsLookupResult, error) {
	s.refsRuns++
	return s.refsResult, nil
}

func TestReferenceResultsPreferCompoundCapabilities(t *testing.T) {
	store := &compoundReferenceResultStore{
		referenceResultFallbackStore: &referenceResultFallbackStore{},
		calleeResult:                 CalleeLookupResult{Symbols: map[string][]Symbol{"Callee": {{Name: "Callee"}}}},
		refsResult:                   RefsLookupResult{References: []Reference{{Kind: RefKindRead}}},
	}
	callee, err := LookupCalleeResult(context.Background(), store, "Target", "target.go")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := LookupRefsResult(context.Background(), store, "uid")
	if err != nil {
		t.Fatal(err)
	}
	if store.calleeRuns != 1 || store.refsRuns != 1 || store.calleeCalls != 0 || store.readerCalls != 0 || store.writerCalls != 0 || store.batchCalls != 0 {
		t.Fatalf("compound/fallback calls = %d/%d/%d/%d/%d/%d", store.calleeRuns, store.refsRuns, store.calleeCalls, store.readerCalls, store.writerCalls, store.batchCalls)
	}
	if callee.Symbols["Callee"][0].Name != "Callee" || len(refs.References) != 1 {
		t.Fatalf("results = %#v / %#v", callee, refs)
	}
}

func TestReferenceResultsFallbackBatchDefinitions(t *testing.T) {
	store := &referenceResultFallbackStore{
		callees: []Reference{{SymbolName: "Callee"}, {SymbolName: "Callee"}},
		readers: []Reference{{Kind: RefKindRead, CallerName: "Reader"}},
		writers: []Reference{{Kind: RefKindWrite, CallerName: "Writer"}},
		symbols: map[string][]Symbol{},
	}
	if _, err := LookupCalleeResult(context.Background(), store, "Target", "target.go"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Target", "Callee"}; !reflect.DeepEqual(store.batchNames, want) {
		t.Fatalf("callee batch = %v, want %v", store.batchNames, want)
	}
	if _, err := LookupRefsResult(context.Background(), store, "uid"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Reader", "Writer"}; !reflect.DeepEqual(store.batchNames, want) {
		t.Fatalf("refs batch = %v, want %v", store.batchNames, want)
	}
	if store.readerCalls != 1 || store.writerCalls != 1 || store.batchCalls != 2 {
		t.Fatalf("fallback calls = readers %d writers %d batches %d", store.readerCalls, store.writerCalls, store.batchCalls)
	}
}
