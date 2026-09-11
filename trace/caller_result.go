package trace

import "context"

// ReferenceLookupResult contains references and the definitions needed to
// resolve their endpoints. Symbols is keyed by symbol name.
type ReferenceLookupResult struct {
	Symbols    map[string][]Symbol
	References []Reference
}

// CallerLookupResult is the compound result used by caller traces.
type CallerLookupResult = ReferenceLookupResult

// CalleeLookupResult is the compound result used by callee traces.
type CalleeLookupResult = ReferenceLookupResult

// RefsLookupResult contains both read and write references plus their caller
// definitions. Kind-specific consumers filter References after this lookup.
type RefsLookupResult = ReferenceLookupResult

// CallerResultStore optionally reads all data needed for a caller trace from a
// single store snapshot.
type CallerResultStore interface {
	LookupCallerResult(ctx context.Context, symbolName string) (CallerLookupResult, error)
}

// CalleeResultStore optionally reads all data needed for a callee trace from a
// single store snapshot.
type CalleeResultStore interface {
	LookupCalleeResult(ctx context.Context, symbolName, file string) (CalleeLookupResult, error)
}

// RefsResultStore optionally reads readers, writers, and their caller
// definitions from a single store snapshot.
type RefsResultStore interface {
	LookupRefsResult(ctx context.Context, symbolName string) (RefsLookupResult, error)
}

// LookupCallerResult prefers a store's compound snapshot capability. Stores
// without it retain the loaded-store lookup path with one deterministic batch
// for the target and all unique caller names.
func LookupCallerResult(ctx context.Context, store SymbolStore, symbolName string) (CallerLookupResult, error) {
	if source, ok := store.(CallerResultStore); ok {
		result, err := source.LookupCallerResult(ctx, symbolName)
		if err != nil {
			return CallerLookupResult{}, err
		}
		return result, nil
	}

	refs, err := store.LookupCallers(ctx, symbolName)
	if err != nil {
		return CallerLookupResult{}, err
	}
	names := make([]string, 0, len(refs)+1)
	names = append(names, symbolName)
	seen := map[string]struct{}{symbolName: {}}
	for _, ref := range refs {
		if _, ok := seen[ref.CallerName]; ok {
			continue
		}
		seen[ref.CallerName] = struct{}{}
		names = append(names, ref.CallerName)
	}
	symbols, err := store.LookupSymbolsBatch(ctx, names)
	if err != nil {
		return CallerLookupResult{}, err
	}
	return CallerLookupResult{Symbols: symbols, References: refs}, nil
}

// LookupCalleeResult prefers a compound store snapshot and otherwise uses the
// loaded store's existing methods with one batch definition lookup.
func LookupCalleeResult(ctx context.Context, store SymbolStore, symbolName, file string) (CalleeLookupResult, error) {
	if source, ok := store.(CalleeResultStore); ok {
		result, err := source.LookupCalleeResult(ctx, symbolName, file)
		if err != nil {
			return CalleeLookupResult{}, err
		}
		return result, nil
	}
	if file == "" {
		targets, err := store.LookupSymbol(ctx, symbolName)
		if err != nil {
			return CalleeLookupResult{}, err
		}
		if len(targets) == 0 {
			return CalleeLookupResult{Symbols: map[string][]Symbol{}}, nil
		}
		file = targets[0].File
	}
	refs, err := store.LookupCallees(ctx, symbolName, file)
	if err != nil {
		return CalleeLookupResult{}, err
	}
	names := referenceDefinitionNames(symbolName, refs, false)
	symbols, err := store.LookupSymbolsBatch(ctx, names)
	if err != nil {
		return CalleeLookupResult{}, err
	}
	return CalleeLookupResult{Symbols: symbols, References: refs}, nil
}

// LookupRefsResult prefers a compound store snapshot and otherwise reads both
// access kinds before batching their caller definitions.
func LookupRefsResult(ctx context.Context, store SymbolStore, symbolName string) (RefsLookupResult, error) {
	if source, ok := store.(RefsResultStore); ok {
		result, err := source.LookupRefsResult(ctx, symbolName)
		if err != nil {
			return RefsLookupResult{}, err
		}
		return result, nil
	}
	readers, err := store.LookupReaders(ctx, symbolName)
	if err != nil {
		return RefsLookupResult{}, err
	}
	writers, err := store.LookupWriters(ctx, symbolName)
	if err != nil {
		return RefsLookupResult{}, err
	}
	refs := append(append([]Reference(nil), readers...), writers...)
	symbols, err := store.LookupSymbolsBatch(ctx, referenceDefinitionNames("", refs, true))
	if err != nil {
		return RefsLookupResult{}, err
	}
	return RefsLookupResult{Symbols: symbols, References: refs}, nil
}

func referenceDefinitionNames(target string, refs []Reference, callers bool) []string {
	names := make([]string, 0, len(refs)+1)
	seen := make(map[string]struct{}, len(refs)+1)
	if target != "" {
		names = append(names, target)
		seen[target] = struct{}{}
	}
	for _, ref := range refs {
		name := ref.SymbolName
		if callers {
			name = ref.CallerName
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}
