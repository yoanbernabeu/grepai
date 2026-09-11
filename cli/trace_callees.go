package cli

import (
	"context"

	"github.com/yoanbernabeu/grepai/trace"
)

func lookupCalleesFromStore(ctx context.Context, store trace.SymbolStore, symbolName string) (*trace.Symbol, []trace.CalleeInfo, error) {
	lookup, err := trace.LookupCalleeResult(ctx, store, symbolName, "")
	if err != nil {
		return nil, nil, err
	}
	refs := lookup.References
	target := pickBestTargetSymbol(lookup.Symbols[symbolName], refs)
	callees := make([]trace.CalleeInfo, 0, len(refs))
	for _, ref := range refs {
		callee := trace.Symbol{Name: ref.SymbolName}
		if definitions := lookup.Symbols[ref.SymbolName]; len(definitions) > 0 {
			callee = definitions[0]
		}
		callees = append(callees, trace.CalleeInfo{
			Symbol:   callee,
			CallSite: trace.CallSite{File: ref.File, Line: ref.Line, Context: ref.Context},
		})
	}
	return target, callees, nil
}
