package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
)

// --mode dispatch for the trace commands.
//
// `grepai trace` answers from the symbol index that `grepai watch` built, and
// that index holds whatever the extractor configured at watch time produced —
// an index built in "fast" mode contains no tree-sitter symbols at all, so
// there is nothing for a precise query to find. Honoring --mode therefore
// means re-extracting the project with the requested extractor.
//
// Re-extraction is O(repo), so it only happens when the user explicitly
// passes --mode and the requested mode differs from the one the index was
// built with (recorded by GOBSymbolStore.SetExtractorMode). Everything else
// reads the index as-is and pays nothing.

// traceResolvedMode is the mode that actually produced the symbols being
// reported. It is what TraceResult.Mode carries, so the JSON output names
// the extractor the answer came from rather than echoing the flag back.
var traceResolvedMode string

// resolvedTraceMode is what TraceResult.Mode should carry. It falls back to
// the raw flag on paths that bail out before a store is resolved.
func resolvedTraceMode() string {
	if traceResolvedMode != "" {
		return traceResolvedMode
	}
	return traceMode
}

// resolveTraceMode parses --mode and reports whether the user set it
// explicitly. An unrecognized value warns once and falls back to auto,
// matching the watcher's behavior.
func resolveTraceMode(cmd *cobra.Command) (trace.Mode, bool) {
	mode, ok := trace.ParseMode(traceMode)
	if !ok {
		fmt.Fprintf(os.Stderr, "Warning: trace mode %q is not recognized; using %q (valid values: auto, fast, precise)\n", traceMode, mode)
	}
	return mode, cmd.Flags().Changed("mode")
}

// indexMatchesMode reports whether an index built in indexMode can answer a
// query for want. An index predating mode tracking reports "", which we
// treat as a match: re-extracting every legacy index on its owner's first
// trace call would be a nasty surprise, and the flag defaulting to the same
// value the watcher used makes a mismatch unlikely.
func indexMatchesMode(indexMode string, want trace.Mode) bool {
	return indexMode == "" || indexMode == string(want)
}

// applyTraceModeSingle returns the store the single-project trace commands
// should query, honoring --mode. The returned cleanup is always non-nil.
func applyTraceModeSingle(ctx context.Context, cmd *cobra.Command, projectRoot string, indexed *trace.GOBSymbolStore) (*trace.GOBSymbolStore, func(), error) {
	mode, explicit := resolveTraceMode(cmd)
	indexMode := indexed.ExtractorMode()

	if !explicit || indexMatchesMode(indexMode, mode) {
		traceResolvedMode = effectiveModeName(indexMode, mode)
		return indexed, func() {}, nil
	}

	fmt.Fprintf(os.Stderr, "Note: index was built in %q mode; re-extracting %s in %q mode (set trace.mode and re-run `grepai watch` to make this permanent)\n", indexMode, projectRoot, mode)

	store, cleanup, err := reextractProject(ctx, projectRoot, mode)
	if err != nil {
		return nil, func() {}, err
	}
	traceResolvedMode = string(mode)
	return store, cleanup, nil
}

// applyTraceModeWorkspace is the workspace-scoped counterpart: it replaces
// any store whose index was built in a different mode with a freshly
// extracted one.
func applyTraceModeWorkspace(ctx context.Context, cmd *cobra.Command, stores []trace.SymbolStore) ([]trace.SymbolStore, func(), error) {
	mode, explicit := resolveTraceMode(cmd)
	traceResolvedMode = string(mode)
	if !explicit {
		return stores, func() {}, nil
	}

	projects, err := trace.WorkspaceProjects(traceWorkspace, traceProject)
	if err != nil {
		return nil, func() {}, err
	}
	if len(projects) != len(stores) {
		// Should not happen: both come from WorkspaceProjects. Bail out
		// rather than pair a store with the wrong project root.
		return nil, func() {}, fmt.Errorf("workspace %q: %d projects but %d symbol stores", traceWorkspace, len(projects), len(stores))
	}

	var cleanups []func()
	cleanup := func() {
		for _, c := range cleanups {
			c()
		}
	}

	out := make([]trace.SymbolStore, len(stores))
	for i, ss := range stores {
		gs, isGOB := ss.(*trace.GOBSymbolStore)
		if !isGOB || indexMatchesMode(gs.ExtractorMode(), mode) {
			out[i] = ss
			continue
		}
		fmt.Fprintf(os.Stderr, "Note: project %s was indexed in %q mode; re-extracting in %q mode\n", projects[i].Name, gs.ExtractorMode(), mode)
		fresh, c, err := reextractProject(ctx, projects[i].Path, mode)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		cleanups = append(cleanups, c)
		out[i] = fresh
	}
	return out, cleanup, nil
}

// effectiveModeName names the mode that produced the symbols we are about to
// report. A legacy index has no record of its own mode; the requested mode is
// the best available answer, and it is what the watcher would have used.
func effectiveModeName(indexMode string, want trace.Mode) string {
	if indexMode != "" {
		return indexMode
	}
	return string(want)
}

// reextractProject builds a throwaway symbol store for projectRoot using the
// requested mode. The store lives in a temp directory that cleanup removes;
// it is never written to the project's real index.
func reextractProject(ctx context.Context, projectRoot string, mode trace.Mode) (*trace.GOBSymbolStore, func(), error) {
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config for %s: %w", projectRoot, err)
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, cfg.Ignore, cfg.ExternalGitignore)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher).
		WithCustomExtensions(cfg.Chunking.CustomExtensions)

	files, _, err := scanner.Scan()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to scan %s: %w", projectRoot, err)
	}

	tmpDir, err := os.MkdirTemp("", "grepai-trace-mode-*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	store := trace.NewGOBSymbolStore(filepath.Join(tmpDir, "symbols.gob"))
	store.SetExtractorMode(string(mode))
	extractor := trace.NewCompoundExtractor(mode)

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Path))
		if !isTracedLanguage(ext, cfg.Trace.EnabledLanguages) {
			continue
		}
		symbols, refs, err := extractor.ExtractAll(ctx, file.Path, file.Content)
		if err != nil {
			// Matches the watcher: one unparseable file (or, in precise
			// mode, one extension with no compiled-in grammar) warns and
			// is skipped rather than sinking the whole query.
			fmt.Fprintf(os.Stderr, "Warning: failed to extract symbols from %s: %v\n", file.Path, err)
			continue
		}
		if err := store.SaveFile(ctx, file.Path, symbols, refs); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("failed to record symbols for %s: %w", file.Path, err)
		}
	}

	return store, cleanup, nil
}
