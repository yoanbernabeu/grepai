package trace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestGOBSymbolStore_CleanPersistIsNoOp(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()

	seed := NewGOBSymbolStore(indexPath)
	if err := seed.SaveFile(ctx, "main.go", []Symbol{{Name: "main", File: "main.go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := seed.Persist(ctx); err != nil {
		t.Fatalf("seed Persist failed: %v", err)
	}

	store := NewGOBSymbolStore(indexPath)
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	oldTime := time.Unix(1, 0)
	if err := os.Chtimes(indexPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("first clean Persist failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("second clean Persist failed: %v", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatal("clean Persist rewrote index")
	}
}

func TestGOBSymbolStore_LoadMissingIndexPreservesPendingChanges(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()
	store := NewGOBSymbolStore(indexPath)

	if err := store.SaveFile(ctx, "main.go", []Symbol{{Name: "main", File: "main.go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load missing index failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	symbols, err := reloaded.LookupSymbol(ctx, "main")
	if err != nil {
		t.Fatalf("LookupSymbol failed: %v", err)
	}
	if len(symbols) != 1 || symbols[0].File != "main.go" {
		t.Fatalf("pending symbols were lost: %#v", symbols)
	}
}

func TestGOBSymbolStore_UntouchedMissingReaderCloseWritesNothing(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(indexPath)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load missing index failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("untouched missing-index reader wrote %s: %v", indexPath, err)
	}
}

func TestGOBSymbolStore_MissingReaderCannotOverwriteLaterWriter(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()
	reader := NewGOBSymbolStore(indexPath)
	if err := reader.Load(ctx); err != nil {
		t.Fatalf("reader Load failed: %v", err)
	}

	writer := NewGOBSymbolStore(indexPath)
	symbol := Symbol{Name: "Writer", File: "writer.go", Kind: KindFunction}
	if err := writer.SaveFile(ctx, "writer.go", []Symbol{symbol}, nil); err != nil {
		t.Fatalf("writer SaveFile failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer Close failed: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("reader Close failed: %v", err)
	}

	check := NewGOBSymbolStore(indexPath)
	if err := check.Load(ctx); err != nil {
		t.Fatalf("check Load failed: %v", err)
	}
	symbols, err := check.LookupSymbol(ctx, "Writer")
	if err != nil || len(symbols) != 1 || symbols[0].File != "writer.go" {
		t.Fatalf("writer symbols were overwritten: symbols=%#v err=%v", symbols, err)
	}
}

func TestGOBSymbolStoreReturnsOwnedSlices(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()
	store := NewGOBSymbolStore(indexPath)
	symbols := []Symbol{{Name: "Target", File: "main.go", Kind: KindFunction}}
	refs := []Reference{{SymbolName: "Target", File: "main.go", Kind: RefKindCall, CallerName: "Caller"}}
	if err := store.SaveFile(ctx, "main.go", symbols, refs); err != nil {
		t.Fatal(err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatal(err)
	}
	symbols[0].Name = "input-mutated"
	refs[0].CallerName = "input-mutated"
	gotSymbols, _ := store.LookupSymbol(ctx, "Target")
	gotSymbols[0].Name = "getter-mutated"
	gotCallers, _ := store.LookupCallers(ctx, "Target")
	gotCallers[0].CallerName = "getter-mutated"
	gotForFile, _ := store.GetSymbolsForFile(ctx, "main.go")
	gotForFile[0].Name = "file-getter-mutated"
	gotSymbols, _ = store.LookupSymbol(ctx, "Target")
	gotCallers, _ = store.LookupCallers(ctx, "Target")
	if gotSymbols[0].Name != "Target" || gotCallers[0].CallerName != "Caller" {
		t.Fatalf("store state changed through returned slice: symbols=%#v callers=%#v", gotSymbols, gotCallers)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatal(err)
	}
	gotSymbols, _ = reloaded.LookupSymbol(ctx, "Target")
	gotCallers, _ = reloaded.LookupCallers(ctx, "Target")
	if len(gotSymbols) != 1 || gotSymbols[0].Name != "Target" || len(gotCallers) != 1 || gotCallers[0].CallerName != "Caller" {
		t.Fatalf("persisted symbol data changed through alias: symbols=%#v callers=%#v", gotSymbols, gotCallers)
	}
}

func TestGOBSymbolStore_DeleteFileRemovesExtractorVersion(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()
	store := NewGOBSymbolStore(indexPath)

	if err := store.SaveFileWithSignature(ctx, "main.go", "hash", "extractor-v1", nil, nil); err != nil {
		t.Fatalf("SaveFileWithSignature failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("initial Persist failed: %v", err)
	}
	if err := store.DeleteFile(ctx, "main.go"); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("delete Persist failed: %v", err)
	}

	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if _, ok := reloaded.GetFileExtractorVersion("main.go"); ok {
		t.Fatal("extractor version survived DeleteFile")
	}
}

func TestGOBSymbolStore_SaveFileWithSignaturePersistsBothFingerprints(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	ctx := context.Background()
	store := NewGOBSymbolStore(indexPath)

	if err := store.SaveFileWithSignature(ctx, "main.go", "hash", "extractor-v1", nil, nil); err != nil {
		t.Fatalf("SaveFileWithSignature failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}
	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	hash, hashOK := reloaded.GetFileContentHash("main.go")
	version, versionOK := reloaded.GetFileExtractorVersion("main.go")
	if !hashOK || hash != "hash" || !versionOK || version != "extractor-v1" {
		t.Fatalf("fingerprints = hash(%q,%v) version(%q,%v)", hash, hashOK, version, versionOK)
	}
}

func TestGOBSymbolStore_LoadFallsBackWhenLockFileCannotOpen(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "symbols.gob")
	ctx := context.Background()
	seed := NewGOBSymbolStore(indexPath)
	if err := seed.SaveFile(ctx, "main.go", []Symbol{{Name: "main", File: "main.go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := seed.Persist(ctx); err != nil {
		t.Fatalf("seed Persist failed: %v", err)
	}

	store := NewGOBSymbolStore(indexPath)
	store.lockPath = dir // OpenFile(O_RDWR) on a directory fails; Load must use its fallback.
	if err := store.Load(ctx); err != nil {
		t.Fatalf("fallback Load failed: %v", err)
	}
	symbols, err := store.LookupSymbol(ctx, "main")
	if err != nil || len(symbols) != 1 || symbols[0].File != "main.go" {
		t.Fatalf("fallback Load symbols = %#v, %v", symbols, err)
	}
}

func TestGOBSymbolStore_LoadUnreadableIndexReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions required")
	}
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "symbols.gob")
	if err := os.WriteFile(indexPath, []byte("symbols"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	defer os.Chmod(dir, 0o700)

	store := NewGOBSymbolStore(indexPath)
	if err := store.Load(context.Background()); err == nil {
		t.Fatal("Load succeeded for unreadable symbol index")
	}
}

func TestGOBSymbolStore_LoadCorruptIndexReturnsError(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	if err := os.WriteFile(indexPath, []byte("not gob"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	store := NewGOBSymbolStore(indexPath)
	if err := store.Load(context.Background()); err == nil {
		t.Fatal("Load succeeded for corrupt symbol index")
	}
}

func TestGOBSymbolStore_DirtyPersistWritesOnce(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	if err := store.Persist(ctx); err != nil {
		t.Fatalf("initial Persist failed: %v", err)
	}
	oldTime := time.Unix(1, 0)
	if err := os.Chtimes(indexPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	if err := store.SaveFile(ctx, "main.go", []Symbol{{Name: "main", File: "main.go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("dirty Persist failed: %v", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.ModTime().Equal(oldTime) {
		t.Fatal("dirty Persist did not rewrite index")
	}
	if err := os.Chtimes(indexPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("clean Persist failed: %v", err)
	}
	info, err = os.Stat(indexPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatal("clean Persist rewrote index")
	}
}

func TestGOBSymbolStore_FailedPersistStaysDirty(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")
	ctx := context.Background()
	store := NewGOBSymbolStore(indexPath)

	if err := store.SaveFile(ctx, "main.go", []Symbol{{Name: "main", File: "main.go"}}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := os.Mkdir(indexPath, 0o755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	blocker := filepath.Join(indexPath, "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := store.Persist(ctx); err == nil {
		t.Fatal("Persist should fail when target is a non-empty directory")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatalf("Remove blocker failed: %v", err)
	}
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("Remove target directory failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("retry Persist failed: %v", err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("retry Persist did not write index: %v", err)
	}
}

func TestGOBSymbolStore_CleanCloseIsNoOp(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(indexPath)
	ctx := context.Background()

	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}
	oldTime := time.Unix(1, 0)
	if err := os.Chtimes(indexPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatal("clean Close rewrote index")
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

func TestGOBSymbolStore_PersistCreatesLockFileAndNoTempFiles(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "symbols.gob")

	store := NewGOBSymbolStore(indexPath)
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	lockPath := indexPath + ".lock"
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file at %s: %v", lockPath, err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "symbols.gob.tmp-") {
			t.Fatalf("unexpected temporary file left behind: %s", entry.Name())
		}
	}
}

func TestGOBSymbolStore_ReferenceKindFilters(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(indexPath)

	symbols := []Symbol{
		{Name: "uidConsumer", Kind: KindFunction, File: "store.ts", Line: 1, Language: "typescript"},
	}
	refs := []Reference{
		{SymbolName: "uid", Kind: RefKindRead, File: "store.ts", Line: 2, CallerName: "uidConsumer", CallerFile: "store.ts"},
		{SymbolName: "uid", Kind: RefKindWrite, File: "store.ts", Line: 3, CallerName: "uidConsumer", CallerFile: "store.ts"},
		{SymbolName: "uid", Kind: RefKindCall, File: "store.ts", Line: 4, CallerName: "uidConsumer", CallerFile: "store.ts"},
	}

	if err := store.SaveFile(ctx, "store.ts", symbols, refs); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	callers, err := store.LookupCallers(ctx, "uid")
	if err != nil {
		t.Fatalf("LookupCallers failed: %v", err)
	}
	if len(callers) != 1 || callers[0].Kind != RefKindCall {
		t.Fatalf("expected only call refs, got %+v", callers)
	}

	readers, err := store.LookupReaders(ctx, "uid")
	if err != nil {
		t.Fatalf("LookupReaders failed: %v", err)
	}
	if len(readers) != 1 || readers[0].Kind != RefKindRead {
		t.Fatalf("expected only read refs, got %+v", readers)
	}

	writers, err := store.LookupWriters(ctx, "uid")
	if err != nil {
		t.Fatalf("LookupWriters failed: %v", err)
	}
	if len(writers) != 1 || writers[0].Kind != RefKindWrite {
		t.Fatalf("expected only write refs, got %+v", writers)
	}
}

func TestGOBSymbolStore_LookupSymbolsBatch(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := store.SaveFile(ctx, "one.go", []Symbol{
		{Name: "Shared", File: "one.go", Line: 1},
		{Name: "OnlyOne", File: "one.go", Line: 2},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(ctx, "two.go", []Symbol{{Name: "Shared", File: "two.go", Line: 3}}, nil); err != nil {
		t.Fatal(err)
	}

	got, err := store.LookupSymbolsBatch(ctx, []string{"Shared", "Missing", "OnlyOne", "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got["Shared"]) != 2 || len(got["OnlyOne"]) != 1 {
		t.Fatalf("unexpected grouped symbols: %#v", got)
	}
	if _, ok := got["Missing"]; ok {
		t.Fatalf("missing name should not be present: %#v", got)
	}
	empty, err := store.LookupSymbolsBatch(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty lookup = %#v, %v", empty, err)
	}
}

func TestGOBSymbolStore_LookupSymbolsBatchReturnsCallerOwnedSlices(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")
	store := NewGOBSymbolStore(indexPath)

	symbols := []Symbol{
		{Name: "Shared", Kind: KindFunction, File: "one.go", Line: 1, Language: "go"},
		{Name: "Shared", Kind: KindFunction, File: "two.go", Line: 3, Language: "go"},
	}
	if err := store.SaveFile(ctx, "one.go", symbols[:1], nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(ctx, "two.go", symbols[1:], nil); err != nil {
		t.Fatal(err)
	}

	batch, err := store.LookupSymbolsBatch(ctx, []string{"Shared"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || len(batch["Shared"]) != 2 {
		t.Fatalf("unexpected batch result: %#v", batch)
	}

	// Mutate the caller-owned result: element fields, re-slicing, and append.
	batch["Shared"][0].File = "hacked.go"
	batch["Shared"][0].Line = 999
	batch["Shared"] = batch["Shared"][:1]
	batch["Shared"] = append(batch["Shared"], Symbol{Name: "Shared", File: "injected.go"})

	// Clean in-memory state must be unaffected by the caller's mutations.
	fresh, err := store.LookupSymbol(ctx, "Shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 2 {
		t.Fatalf("expected 2 symbols after caller mutation, got %d", len(fresh))
	}
	for _, sym := range fresh {
		if sym.File == "hacked.go" || sym.Line == 999 {
			t.Fatalf("caller mutation leaked into store state: %+v", fresh)
		}
		if sym.File != "one.go" && sym.File != "two.go" {
			t.Fatalf("unexpected symbol file after caller mutation: %+v", fresh)
		}
	}

	// Persisted state must also be unaffected.
	if err := store.Persist(ctx); err != nil {
		t.Fatal(err)
	}
	reloaded := NewGOBSymbolStore(indexPath)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatal(err)
	}
	persisted, err := reloaded.LookupSymbol(ctx, "Shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected 2 symbols after reload, got %d", len(persisted))
	}
	for _, sym := range persisted {
		if sym.File == "hacked.go" || sym.File == "injected.go" {
			t.Fatalf("caller mutation leaked into persisted state: %+v", persisted)
		}
		if sym.Line == 999 {
			t.Fatalf("caller mutation leaked into persisted state: %+v", persisted)
		}
	}
}

func TestGOBSymbolStore_LookupSymbolsBatchConcurrentMutation(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	symbols := []Symbol{
		{Name: "Shared", Kind: KindFunction, File: "one.go", Line: 1, Language: "go"},
		{Name: "Shared", Kind: KindFunction, File: "two.go", Line: 3, Language: "go"},
	}
	if err := store.SaveFile(ctx, "one.go", symbols[:1], nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFile(ctx, "two.go", symbols[1:], nil); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			batch, err := store.LookupSymbolsBatch(ctx, []string{"Shared"})
			if err != nil {
				t.Error(err)
				return
			}
			// Caller-owned results may be freely mutated without touching
			// store-owned memory.
			for j := range batch["Shared"] {
				batch["Shared"][j].File = "mutated.go"
				batch["Shared"][j].Line = i
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := store.LookupSymbol(ctx, "Shared"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
