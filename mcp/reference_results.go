package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/yoanbernabeu/grepai/trace"
)

func lookupRefUsagesFromStores(ctx context.Context, stores []trace.SymbolStore, symbolName string) ([]RefUsage, []RefUsage) {
	readers := make([]RefUsage, 0)
	writers := make([]RefUsage, 0)
	for _, store := range stores {
		lookup, err := trace.LookupRefsResult(ctx, store, symbolName)
		if err != nil {
			log.Printf("Warning: failed to lookup refs of %q: %v", symbolName, err)
			continue
		}
		for _, ref := range lookup.References {
			usage := RefUsage{
				Symbol:   resolveRefCallerSymbol(lookup.Symbols, ref),
				Access:   ref.Kind,
				AccessAt: trace.CallSite{File: ref.File, Line: ref.Line, Context: ref.Context},
			}
			switch ref.Kind {
			case trace.RefKindRead:
				readers = append(readers, usage)
			case trace.RefKindWrite:
				writers = append(writers, usage)
			}
		}
	}
	return readers, writers
}

func (s *Server) handleRefsGraphFromStores(ctx context.Context, symbolName string, compact bool, format string, stores []trace.SymbolStore) (*mcp.CallToolResult, error) {
	readers, writers := lookupRefUsagesFromStores(ctx, stores, symbolName)
	var data any
	if compact {
		data = map[string]any{
			"query": symbolName, "kind": "property", "mode": "fast",
			"readers": compactRefUsages(readers), "writers": compactRefUsages(writers),
		}
	} else {
		data = map[string]any{
			"query": symbolName, "kind": "property", "mode": "fast",
			"readers": readers, "writers": writers,
		}
	}
	output, err := encodeOutput(data, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode results: %v", err)), nil
	}
	return mcp.NewToolResultText(output), nil
}

func compactRefUsages(usages []RefUsage) []RefUsageCompact {
	result := make([]RefUsageCompact, 0, len(usages))
	for _, usage := range usages {
		result = append(result, RefUsageCompact{
			Symbol:   usage.Symbol,
			Access:   usage.Access,
			AccessAt: CallSiteCompact{File: usage.AccessAt.File, Line: usage.AccessAt.Line},
		})
	}
	return result
}
