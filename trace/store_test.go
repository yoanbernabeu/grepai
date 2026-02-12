package trace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGOBSymbolStore_should_load_empty_when_no_file_exists(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load should return nil when file does not exist, got: %v", err)
	}

	symbols, err := store.LookupSymbol(ctx, "anything")
	if err != nil {
		t.Fatalf("LookupSymbol should not error on empty store: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols on empty store, got %d", len(symbols))
	}
}

func TestGOBSymbolStore_should_save_and_lookup_symbols(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "HandleRequest", Kind: KindFunction, File: "server.go", Line: 10, Package: "main", Exported: true, Language: "go"},
		{Name: "parseBody", Kind: KindFunction, File: "server.go", Line: 50, Package: "main", Exported: false, Language: "go"},
	}

	err := store.SaveFile(ctx, "server.go", symbols, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	result, err := store.LookupSymbol(ctx, "HandleRequest")
	if err != nil {
		t.Fatalf("LookupSymbol failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(result))
	}
	if result[0].File != "server.go" {
		t.Errorf("expected file server.go, got %s", result[0].File)
	}
	if result[0].Line != 10 {
		t.Errorf("expected line 10, got %d", result[0].Line)
	}
	if result[0].Kind != KindFunction {
		t.Errorf("expected kind function, got %s", result[0].Kind)
	}

	result2, err := store.LookupSymbol(ctx, "parseBody")
	if err != nil {
		t.Fatalf("LookupSymbol failed: %v", err)
	}
	if len(result2) != 1 {
		t.Fatalf("expected 1 symbol for parseBody, got %d", len(result2))
	}
}

func TestGOBSymbolStore_should_save_and_lookup_callers(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "ProcessOrder", Kind: KindFunction, File: "order.go", Line: 20, Language: "go"},
	}
	refs := []Reference{
		{SymbolName: "ProcessOrder", File: "handler.go", Line: 30, CallerName: "HandleOrder", CallerFile: "handler.go", CallerLine: 25, Context: "ProcessOrder(order)"},
		{SymbolName: "ProcessOrder", File: "worker.go", Line: 45, CallerName: "RunWorker", CallerFile: "worker.go", CallerLine: 40, Context: "ProcessOrder(item)"},
	}

	err := store.SaveFile(ctx, "handler.go", symbols, refs)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	callers, err := store.LookupCallers(ctx, "ProcessOrder")
	if err != nil {
		t.Fatalf("LookupCallers failed: %v", err)
	}
	if len(callers) != 2 {
		t.Fatalf("expected 2 callers, got %d", len(callers))
	}

	callerNames := map[string]bool{}
	for _, ref := range callers {
		callerNames[ref.CallerName] = true
	}
	if !callerNames["HandleOrder"] {
		t.Error("expected HandleOrder in callers")
	}
	if !callerNames["RunWorker"] {
		t.Error("expected RunWorker in callers")
	}
}

func TestGOBSymbolStore_should_save_and_lookup_callees(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "HandleRequest", Kind: KindFunction, File: "server.go", Line: 10, Language: "go"},
		{Name: "ValidateInput", Kind: KindFunction, File: "server.go", Line: 50, Language: "go"},
		{Name: "SaveRecord", Kind: KindFunction, File: "server.go", Line: 80, Language: "go"},
	}
	refs := []Reference{
		{SymbolName: "ValidateInput", File: "server.go", Line: 15, CallerName: "HandleRequest", CallerFile: "server.go", CallerLine: 10, Context: "ValidateInput(req)"},
		{SymbolName: "SaveRecord", File: "server.go", Line: 20, CallerName: "HandleRequest", CallerFile: "server.go", CallerLine: 10, Context: "SaveRecord(data)"},
	}

	err := store.SaveFile(ctx, "server.go", symbols, refs)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	callees, err := store.LookupCallees(ctx, "HandleRequest", "server.go")
	if err != nil {
		t.Fatalf("LookupCallees failed: %v", err)
	}
	if len(callees) < 2 {
		t.Fatalf("expected at least 2 callees, got %d", len(callees))
	}

	calleeNames := map[string]bool{}
	for _, ref := range callees {
		calleeNames[ref.SymbolName] = true
	}
	if !calleeNames["ValidateInput"] {
		t.Error("expected ValidateInput in callees")
	}
	if !calleeNames["SaveRecord"] {
		t.Error("expected SaveRecord in callees")
	}
}

func TestGOBSymbolStore_should_delete_file_symbols(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "Foo", Kind: KindFunction, File: "a.go", Line: 1, Language: "go"},
	}
	refs := []Reference{
		{SymbolName: "Foo", File: "a.go", Line: 5, CallerName: "Bar", CallerFile: "a.go", CallerLine: 10},
	}

	err := store.SaveFile(ctx, "a.go", symbols, refs)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	result, _ := store.LookupSymbol(ctx, "Foo")
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol before delete, got %d", len(result))
	}

	err = store.DeleteFile(ctx, "a.go")
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	result, _ = store.LookupSymbol(ctx, "Foo")
	if len(result) != 0 {
		t.Errorf("expected 0 symbols after delete, got %d", len(result))
	}

	callers, _ := store.LookupCallers(ctx, "Foo")
	if len(callers) != 0 {
		t.Errorf("expected 0 callers after delete, got %d", len(callers))
	}
}

