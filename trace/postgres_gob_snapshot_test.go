package trace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLockedGOBSymbolSnapshotLoadsLegacyData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.gob")
	seed := NewGOBSymbolStore(path)
	symbol := Symbol{Name: "Target", File: "target.go", Line: 3, Kind: KindFunction}
	if err := seed.SaveFileWithSignature(context.Background(), symbol.File, "hash", "extractor", []Symbol{symbol}, nil); err != nil {
		t.Fatalf("SaveFileWithSignature: %v", err)
	}
	if err := seed.Persist(context.Background()); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	store, loaded, err := loadLockedGOBSymbolSnapshot(path)
	if err != nil {
		t.Fatalf("loadLockedGOBSymbolSnapshot: %v", err)
	}
	if !loaded {
		t.Fatal("loaded = false, want true")
	}
	got, err := store.LookupSymbol(context.Background(), symbol.Name)
	if err != nil || len(got) != 1 || got[0] != symbol {
		t.Fatalf("LookupSymbol = %#v, %v", got, err)
	}
}

func TestLoadLockedGOBSymbolSnapshotMissingIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.gob")
	store, loaded, err := loadLockedGOBSymbolSnapshot(path)
	if err != nil {
		t.Fatalf("loadLockedGOBSymbolSnapshot: %v", err)
	}
	if loaded {
		t.Fatal("loaded = true, want false")
	}
	if store == nil {
		t.Fatal("store = nil")
	}
}

func TestLoadLockedGOBSymbolSnapshotRejectsCorruptData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.gob")
	if err := os.WriteFile(path, []byte("not gob"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := loadLockedGOBSymbolSnapshot(path); err == nil {
		t.Fatal("corrupt GOB load succeeded")
	}
}
