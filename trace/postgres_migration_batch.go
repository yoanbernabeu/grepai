package trace

import (
	"sort"
	"time"
)

// migrationRefKey identifies a dedup bucket of references: GOB stores
// references grouped by symbol name, while the migration must emit them in
// per-file call-graph order.
type migrationRefKey struct {
	caller string
	callee string
	file   string
	line   int
}

// migrationBatchIterator owns the freshly decoded GOB index maps for the
// duration of a Postgres import and hands out one bounded batch of files at
// a time. The constructor only takes over the original map/slice references
// and builds O(files+names) key metadata — it never copies row structs.
// nextBatch rescans the remaining decoded name buckets in place, copies out
// only the selected batch's rows, and zeroes consumed source slots so the
// decoded snapshot shrinks as batches advance. Row storage beyond the
// decoded source therefore scales with the current batch, not the whole
// index. The tradeoff is CPU: each batch rescans the surviving symbol,
// reference, and call-edge slots, costing O(batches × (symbols + refs +
// edges)) worst case (e.g. one name bucket spanning every file).
type migrationBatchIterator struct {
	files             []string
	next              int
	contentHashes     map[string]string
	extractorVersions map[string]string
	symbolsByName     map[string][]Symbol
	symbolNames       []string
	symbolsLeft       map[string]int
	refsByName        map[string][]Reference
	refNames          []string
	refsLeft          map[string]int
	callGraph         []CallEdge
}

// newMigrationBatchIterator takes ownership of the decoded store collections
// without copying any rows; the store relinquishes them.
func newMigrationBatchIterator(store *GOBSymbolStore) *migrationBatchIterator {
	it := &migrationBatchIterator{
		files:             make([]string, 0, len(store.fileIndex)),
		contentHashes:     store.fileContentHashes,
		extractorVersions: store.fileExtractorVersions,
		symbolsByName:     store.index.Symbols,
		symbolNames:       sortedMigrationKeys(store.index.Symbols),
		symbolsLeft:       make(map[string]int, len(store.index.Symbols)),
		refsByName:        store.index.References,
		refNames:          sortedMigrationKeys(store.index.References),
		refsLeft:          make(map[string]int, len(store.index.References)),
		callGraph:         store.index.CallGraph,
	}
	for name, symbols := range it.symbolsByName {
		it.symbolsLeft[name] = len(symbols)
	}
	for name, refs := range it.refsByName {
		it.refsLeft[name] = len(refs)
	}
	for file := range store.fileIndex {
		it.files = append(it.files, file)
	}
	sort.Strings(it.files)
	store.index.Symbols = nil
	store.index.References = nil
	store.index.CallGraph = nil
	return it
}

func sortedMigrationKeys[V any](byName map[string][]V) []string {
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// progress reports files consumed so far and the total, for migration logs.
func (it *migrationBatchIterator) progress() (done, total int) {
	return it.next, len(it.files)
}

// nextBatch materializes row values for the next bounded run of files and
// releases the source slots they were drained from. ok is false once every
// file has been emitted.
func (it *migrationBatchIterator) nextBatch() (batch []migrationFileRows, ok bool) {
	if it.next >= len(it.files) {
		return nil, false
	}
	end := min(it.next+migrationBatchSize, len(it.files))
	batchFiles := it.files[it.next:end]
	it.next = end
	selected := make(map[string]struct{}, len(batchFiles))
	for _, file := range batchFiles {
		selected[file] = struct{}{}
	}
	symbols := it.takeSelectedSymbols(selected)
	refs := it.takeSelectedRefs(selected)
	batch = make([]migrationFileRows, 0, len(batchFiles))
	for _, file := range batchFiles {
		batch = append(batch, migrationFileRows{
			filePath:         file,
			contentHash:      it.contentHashes[file],
			extractorVersion: it.extractorVersions[file],
			symbols:          symbols[file],
			refs:             refs[file],
			modTime:          time.Now().UTC(),
		})
	}
	return batch, true
}

// takeSelectedSymbols copies out only the selected files' symbols, zeroing
// the consumed source slots and deleting fully drained name buckets.
// Sorted name iteration keeps per-file symbol order deterministic.
func (it *migrationBatchIterator) takeSelectedSymbols(selected map[string]struct{}) map[string][]Symbol {
	out := make(map[string][]Symbol, len(selected))
	for _, name := range it.symbolNames {
		slice, ok := it.symbolsByName[name]
		if !ok {
			continue
		}
		for i, symbol := range slice {
			if _, hit := selected[symbol.File]; !hit {
				continue
			}
			out[symbol.File] = append(out[symbol.File], symbol)
			slice[i] = Symbol{}
			it.symbolsLeft[name]--
		}
		if it.symbolsLeft[name] == 0 {
			delete(it.symbolsByName, name)
			delete(it.symbolsLeft, name)
		}
	}
	return out
}

// takeSelectedRefs reconstructs the selected files' reference order exactly
// like the previous whole-index grouping. It first copies the selected rows
// into per-batch dedup buckets keyed by (caller, callee, file, line),
// zeroing consumed source slots; every ref in one bucket shares the same
// name slice, so in-slice insertion order is preserved. Call-graph edges in
// their original order then consume one bucket entry each, and unconsumed
// leftovers (empty/top-level callers or duplicates beyond the edge count)
// follow in first-encounter bucket order — deterministic because name
// iteration is sorted. Buckets and cursors never cross files because the
// key includes the file, so all of this stays batch-local.
func (it *migrationBatchIterator) takeSelectedRefs(selected map[string]struct{}) map[string][]Reference {
	buckets := make(map[migrationRefKey][]Reference)
	bucketFiles := make(map[string][]migrationRefKey, len(selected))
	for _, name := range it.refNames {
		slice, ok := it.refsByName[name]
		if !ok {
			continue
		}
		for i, ref := range slice {
			if _, hit := selected[ref.File]; !hit {
				continue
			}
			key := migrationRefKey{caller: ref.CallerName, callee: ref.SymbolName, file: ref.File, line: ref.Line}
			if _, seen := buckets[key]; !seen {
				bucketFiles[ref.File] = append(bucketFiles[ref.File], key)
			}
			buckets[key] = append(buckets[key], ref)
			slice[i] = Reference{}
			it.refsLeft[name]--
		}
		if it.refsLeft[name] == 0 {
			delete(it.refsByName, name)
			delete(it.refsLeft, name)
		}
	}
	out := make(map[string][]Reference, len(selected))
	consumed := make(map[migrationRefKey]int, len(buckets))
	for i, edge := range it.callGraph {
		if edge == (CallEdge{}) {
			continue
		}
		if _, hit := selected[edge.File]; !hit {
			continue
		}
		key := migrationRefKey{caller: edge.Caller, callee: edge.Callee, file: edge.File, line: edge.Line}
		if next := consumed[key]; next < len(buckets[key]) {
			out[edge.File] = append(out[edge.File], buckets[key][next])
			consumed[key] = next + 1
		}
		it.callGraph[i] = CallEdge{}
	}
	for file, keys := range bucketFiles {
		for _, key := range keys {
			out[file] = append(out[file], buckets[key][consumed[key]:]...)
		}
	}
	return out
}