func TestGOBSymbolStore_should_report_file_indexed(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	if store.IsFileIndexed("main.go") {
		t.Error("expected file not indexed before SaveFile")
	}

	symbols := []Symbol{
		{Name: "Main", Kind: KindFunction, File: "main.go", Line: 1, Language: "go"},
	}
	err := store.SaveFile(ctx, "main.go", symbols, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	if !store.IsFileIndexed("main.go") {
		t.Error("expected file indexed after SaveFile")
	}

	err = store.DeleteFile(ctx, "main.go")
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	if store.IsFileIndexed("main.go") {
		t.Error("expected file not indexed after DeleteFile")
	}
}

func TestGOBSymbolStore_should_persist_and_reload(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")
	ctx := context.Background()

	// Create store, save data, persist
	store1 := NewGOBSymbolStore(indexPath)
	symbols := []Symbol{
		{Name: "Persist", Kind: KindFunction, File: "persist.go", Line: 5, Language: "go"},
	}
	refs := []Reference{
		{SymbolName: "Persist", File: "persist.go", Line: 10, CallerName: "Save", CallerFile: "persist.go", CallerLine: 8},
	}
	err := store1.SaveFile(ctx, "persist.go", symbols, refs)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	err = store1.Persist(ctx)
	if err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Create new store at same path, load, verify
	store2 := NewGOBSymbolStore(indexPath)
	err = store2.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	result, err := store2.LookupSymbol(ctx, "Persist")
	if err != nil {
		t.Fatalf("LookupSymbol failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol after reload, got %d", len(result))
	}
	if result[0].File != "persist.go" {
		t.Errorf("expected file persist.go, got %s", result[0].File)
	}

	callers, err := store2.LookupCallers(ctx, "Persist")
	if err != nil {
		t.Fatalf("LookupCallers failed: %v", err)
	}
	if len(callers) != 1 {
		t.Fatalf("expected 1 caller after reload, got %d", len(callers))
	}

	if !store2.IsFileIndexed("persist.go") {
		t.Error("expected file indexed after reload")
	}
}

func TestGOBSymbolStore_should_build_call_graph(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "A", Kind: KindFunction, File: "graph.go", Line: 1, Language: "go"},
		{Name: "B", Kind: KindFunction, File: "graph.go", Line: 10, Language: "go"},
		{Name: "C", Kind: KindFunction, File: "graph.go", Line: 20, Language: "go"},
	}
	// A calls B, B calls C
	refs := []Reference{
		{SymbolName: "B", File: "graph.go", Line: 5, CallerName: "A", CallerFile: "graph.go", CallerLine: 1},
		{SymbolName: "C", File: "graph.go", Line: 15, CallerName: "B", CallerFile: "graph.go", CallerLine: 10},
	}

	err := store.SaveFile(ctx, "graph.go", symbols, refs)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "B", 2)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if graph.Root != "B" {
		t.Errorf("expected root B, got %s", graph.Root)
	}

	// B should have edges: A->B (caller) and B->C (callee)
	if len(graph.Edges) < 2 {
		t.Fatalf("expected at least 2 edges, got %d", len(graph.Edges))
	}

	edgeSet := map[string]bool{}
	for _, e := range graph.Edges {
		edgeSet[e.Caller+"->"+e.Callee] = true
	}
	if !edgeSet["A->B"] {
		t.Error("expected edge A->B in call graph")
	}
	if !edgeSet["B->C"] {
		t.Error("expected edge B->C in call graph")
	}

	// Nodes should include A, B, C
	if len(graph.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes, got %d", len(graph.Nodes))
	}
}

