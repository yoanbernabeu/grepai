package trace

import "context"

// SymbolCounter optionally exposes a symbol count without computing full
// index statistics. Other SymbolStore implementations retain the GetStats fallback.
type SymbolCounter interface {
	CountSymbols(ctx context.Context) (int, error)
}

// CountSymbolsForReadiness uses a store's optional counter, falling back to
// GetStats only when no counter is available, never after a counter error.
func CountSymbolsForReadiness(ctx context.Context, store SymbolStore) (int, error) {
	if counter, ok := store.(SymbolCounter); ok {
		total, err := counter.CountSymbols(ctx)
		if err != nil {
			return 0, err
		}
		return total, nil
	}
	stats, err := store.GetStats(ctx)
	if err != nil {
		return 0, err
	}
	return stats.TotalSymbols, nil
}
