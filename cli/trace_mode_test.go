package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestIndexMatchesMode(t *testing.T) {
	tests := []struct {
		name      string
		indexMode string
		want      trace.Mode
		match     bool
	}{
		{"same mode", "precise", trace.ModePrecise, true},
		{"different mode", "fast", trace.ModePrecise, false},
		{"legacy index has no recorded mode", "", trace.ModePrecise, true},
		{"auto index queried as auto", "auto", trace.ModeAuto, true},
		{"auto index queried as fast", "auto", trace.ModeFast, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := indexMatchesMode(tt.indexMode, tt.want); got != tt.match {
				t.Fatalf("indexMatchesMode(%q, %q) = %v, want %v", tt.indexMode, tt.want, got, tt.match)
			}
		})
	}
}

func TestGOBSymbolStore_ExtractorModeRoundTrips(t *testing.T) {
	ctx := context.Background()
	indexPath := filepath.Join(t.TempDir(), "symbols.gob")

	writer := trace.NewGOBSymbolStore(indexPath)
	writer.SetExtractorMode("precise")
	if err := writer.SaveFile(ctx, "main.go", []trace.Symbol{
		{Name: "Login", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	if err := writer.Persist(ctx); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	reader := trace.NewGOBSymbolStore(indexPath)
	if err := reader.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reader.ExtractorMode(); got != "precise" {
		t.Fatalf("ExtractorMode() = %q, want %q", got, "precise")
	}
}

// traceModeTestCmd builds a command carrying the same --mode flag the real
// trace subcommands register, so Flags().Changed("mode") behaves as it does
// in production.
func traceModeTestCmd(t *testing.T, mode string) *cobra.Command {
	t.Helper()
	prevMode, prevResolved := traceMode, traceResolvedMode
	t.Cleanup(func() { traceMode, traceResolvedMode = prevMode, prevResolved })
	traceResolvedMode = ""

	cmd := &cobra.Command{Use: "callers"}
	cmd.Flags().StringVarP(&traceMode, "mode", "m", "auto", "")
	if mode != "" {
		if err := cmd.Flags().Set("mode", mode); err != nil {
			t.Fatalf("failed to set --mode: %v", err)
		}
	}
	return cmd
}

// traceModeTestProject writes a project whose config traces .go files and
// whose only source file defines helperMethod and calls it from run.
func traceModeTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(config.GetConfigDir(root), 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	cfgYAML := "embedder:\n  provider: ollama\nstore:\n  backend: gob\ntrace:\n  enabled: true\n  mode: fast\n  enabled_languages:\n    - .go\n"
	if err := os.WriteFile(config.GetConfigPath(root), []byte(cfgYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	src := "package main\n\nfunc helperMethod() {}\n\nfunc run() {\n\thelperMethod()\n}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(src), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	return root
}

// seedTraceIndex persists an index that claims indexMode and contains only a
// sentinel symbol, so any symbol the assertions find must have come from a
// fresh extraction rather than from the index.
func seedTraceIndex(t *testing.T, root, indexMode string) *trace.GOBSymbolStore {
	t.Helper()
	ctx := context.Background()
	ss := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(root))
	if err := ss.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	ss.SetExtractorMode(indexMode)
	if err := ss.SaveFile(ctx, "main.go", []trace.Symbol{
		{Name: "stale_sentinel", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbols: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	return ss
}

// TestApplyTraceModeSingle_ReextractsOnExplicitModeMismatch is the
// maintainer's repro from PR #260: an index built in "fast" mode, queried
// with --mode precise, must not answer out of the fast index.
func TestApplyTraceModeSingle_ReextractsOnExplicitModeMismatch(t *testing.T) {
	ctx := context.Background()
	root := traceModeTestProject(t)
	indexed := seedTraceIndex(t, root, "fast")

	cmd := traceModeTestCmd(t, "precise")
	store, cleanup, err := applyTraceModeSingle(ctx, cmd, root, indexed)
	if err != nil {
		t.Fatalf("applyTraceModeSingle: %v", err)
	}
	defer cleanup()

	if store == indexed {
		t.Fatal("expected a re-extracted store, got the persisted index")
	}
	if traceResolvedMode != "precise" {
		t.Fatalf("traceResolvedMode = %q, want %q", traceResolvedMode, "precise")
	}

	sentinel, err := store.LookupSymbol(ctx, "stale_sentinel")
	if err != nil {
		t.Fatalf("LookupSymbol: %v", err)
	}
	if len(sentinel) != 0 {
		t.Fatalf("re-extracted store still carries the seeded sentinel: %+v", sentinel)
	}

	got, err := store.LookupSymbol(ctx, "helperMethod")
	if err != nil {
		t.Fatalf("LookupSymbol: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("re-extraction produced no symbol for helperMethod")
	}
}

// TestApplyTraceModeSingle_UsesIndexWhenModeMatches guards the fast path:
// asking for the mode the index was already built with must not re-extract.
func TestApplyTraceModeSingle_UsesIndexWhenModeMatches(t *testing.T) {
	ctx := context.Background()
	root := traceModeTestProject(t)
	indexed := seedTraceIndex(t, root, "fast")

	cmd := traceModeTestCmd(t, "fast")
	store, cleanup, err := applyTraceModeSingle(ctx, cmd, root, indexed)
	if err != nil {
		t.Fatalf("applyTraceModeSingle: %v", err)
	}
	defer cleanup()

	if store != indexed {
		t.Fatal("expected the persisted index, got a re-extracted store")
	}
	if traceResolvedMode != "fast" {
		t.Fatalf("traceResolvedMode = %q, want %q", traceResolvedMode, "fast")
	}
}

// TestApplyTraceModeSingle_NoFlagNeverReextracts is the default path: no
// --mode means the index answers, and the reported mode is the index's own.
func TestApplyTraceModeSingle_NoFlagNeverReextracts(t *testing.T) {
	ctx := context.Background()
	root := traceModeTestProject(t)
	indexed := seedTraceIndex(t, root, "precise")

	cmd := traceModeTestCmd(t, "")
	store, cleanup, err := applyTraceModeSingle(ctx, cmd, root, indexed)
	if err != nil {
		t.Fatalf("applyTraceModeSingle: %v", err)
	}
	defer cleanup()

	if store != indexed {
		t.Fatal("expected the persisted index, got a re-extracted store")
	}
	if traceResolvedMode != "precise" {
		t.Fatalf("traceResolvedMode = %q, want the index's own mode %q", traceResolvedMode, "precise")
	}
}

func TestResolvedTraceMode_FallsBackToFlag(t *testing.T) {
	prevMode, prevResolved := traceMode, traceResolvedMode
	defer func() { traceMode, traceResolvedMode = prevMode, prevResolved }()

	traceMode, traceResolvedMode = "precise", ""
	if got := resolvedTraceMode(); got != "precise" {
		t.Fatalf("resolvedTraceMode() = %q, want %q", got, "precise")
	}

	traceResolvedMode = "fast"
	if got := resolvedTraceMode(); got != "fast" {
		t.Fatalf("resolvedTraceMode() = %q, want %q", got, "fast")
	}
}
