package trace

import (
	"fmt"
	"testing"
)

func liveReferences(refs []Reference) int {
	count := 0
	for _, ref := range refs {
		if ref != (Reference{}) {
			count++
		}
	}
	return count
}

func liveReferencesSymbol(symbols []Symbol) int {
	count := 0
	for _, symbol := range symbols {
		if symbol != (Symbol{}) {
			count++
		}
	}
	return count
}

// TestMigrationBatchIteratorSharedNameBucketStaysBatchScoped is the
// adversarial regression for the large-GOB memory fix: a single symbol-name
// bucket spans more files than one batch. The constructor must retain the
// original decoded backing rows (never copy them), and each batch may copy
// out only the selected files' rows while the remaining source stays
// resident and is released slot by slot.
func TestMigrationBatchIteratorSharedNameBucketStaysBatchScoped(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	total := migrationBatchSize + 7
	for i := 0; i < total; i++ {
		file := fmt.Sprintf("adv/%04d.go", i)
		refs := []Reference{
			{SymbolName: "Target", Kind: RefKindCall, File: file, Line: 2, CallerName: "Main", Context: "first"},
			{SymbolName: "Target", Kind: RefKindCall, File: file, Line: 3, CallerName: "Main", Context: "second"},
			{SymbolName: "Target", File: file, Line: 4, CallerName: "<top-level>"},
		}
		saveMigrationFixture(t, store, file, []Symbol{{Name: "Main", File: file, Line: 1}}, refs)
	}
	origRefs := store.index.References["Target"]
	origSymbols := store.index.Symbols["Main"]
	it := newMigrationBatchIterator(store)
	// The constructor takes ownership of the decoded maps but must not copy
	// any row structs: the backing arrays have to be the original ones.
	if len(it.refsByName["Target"]) != 3*total || &it.refsByName["Target"][0] != &origRefs[0] {
		t.Fatalf("constructor did not retain original reference backing rows")
	}
	if len(it.symbolsByName["Main"]) != total || &it.symbolsByName["Main"][0] != &origSymbols[0] {
		t.Fatalf("constructor did not retain original symbol backing rows")
	}
	if store.index.Symbols != nil || store.index.References != nil || store.index.CallGraph != nil {
		t.Fatal("store must relinquish decoded collections to the iterator")
	}

	batch, ok := it.nextBatch()
	if !ok || len(batch) != migrationBatchSize {
		t.Fatalf("first batch = %d files, ok=%v; want %d", len(batch), ok, migrationBatchSize)
	}
	refsInBatch := 0
	for _, file := range batch {
		if len(file.refs) != 3 || len(file.symbols) != 1 {
			t.Fatalf("file %q rows = %d refs, %d symbols", file.filePath, len(file.refs), len(file.symbols))
		}
		if file.refs[0].Context != "first" || file.refs[1].Context != "second" || file.refs[2].CallerName != "<top-level>" {
			t.Fatalf("file %q ref order = %#v", file.filePath, file.refs)
		}
		refsInBatch += len(file.refs)
	}
	if refsInBatch != 3*migrationBatchSize {
		t.Fatalf("first batch refs = %d, want %d", refsInBatch, 3*migrationBatchSize)
	}
	// Only the selected files' source slots may be consumed; the remaining
	// files' rows stay resident in the shared name bucket.
	if got := liveReferences(it.refsByName["Target"]); got != 3*7 {
		t.Fatalf("live refs after first batch = %d, want %d (remaining 7 files)", got, 3*7)
	}
	if got := liveReferencesSymbol(it.symbolsByName["Main"]); got != 7 {
		t.Fatalf("live symbols after first batch = %d, want 7", got)
	}

	rest := drainMigrationIterator(t, it)
	if len(rest) != 7 {
		t.Fatalf("remaining files = %d, want 7", len(rest))
	}
	for _, file := range rest {
		if len(file.refs) != 3 || file.refs[0].Context != "first" || file.refs[2].CallerName != "<top-level>" {
			t.Fatalf("remaining file %q refs = %#v", file.filePath, file.refs)
		}
	}
	if len(it.refsByName) != 0 || len(it.symbolsByName) != 0 {
		t.Fatalf("iterator retains buckets after exhaustion: %d ref names, %d symbol names", len(it.refsByName), len(it.symbolsByName))
	}
	for i, edge := range it.callGraph {
		if edge != (CallEdge{}) {
			t.Fatalf("call edge %d not released: %#v", i, edge)
		}
	}
}

func TestMigrationBatchIteratorReleasesConsumedSourceEntries(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	total := migrationBatchSize + 3
	for i := 0; i < total; i++ {
		file := fmt.Sprintf("legacy/%04d.go", i)
		name := fmt.Sprintf("Legacy%d", i)
		saveMigrationFixture(t, store, file,
			[]Symbol{{Name: name, File: file, Line: 1}},
			[]Reference{{SymbolName: "Target", Kind: RefKindCall, File: file, Line: 2, CallerName: name}})
	}
	it := newMigrationBatchIterator(store)
	// Ownership moved; rows are retained, not drained, at construction time.
	if store.index.Symbols != nil || store.index.References != nil {
		t.Fatal("store must relinquish decoded collections to the iterator")
	}
	if len(it.symbolsByName) != total || len(it.refsByName["Target"]) != total {
		t.Fatalf("constructor retention = %d symbol names, %d target refs; want %d each", len(it.symbolsByName), len(it.refsByName["Target"]), total)
	}
	batch, ok := it.nextBatch()
	if !ok || len(batch) != migrationBatchSize {
		t.Fatalf("first batch = %d files, ok=%v", len(batch), ok)
	}
	// After one batch, only the remaining files' entries may be retained.
	if len(it.symbolsByName) != 3 {
		t.Fatalf("retained symbol names = %d, want 3", len(it.symbolsByName))
	}
	if got := liveReferences(it.refsByName["Target"]); got != 3 {
		t.Fatalf("live refs after first batch = %d, want 3", got)
	}
	zeroed := 0
	for _, edge := range it.callGraph {
		if edge == (CallEdge{}) {
			zeroed++
		}
	}
	if zeroed != migrationBatchSize {
		t.Fatalf("zeroed call edges = %d, want %d", zeroed, migrationBatchSize)
	}
	rest := drainMigrationIterator(t, it)
	if len(rest) != 3 {
		t.Fatalf("remaining files = %d, want 3", len(rest))
	}
	if len(it.symbolsByName) != 0 || len(it.refsByName) != 0 {
		t.Fatalf("iterator retains data after exhaustion: %d symbol names, %d ref names", len(it.symbolsByName), len(it.refsByName))
	}
	for i, edge := range it.callGraph {
		if edge != (CallEdge{}) {
			t.Fatalf("call edge %d not released: %#v", i, edge)
		}
	}
}
