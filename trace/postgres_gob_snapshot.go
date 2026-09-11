package trace

import (
	"encoding/gob"
	"fmt"
	"os"
)

// loadLockedGOBSymbolSnapshot decodes a legacy GOB while the migration caller
// holds its exclusive file lock. It intentionally avoids GOBSymbolStore.Load,
// which would acquire the same lock again and can deadlock.
func loadLockedGOBSymbolSnapshot(indexPath string) (*GOBSymbolStore, bool, error) {
	file, err := os.Open(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewGOBSymbolStore(indexPath), false, nil
		}
		return nil, false, fmt.Errorf("failed to open symbol index: %w", err)
	}
	defer file.Close()

	var data gobSymbolData
	// #nosec G709 -- intentional local-cache migration into a fixed data-only schema; untrusted GOB imports are unsupported.
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		return nil, false, fmt.Errorf("failed to decode symbol index: %w", err)
	}
	store := NewGOBSymbolStore(indexPath)
	store.index = &data.Index
	store.fileIndex = data.FileIndex
	store.fileContentHashes = data.FileContentHashes
	store.fileExtractorVersions = data.FileExtractorVersions
	if store.index.Symbols == nil {
		store.index.Symbols = make(map[string][]Symbol)
	}
	if store.index.References == nil {
		store.index.References = make(map[string][]Reference)
	}
	if store.index.CallGraph == nil {
		store.index.CallGraph = []CallEdge{}
	}
	if store.fileIndex == nil {
		store.fileIndex = make(map[string]bool)
	}
	if store.fileContentHashes == nil {
		store.fileContentHashes = make(map[string]string)
	}
	if store.fileExtractorVersions == nil {
		store.fileExtractorVersions = make(map[string]string)
	}
	return store, true, nil
}
