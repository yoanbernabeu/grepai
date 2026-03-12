package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestRunRefs_should_return_readers_and_writers_from_index(t *testing.T) {
	projectRoot := t.TempDir()
	if err := config.DefaultConfig().Save(projectRoot); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to chdir to project root: %v", err)
	}

	ctx := context.Background()
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	refs := []trace.Reference{
		{
			SymbolName: "uid",
			Kind:       trace.RefKindRead,
			File:       "src/views/Home.vue",
			Line:       21,
			Context:    "return Boolean(this.store.uid)",
			CallerName: "loggedIn",
			CallerFile: "src/views/Home.vue",
			CallerLine: 20,
		},
		{
			SymbolName: "uid",
			Kind:       trace.RefKindWrite,
			File:       "src/store/index.ts",
			Line:       100,
			Context:    "this.uid = authUser.uid",
			CallerName: "user_details",
			CallerFile: "src/store/index.ts",
			CallerLine: 99,
		},
	}
	symbols := []trace.Symbol{
		{Name: "loggedIn", Kind: trace.KindMethod, File: "src/views/Home.vue", Line: 20, Language: "typescript"},
		{Name: "user_details", Kind: trace.KindMethod, File: "src/store/index.ts", Line: 99, Language: "typescript"},
	}
	if err := symbolStore.SaveFile(ctx, filepath.ToSlash("src/mix.ts"), symbols, refs); err != nil {
		t.Fatalf("failed to save symbols/refs: %v", err)
	}
	if err := symbolStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist symbol store: %v", err)
	}

	origWorkspace := refsWorkspace
	origProject := refsProject
	defer func() {
		refsWorkspace = origWorkspace
		refsProject = origProject
	}()
	refsWorkspace = ""
	refsProject = ""

	readers, err := runRefs("uid", true)
	if err != nil {
		t.Fatalf("runRefs readers failed: %v", err)
	}
	if readers.Query != "uid" || readers.Kind != "property" {
		t.Fatalf("unexpected readers header: %+v", readers)
	}
	if len(readers.Readers) != 1 {
		t.Fatalf("expected 1 reader, got %d", len(readers.Readers))
	}
	if readers.Readers[0].Symbol.Name != "loggedIn" {
		t.Fatalf("expected reader symbol loggedIn, got %q", readers.Readers[0].Symbol.Name)
	}

	writers, err := runRefs("uid", false)
	if err != nil {
		t.Fatalf("runRefs writers failed: %v", err)
	}
	if len(writers.Writers) != 1 {
		t.Fatalf("expected 1 writer, got %d", len(writers.Writers))
	}
	if writers.Writers[0].Symbol.Name != "user_details" {
		t.Fatalf("expected writer symbol user_details, got %q", writers.Writers[0].Symbol.Name)
	}
}

func TestOutputRefsGraphResult_should_output_valid_json(t *testing.T) {
	oldJSON := refsJSON
	oldTOON := refsTOON
	refsJSON = true
	refsTOON = false
	defer func() {
		refsJSON = oldJSON
		refsTOON = oldTOON
	}()

	graph := refsGraphResult{
		Query: "uid",
		Kind:  "property",
		Mode:  "fast",
		Readers: []refsUsage{
			{
				Symbol: trace.Symbol{Name: "loggedIn", File: "src/Home.vue", Line: 10},
				Access: trace.RefKindRead,
				AccessAt: trace.CallSite{
					File: "src/Home.vue", Line: 12, Context: "store.uid",
				},
			},
		},
		Writers: []refsUsage{
			{
				Symbol: trace.Symbol{Name: "user_details", File: "src/store.ts", Line: 20},
				Access: trace.RefKindWrite,
				AccessAt: trace.CallSite{
					File: "src/store.ts", Line: 22, Context: "this.uid = x",
				},
			},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := outputRefsGraphResult(graph)
	_ = w.Close()
	os.Stdout = oldStdout
	if err != nil {
		t.Fatalf("outputRefsGraphResult failed: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	var decoded refsGraphResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if decoded.Query != "uid" {
		t.Fatalf("decoded query = %q, want uid", decoded.Query)
	}
	if len(decoded.Readers) != 1 || len(decoded.Writers) != 1 {
		t.Fatalf("expected one reader and one writer, got %d/%d", len(decoded.Readers), len(decoded.Writers))
	}
}