func TestGOBSymbolStore_should_get_stats(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols1 := []Symbol{
		{Name: "Foo", Kind: KindFunction, File: "a.go", Line: 1, Language: "go"},
		{Name: "Bar", Kind: KindFunction, File: "a.go", Line: 10, Language: "go"},
	}
	refs1 := []Reference{
		{SymbolName: "Foo", File: "a.go", Line: 5, CallerName: "Bar", CallerFile: "a.go", CallerLine: 10},
	}

	err := store.SaveFile(ctx, "a.go", symbols1, refs1)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	symbols2 := []Symbol{
		{Name: "Baz", Kind: KindFunction, File: "b.go", Line: 1, Language: "go"},
	}
	err = store.SaveFile(ctx, "b.go", symbols2, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalSymbols != 3 {
		t.Errorf("expected 3 total symbols, got %d", stats.TotalSymbols)
	}
	if stats.TotalReferences != 1 {
		t.Errorf("expected 1 total reference, got %d", stats.TotalReferences)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 total files, got %d", stats.TotalFiles)
	}
}

func TestGOBSymbolStore_should_replace_file_on_save(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	// First save
	symbols1 := []Symbol{
		{Name: "OldFunc", Kind: KindFunction, File: "replace.go", Line: 1, Language: "go"},
		{Name: "Shared", Kind: KindFunction, File: "replace.go", Line: 10, Language: "go"},
	}
	err := store.SaveFile(ctx, "replace.go", symbols1, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// Second save for the same file with different symbols
	symbols2 := []Symbol{
		{Name: "NewFunc", Kind: KindFunction, File: "replace.go", Line: 1, Language: "go"},
		{Name: "Shared", Kind: KindFunction, File: "replace.go", Line: 20, Language: "go"},
	}
	err = store.SaveFile(ctx, "replace.go", symbols2, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// OldFunc should be gone
	result, _ := store.LookupSymbol(ctx, "OldFunc")
	if len(result) != 0 {
		t.Errorf("expected OldFunc removed after re-save, got %d", len(result))
	}

	// NewFunc should exist
	result, _ = store.LookupSymbol(ctx, "NewFunc")
	if len(result) != 1 {
		t.Errorf("expected 1 NewFunc, got %d", len(result))
	}

	// Shared should exist exactly once (not duplicated)
	result, _ = store.LookupSymbol(ctx, "Shared")
	if len(result) != 1 {
		t.Errorf("expected 1 Shared (no duplicates), got %d", len(result))
	}
	if result[0].Line != 20 {
		t.Errorf("expected Shared at line 20 (updated), got %d", result[0].Line)
	}
}

func TestGOBSymbolStore_should_persist_creating_parent_directories(t *testing.T) {
	tmpDir := t.TempDir()
	// Nested path where parent .grepai/ doesn't exist yet
	indexPath := filepath.Join(tmpDir, "project", ".grepai", "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	symbols := []Symbol{
		{Name: "Foo", Kind: KindFunction, File: "main.go", Line: 1, Language: "go"},
	}
	err := store.SaveFile(ctx, "main.go", symbols, nil)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	err = store.Persist(ctx)
	if err != nil {
		t.Fatalf("Persist should create parent directories, got: %v", err)
	}

	// Verify file was written by loading into a new store
	store2 := NewGOBSymbolStore(indexPath)
	err = store2.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed after Persist with nested path: %v", err)
	}

	result, _ := store2.LookupSymbol(ctx, "Foo")
	if len(result) != 1 {
		t.Errorf("expected 1 symbol after reload from nested path, got %d", len(result))
	}
}

func TestGOBSymbolStore_should_return_empty_for_unknown_symbol(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	result, err := store.LookupSymbol(ctx, "NonExistent")
	if err != nil {
		t.Fatalf("LookupSymbol should not error for unknown symbol: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice for unknown symbol, got %d", len(result))
	}

	callers, err := store.LookupCallers(ctx, "NonExistent")
	if err != nil {
		t.Fatalf("LookupCallers should not error for unknown symbol: %v", err)
	}
	if len(callers) != 0 {
		t.Errorf("expected empty slice for unknown callers, got %d", len(callers))
	}
}

func TestGOBSymbolStore_ContentHashLifecycle(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	symbols := []Symbol{
		{
			Name:     "main",
			Kind:     KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}
	refs := []Reference{
		{
			SymbolName: "main",
			File:       "main.go",
			Line:       1,
			CallerName: "<top-level>",
		},
	}

	if err := store.SaveFileWithContentHash(ctx, "main.go", "hash-1", symbols, refs); err != nil {
		t.Fatalf("SaveFileWithContentHash failed: %v", err)
	}

	hash, ok := store.GetFileContentHash("main.go")
	if !ok || hash != "hash-1" {
		t.Fatalf("expected hash-1 in memory, got ok=%v hash=%q", ok, hash)
	}

	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hash, ok = reloaded.GetFileContentHash("main.go")
	if !ok || hash != "hash-1" {
		t.Fatalf("expected hash-1 after reload, got ok=%v hash=%q", ok, hash)
	}
	if !reloaded.IsFileIndexed("main.go") {
		t.Fatal("expected file to be marked indexed")
	}

	if err := reloaded.DeleteFile(ctx, "main.go"); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	if _, ok := reloaded.GetFileContentHash("main.go"); ok {
		t.Fatal("expected content hash to be removed on delete")
	}
	if reloaded.IsFileIndexed("main.go") {
		t.Fatal("expected file index marker to be removed on delete")
	}
}

func TestGOBSymbolStore_SaveFileClearsHashForBackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	if err := store.SaveFileWithContentHash(ctx, "main.go", "hash-1", nil, nil); err != nil {
		t.Fatalf("SaveFileWithContentHash failed: %v", err)
	}

	if err := store.SaveFile(ctx, "main.go", nil, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	if _, ok := store.GetFileContentHash("main.go"); ok {
		t.Fatal("expected SaveFile without hash to clear stored hash")
	}
}

func TestGOBSymbolStore_PersistCreatesMissingParentDir(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "missing", ".grepai", "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected persisted symbol index file at %s: %v", indexPath, err)
	}
}
