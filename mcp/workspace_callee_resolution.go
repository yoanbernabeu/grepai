package mcp

import (
	"context"
	"log"

	"github.com/yoanbernabeu/grepai/trace"
)

// resolveCalleeSymbol resolves a callee reference to its definition,
// preferring the originating store, then a cross-project fallback, then a
// name-only placeholder when no loaded store defines the symbol.
func resolveCalleeSymbol(originByName map[string][]trace.Symbol, crossProject map[string]trace.Symbol, name string) trace.Symbol {
	if defs := originByName[name]; len(defs) > 0 {
		return defs[0]
	}
	if sym, ok := crossProject[name]; ok {
		return sym
	}
	return trace.Symbol{Name: name}
}

// lookupMissingCalleeSymbols resolves callee names that their originating
// store could not define by falling back to the remaining loaded stores in
// deterministic loaded order. Missingness is evaluated per reference origin
// before name dedup, so a name defined by one reference's origin still earns
// a fallback for a later reference whose origin lacks it. Each store receives
// at most one batch query for the still-unresolved names, so the fallback
// adds no per-callee point lookups. Single-store (project mode) resolution
// is unchanged.
func lookupMissingCalleeSymbols(ctx context.Context, stores []trace.SymbolStore, refs []storeReference, origin []map[string][]trace.Symbol) []map[string]trace.Symbol {
	resolved := make([]map[string]trace.Symbol, len(stores))
	for i := range resolved {
		resolved[i] = make(map[string]trace.Symbol)
	}
	if len(stores) < 2 {
		return resolved
	}
	missing := make([][]string, len(stores))
	seen := make([]map[string]struct{}, len(stores))
	for i := range seen {
		seen[i] = make(map[string]struct{})
	}
	for _, item := range refs {
		name := item.ref.SymbolName
		if len(origin[item.storeIndex][name]) > 0 {
			continue
		}
		if _, ok := seen[item.storeIndex][name]; ok {
			continue
		}
		seen[item.storeIndex][name] = struct{}{}
		missing[item.storeIndex] = append(missing[item.storeIndex], name)
	}
	for fallbackIndex, store := range stores {
		queryNames := make([]string, 0)
		querySeen := make(map[string]struct{})
		for originIndex := range stores {
			if originIndex == fallbackIndex {
				continue
			}
			for _, name := range missing[originIndex] {
				if _, ok := resolved[originIndex][name]; ok {
					continue
				}
				if _, ok := querySeen[name]; !ok {
					querySeen[name] = struct{}{}
					queryNames = append(queryNames, name)
				}
			}
		}
		if len(queryNames) == 0 {
			continue
		}
		symbols, err := store.LookupSymbolsBatch(ctx, queryNames)
		if err != nil {
			log.Printf("Warning: failed to lookup cross-project callee symbols: %v", err)
			continue
		}
		for originIndex := range stores {
			if originIndex == fallbackIndex {
				continue
			}
			for _, name := range missing[originIndex] {
				if _, ok := resolved[originIndex][name]; ok {
					continue
				}
				if defs := symbols[name]; len(defs) > 0 {
					resolved[originIndex][name] = defs[0]
				}
			}
		}
	}
	return resolved
}
