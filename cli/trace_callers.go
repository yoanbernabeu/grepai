package cli

import (
	"context"

	"github.com/yoanbernabeu/grepai/trace"
)

func lookupCallersFromStore(ctx context.Context, store trace.SymbolStore, symbolName string) (*trace.Symbol, []trace.CallerInfo, error) {
	lookup, err := trace.LookupCallerResult(ctx, store, symbolName)
	if err != nil {
		return nil, nil, err
	}
	refs := lookup.References
	target := pickBestTargetSymbol(lookup.Symbols[symbolName], refs)
	callers := make([]trace.CallerInfo, 0, len(refs))
	for _, ref := range refs {
		callerSyms := lookup.Symbols[ref.CallerName]
		var callerSym trace.Symbol
		if picked := pickBestSymbolForFile(callerSyms, ref.CallerFile); picked != nil {
			callerSym = *picked
		} else {
			callerSym = trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
		}
		callers = append(callers, trace.CallerInfo{
			Symbol:   callerSym,
			CallSite: trace.CallSite{File: ref.File, Line: ref.Line, Context: ref.Context},
		})
	}
	return target, callers, nil
}
