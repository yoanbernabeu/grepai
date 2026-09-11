package trace

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// drainMigrationIterator consumes every batch and returns rows keyed by file.
func drainMigrationIterator(t *testing.T, it *migrationBatchIterator) map[string]migrationFileRows {
	t.Helper()
	rows := make(map[string]migrationFileRows)
	for {
		batch, ok := it.nextBatch()
		if !ok {
			return rows
		}
		if len(batch) > migrationBatchSize {
			t.Fatalf("batch exceeded %d files: %d", migrationBatchSize, len(batch))
		}
		for _, file := range batch {
			if _, dup := rows[file.filePath]; dup {
				t.Fatalf("file %q emitted twice", file.filePath)
			}
			rows[file.filePath] = file
		}
	}
}

func saveMigrationFixture(t *testing.T, store *GOBSymbolStore, file string, symbols []Symbol, refs []Reference) {
	t.Helper()
	if err := store.SaveFile(context.Background(), file, symbols, refs); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationBatchIteratorPreservesReferenceOrder(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	refs := []Reference{
		{SymbolName: "readFirst", Kind: RefKindRead, File: "main.go", Line: 10, CallerName: "Main"},
		{SymbolName: "callSecond", Kind: RefKindCall, File: "main.go", Line: 10, CallerName: "Main"},
		{SymbolName: "writeThird", Kind: RefKindWrite, File: "main.go", Line: 11, CallerName: "Main"},
	}
	saveMigrationFixture(t, store, "main.go", nil, refs)
	got := drainMigrationIterator(t, newMigrationBatchIterator(store))["main.go"].refs
	if !reflect.DeepEqual(got, refs) {
		t.Fatalf("migration ref order = %#v, want %#v", got, refs)
	}
}

func TestMigrationBatchIteratorRestoresDuplicateKeyOrder(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	refs := []Reference{
		{SymbolName: "Target", Kind: RefKindCall, File: "dup.go", Line: 5, Context: "first", CallerName: "Main"},
		{SymbolName: "Other", Kind: RefKindCall, File: "dup.go", Line: 6, CallerName: "Main"},
		{SymbolName: "Target", Kind: RefKindCall, File: "dup.go", Line: 5, Context: "second", CallerName: "Main"},
	}
	saveMigrationFixture(t, store, "dup.go", nil, refs)
	got := drainMigrationIterator(t, newMigrationBatchIterator(store))["dup.go"].refs
	if !reflect.DeepEqual(got, refs) {
		t.Fatalf("duplicate-key ref order = %#v, want %#v", got, refs)
	}
}

func TestMigrationBatchIteratorIncludesTopLevelRefsAndSymbols(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	refs := []Reference{{SymbolName: "Top", File: "a.go", CallerName: "<top-level>"}}
	saveMigrationFixture(t, store, "a.go", []Symbol{{Name: "A", File: "a.go"}}, refs)
	saveMigrationFixture(t, store, "b.go", []Symbol{{Name: "B", File: "b.go"}}, nil)
	rows := drainMigrationIterator(t, newMigrationBatchIterator(store))
	if got := rows["a.go"].refs; !reflect.DeepEqual(got, refs) {
		t.Fatalf("top-level refs = %#v", got)
	}
	if len(rows["a.go"].symbols) != 1 || len(rows["b.go"].symbols) != 1 {
		t.Fatalf("symbols by file = %#v", rows)
	}
}

func TestMigrationBatchIteratorLeftoverOrderIsDeterministic(t *testing.T) {
	// Top-level refs never reach the call graph, so they are bucket leftovers;
	// the iterator must emit them in a stable order (sorted by callee name),
	// not in map-iteration order. Each run uses a fresh store because the
	// iterator consumes the source it owns.
	refs := []Reference{
		{SymbolName: "Zeta", File: "a.go", Line: 1, CallerName: "<top-level>"},
		{SymbolName: "Alpha", File: "a.go", Line: 2, CallerName: "<top-level>"},
	}
	want := []Reference{refs[1], refs[0]}
	for run := 0; run < 5; run++ {
		store := NewGOBSymbolStore("unused")
		saveMigrationFixture(t, store, "a.go", nil, refs)
		got := drainMigrationIterator(t, newMigrationBatchIterator(store))["a.go"].refs
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: leftover order = %#v, want %#v", run, got, want)
		}
	}
}

func TestMigrationBatchIteratorBoundsBatchesAndCoversSortedFiles(t *testing.T) {
	store := NewGOBSymbolStore("unused")
	total := 2*migrationBatchSize + 7
	wantFiles := make([]string, 0, total)
	for i := 0; i < total; i++ {
		file := fmt.Sprintf("legacy/%04d.go", i)
		wantFiles = append(wantFiles, file)
		name := fmt.Sprintf("Legacy%d", i)
		saveMigrationFixture(t, store, file,
			[]Symbol{{Name: name, File: file, Line: 1}},
			[]Reference{{SymbolName: "Target", Kind: RefKindCall, File: file, Line: 2, CallerName: name, CallerFile: file, CallerLine: 1}})
	}
	sort.Strings(wantFiles)
	it := newMigrationBatchIterator(store)
	var gotFiles []string
	refs, symbols := 0, 0
	batches := 0
	for {
		batch, ok := it.nextBatch()
		if !ok {
			break
		}
		batches++
		if len(batch) == 0 || len(batch) > migrationBatchSize {
			t.Fatalf("batch %d size = %d, want 1..%d", batches, len(batch), migrationBatchSize)
		}
		for _, file := range batch {
			gotFiles = append(gotFiles, file.filePath)
			symbols += len(file.symbols)
			refs += len(file.refs)
			if len(file.symbols) != 1 || len(file.refs) != 1 {
				t.Fatalf("file %q rows = %d symbols, %d refs", file.filePath, len(file.symbols), len(file.refs))
			}
			if file.refs[0].CallerName != file.symbols[0].Name {
				t.Fatalf("file %q ref/symbol mismatch: %#v %#v", file.filePath, file.refs[0], file.symbols[0])
			}
		}
	}
	if batches != 3 {
		t.Fatalf("batches = %d, want 3", batches)
	}
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Fatalf("file emission order mismatch: got %d files, want sorted fixture order", len(gotFiles))
	}
	if symbols != total || refs != total {
		t.Fatalf("migrated rows = %d symbols, %d refs; want %d each", symbols, refs, total)
	}
}
