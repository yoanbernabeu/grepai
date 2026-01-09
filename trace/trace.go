// Package trace provides symbol extraction and call graph analysis for code navigation.
package trace

import (
	"context"
	"time"
)

// SymbolKind represents the type of symbol.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindClass     SymbolKind = "class"
	KindInterface SymbolKind = "interface"
	KindType      SymbolKind = "type"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
)

// Symbol represents a symbol definition in the codebase.
type Symbol struct {
	Name      string     `json:"name"`
	Kind      SymbolKind `json:"kind"`
	File      string     `json:"file"`
	Line      int        `json:"line"`
	EndLine   int        `json:"end_line,omitempty"`
	Signature string     `json:"signature,omitempty"`
	Receiver  string     `json:"receiver,omitempty"`
	Package   string     `json:"package,omitempty"`
	Exported  bool       `json:"exported,omitempty"`
	Language  string     `json:"language"`
}

// Reference represents a usage/call of a symbol.
type Reference struct {
	SymbolName string `json:"symbol_name"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column,omitempty"`
	Context    string `json:"context"`
	CallerName string `json:"caller_name"`
	CallerFile string `json:"caller_file"`
	CallerLine int    `json:"caller_line"`
}

// CallEdge represents a caller -> callee relationship.
type CallEdge struct {
	Caller   string `json:"caller"`
	Callee   string `json:"callee"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	CallType string `json:"call_type,omitempty"`
}

// SymbolIndex is the main index structure for symbols and references.
type SymbolIndex struct {
	Symbols    map[string][]Symbol    `json:"symbols"`
	References map[string][]Reference `json:"references"`
	CallGraph  []CallEdge             `json:"call_graph"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Version    int                    `json:"version"`
}

// TraceResult represents the output of a trace query.
type TraceResult struct {
	Query   string       `json:"query"`
	Mode    string       `json:"mode"`
	Symbol  *Symbol      `json:"symbol,omitempty"`
	Callers []CallerInfo `json:"callers,omitempty"`
	Callees []CalleeInfo `json:"callees,omitempty"`
	Graph   *CallGraph   `json:"graph,omitempty"`
}

// CallerInfo represents a function that calls the target.
type CallerInfo struct {
	Symbol   Symbol   `json:"symbol"`
	CallSite CallSite `json:"call_site"`
}

// CalleeInfo represents a function called by the target.
type CalleeInfo struct {
	Symbol   Symbol   `json:"symbol"`
	CallSite CallSite `json:"call_site"`
}

// CallSite represents the location of a function call.
type CallSite struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Context string `json:"context"`
}

// CallGraph represents a multi-level call graph.
type CallGraph struct {
	Root  string            `json:"root"`
	Nodes map[string]Symbol `json:"nodes"`
	Edges []CallEdge        `json:"edges"`
	Depth int               `json:"depth"`
}

// SymbolStats contains index statistics.
type SymbolStats struct {
	TotalSymbols    int       `json:"total_symbols"`
	TotalReferences int       `json:"total_references"`
	TotalFiles      int       `json:"total_files"`
	IndexSize       int64     `json:"index_size"`
	LastUpdated     time.Time `json:"last_updated"`
}

// SymbolExtractor extracts symbols and references from source code.
type SymbolExtractor interface {
	// ExtractSymbols extracts all symbol definitions from a file.
	ExtractSymbols(ctx context.Context, filePath string, content string) ([]Symbol, error)

	// ExtractReferences extracts all symbol references from a file.
	ExtractReferences(ctx context.Context, filePath string, content string) ([]Reference, error)

	// ExtractAll extracts both symbols and references in one pass.
	ExtractAll(ctx context.Context, filePath string, content string) ([]Symbol, []Reference, error)

	// SupportedLanguages returns list of supported file extensions.
	SupportedLanguages() []string

	// Mode returns "fast" or "precise".
	Mode() string
}

// SymbolStore persists and queries the symbol index.
type SymbolStore interface {
	// SaveFile persists symbols and references for a file.
	SaveFile(ctx context.Context, filePath string, symbols []Symbol, refs []Reference) error

	// DeleteFile removes all symbols and references for a file.
	DeleteFile(ctx context.Context, filePath string) error

	// LookupSymbol finds symbol definitions by name.
	LookupSymbol(ctx context.Context, name string) ([]Symbol, error)

	// LookupCallers finds all references/callers of a symbol.
	LookupCallers(ctx context.Context, symbolName string) ([]Reference, error)

	// LookupCallees finds all symbols called by a function.
	LookupCallees(ctx context.Context, symbolName string, file string) ([]Reference, error)

	// GetCallGraph builds a call graph from a starting symbol.
	GetCallGraph(ctx context.Context, symbolName string, depth int) (*CallGraph, error)

	// Load reads the index from storage.
	Load(ctx context.Context) error

	// Persist writes the index to storage.
	Persist(ctx context.Context) error

	// Close shuts down the store.
	Close() error

	// GetStats returns statistics about the symbol index.
	GetStats(ctx context.Context) (*SymbolStats, error)
}
