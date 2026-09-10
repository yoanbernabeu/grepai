//go:build e2e

package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

type liveCleanupExpectation struct {
	presentFiles      []string
	absentFiles       []string
	presentSymbols    map[string][]string
	absentSymbolFiles []string
	presentTraces     map[string]string
	absentTraces      []string
}

type liveCleanupState struct {
	documents   []string
	chunks      map[string]int
	definitions map[string][]trace.Symbol
	references  map[string][]trace.Reference
	traces      map[string]trace.TraceResult
	fileSymbols map[string][]trace.Symbol
}

func (h *liveCleanupHarness) readState(want liveCleanupExpectation) (liveCleanupState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	vector := store.NewGOBStore(config.GetIndexPath(h.root))
	if err := vector.Load(ctx); err != nil {
		return liveCleanupState{}, err
	}
	documents, err := vector.ListDocuments(ctx)
	if err != nil {
		return liveCleanupState{}, err
	}
	chunks, err := vector.GetAllChunks(ctx)
	if err != nil {
		return liveCleanupState{}, err
	}
	state := liveCleanupState{
		documents: documents, chunks: make(map[string]int),
		definitions: make(map[string][]trace.Symbol), references: make(map[string][]trace.Reference),
		traces: make(map[string]trace.TraceResult), fileSymbols: make(map[string][]trace.Symbol),
	}
	for _, chunk := range chunks {
		state.chunks[filepath.ToSlash(chunk.FilePath)]++
	}
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(h.root))
	if err := symbolStore.Load(ctx); err != nil {
		return liveCleanupState{}, err
	}
	for path := range want.presentSymbols {
		symbols, err := symbolStore.GetSymbolsForFile(ctx, path)
		if err != nil {
			return liveCleanupState{}, err
		}
		state.fileSymbols[path] = symbols
	}
	for _, path := range want.absentSymbolFiles {
		symbols, err := symbolStore.GetSymbolsForFile(ctx, path)
		if err != nil {
			return liveCleanupState{}, err
		}
		state.fileSymbols[path] = symbols
	}
	symbols := make([]string, 0, len(want.presentTraces)+len(want.absentTraces))
	for symbol := range want.presentTraces {
		symbols = append(symbols, symbol)
	}
	symbols = append(symbols, want.absentTraces...)
	for _, symbol := range symbols {
		definitions, err := symbolStore.LookupSymbol(ctx, symbol)
		if err != nil {
			return liveCleanupState{}, err
		}
		references, err := symbolStore.LookupCallers(ctx, symbol)
		if err != nil {
			return liveCleanupState{}, err
		}
		state.definitions[symbol] = definitions
		state.references[symbol] = references
		output, err := h.run("trace", "callers", symbol, "--json", "--compact")
		if err != nil {
			return liveCleanupState{}, fmt.Errorf("trace %s: %w (%s)", symbol, err, output)
		}
		var result trace.TraceResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return liveCleanupState{}, err
		}
		state.traces[symbol] = result
	}
	return state, nil
}

func liveCleanupMatches(state liveCleanupState, want liveCleanupExpectation) (bool, string) {
	for _, path := range want.presentFiles {
		if !containsPath(state.documents, path) || state.chunks[path] == 0 {
			return false, fmt.Sprintf("file %q missing: documents=%v chunks=%v", path, state.documents, state.chunks)
		}
	}
	for _, path := range want.absentFiles {
		if containsPath(state.documents, path) || state.chunks[path] != 0 {
			return false, fmt.Sprintf("file %q remains: documents=%v chunks=%v", path, state.documents, state.chunks)
		}
	}
	for path, names := range want.presentSymbols {
		actual := state.fileSymbols[path]
		for _, name := range names {
			found := false
			for _, symbol := range actual {
				if symbol.Name == name && filepath.ToSlash(symbol.File) == path {
					found = true
					break
				}
			}
			if !found {
				return false, fmt.Sprintf("symbol %q missing from %q: %#v", name, path, actual)
			}
		}
	}
	for _, path := range want.absentSymbolFiles {
		if len(state.fileSymbols[path]) != 0 {
			return false, fmt.Sprintf("symbols remain for %q: %#v", path, state.fileSymbols[path])
		}
	}
	for symbol, path := range want.presentTraces {
		definitions := state.definitions[symbol]
		result := state.traces[symbol]
		if len(definitions) == 0 || filepath.ToSlash(definitions[0].File) != path || len(state.references[symbol]) == 0 || result.Symbol == nil || len(result.Callers) == 0 {
			return false, fmt.Sprintf("trace %s not persistent: definitions=%#v refs=%#v result=%#v", symbol, definitions, state.references[symbol], result)
		}
	}
	for _, symbol := range want.absentTraces {
		result := state.traces[symbol]
		if len(state.definitions[symbol]) != 0 || len(state.references[symbol]) != 0 || result.Symbol != nil || len(result.Callers) != 0 {
			return false, fmt.Sprintf("trace %s remains", symbol)
		}
	}
	return true, ""
}

func (h *liveCleanupHarness) awaitState(watch *liveWatchProcess, pid int, want liveCleanupExpectation) {
	deadline := time.NewTimer(liveCleanupTimeout)
	defer deadline.Stop()
	for {
		watch.assertRunning(pid)
		state, err := h.readState(want)
		if err == nil {
			if matched, _ := liveCleanupMatches(state, want); matched {
				watch.assertRunning(pid)
				return
			}
		}
		select {
		case <-time.After(time.Second):
		case <-deadline.C:
			mismatch := ""
			if err != nil {
				mismatch = err.Error()
			} else {
				_, mismatch = liveCleanupMatches(state, want)
			}
			h.t.Fatalf("persistent state did not converge while PID %d was live: %s\n%s", pid, mismatch, watch.output.String())
		}
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if filepath.ToSlash(path) == want {
			return true
		}
	}
	return false
}
