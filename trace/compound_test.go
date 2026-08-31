package trace

import (
	"context"
	"strings"
	"testing"
)

// assertStrictlySorted fails the test if got is not strictly ascending.
// label is used in the failure message to disambiguate call sites.
func assertStrictlySorted(t *testing.T, label string, got []string) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("%s: not strictly sorted at index %d (%q >= %q)", label, i, got[i-1], got[i])
			return
		}
	}
}

// TestParseMode covers the user-input normalization path. ParseMode is
// pure: it never logs, and returns a recognized-flag so the caller can
// surface an unknown-input warning where it belongs (typically the CLI).
func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		in       string
		want     Mode
		wantOK   bool
		wantName string
	}{
		{"", ModeAuto, true, "empty"},
		{"auto", ModeAuto, true, "lower"},
		{"AUTO", ModeAuto, true, "upper"},
		{"  Auto  ", ModeAuto, true, "whitespace"},
		{"fast", ModeFast, true, "fast"},
		{"precise", ModePrecise, true, "precise"},
		{"garbage", ModeAuto, false, "unknown falls back"},
	} {
		got, ok := ParseMode(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("ParseMode(%q) [%s]: got (%q, %v), want (%q, %v)",
				tc.in, tc.wantName, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestCompound_AutoDispatch confirms that auto mode routes tree-sitter-backed
// extensions through the tree-sitter extractor and everything else through
// the regex extractor.
func TestCompound_AutoDispatch(t *testing.T) {
	e := NewCompoundExtractor(ModeAuto)

	// A .go file has a tree-sitter grammar.
	ex, err := e.route("main.go")
	if err != nil {
		t.Fatalf("route(.go) error: %v", err)
	}
	if _, ok := ex.(*TreeSitterExtractor); !ok {
		t.Errorf(".go: expected TreeSitterExtractor, got %T", ex)
	}

	// A .zig file has no tree-sitter grammar (still regex-only after PR 2).
	ex, err = e.route("script.zig")
	if err != nil {
		t.Fatalf("route(.zig) error: %v", err)
	}
	if _, ok := ex.(*RegexExtractor); !ok {
		t.Errorf(".zig: expected RegexExtractor, got %T", ex)
	}

	// An unknown extension still routes to regex (regex returns nil for it
	// downstream — that's its own concern).
	ex, err = e.route("data.xyz")
	if err != nil {
		t.Fatalf("route(.xyz) error: %v", err)
	}
	if _, ok := ex.(*RegexExtractor); !ok {
		t.Errorf(".xyz: expected RegexExtractor, got %T", ex)
	}
}

// TestCompound_FastDispatch forces every file through the regex extractor.
func TestCompound_FastDispatch(t *testing.T) {
	e := NewCompoundExtractor(ModeFast)
	for _, path := range []string{"main.go", "app.py", "script.zig", "data.xyz"} {
		ex, err := e.route(path)
		if err != nil {
			t.Fatalf("route(%s) error: %v", path, err)
		}
		if _, ok := ex.(*RegexExtractor); !ok {
			t.Errorf("%s under ModeFast: expected RegexExtractor, got %T", path, ex)
		}
	}
}

// TestCompound_PreciseDispatch routes everything through tree-sitter and
// errors out for files whose extension has no grammar.
func TestCompound_PreciseDispatch(t *testing.T) {
	e := NewCompoundExtractor(ModePrecise)

	// Tree-sitter-backed: routes successfully.
	if _, err := e.route("main.go"); err != nil {
		t.Errorf("route(.go) under ModePrecise: unexpected error %v", err)
	}

	// No grammar: must produce a descriptive error referencing the
	// extension and file so callers can surface it to the user.
	_, err := e.route("script.zig")
	if err == nil {
		t.Fatalf("route(.zig) under ModePrecise: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "precise") || !strings.Contains(err.Error(), ".zig") {
		t.Errorf("error message should reference precise mode and the extension; got %q", err.Error())
	}
}

// TestCompound_FastMode_NoTreeSitterAllocation confirms that ModeFast
// never materializes the tree-sitter extractor — the laziness contract.
func TestCompound_FastMode_NoTreeSitterAllocation(t *testing.T) {
	e := NewCompoundExtractor(ModeFast)
	if e.ts != nil {
		t.Error("ModeFast: tree-sitter extractor allocated at construction; expected lazy")
	}
	// Route several supported and unsupported extensions; all should go to
	// regex, none should trigger TS lazy init.
	for _, path := range []string{"main.go", "app.py", "script.zig", "data.xyz"} {
		if _, err := e.route(path); err != nil {
			t.Fatalf("route(%s): %v", path, err)
		}
	}
	if e.ts != nil {
		t.Error("ModeFast: tree-sitter extractor allocated after routing; expected never")
	}
}

// TestCompound_AutoMode_LazyTreeSitter confirms that ModeAuto defers
// tree-sitter construction until the first supported file lands.
func TestCompound_AutoMode_LazyTreeSitter(t *testing.T) {
	e := NewCompoundExtractor(ModeAuto)
	if e.ts != nil {
		t.Error("ModeAuto: tree-sitter extractor allocated at construction; expected lazy")
	}
	// Routing an unsupported extension still doesn't allocate the TS path.
	if _, err := e.route("script.zig"); err != nil {
		t.Fatalf("route(.zig): %v", err)
	}
	if e.ts != nil {
		t.Error("ModeAuto: routing an unsupported extension allocated tree-sitter unnecessarily")
	}
	// First supported file: TS is now built.
	if _, err := e.route("main.go"); err != nil {
		t.Fatalf("route(.go): %v", err)
	}
	if e.ts == nil {
		t.Error("ModeAuto: routing a supported extension did not allocate tree-sitter")
	}
}

// TestHasTreeSitterGrammar_FromRegistry confirms HasTreeSitterGrammar
// reflects the registry (treeSitterLanguages) without surprises.
func TestHasTreeSitterGrammar_FromRegistry(t *testing.T) {
	if !HasTreeSitterGrammar(".go") {
		t.Error(".go should be supported")
	}
	if HasTreeSitterGrammar(".zig") {
		t.Error(".zig should not be tree-sitter-backed (it's regex-only)")
	}
	// Case insensitivity.
	if !HasTreeSitterGrammar(".GO") {
		t.Error(".GO (uppercase) should match .go in the registry")
	}
}

// TestTreeSitterExtensions_SortedAndDerived confirms TreeSitterExtensions
// returns a sorted snapshot derived from the registry.
func TestTreeSitterExtensions_SortedAndDerived(t *testing.T) {
	got := TreeSitterExtensions()
	expected := 0
	for _, spec := range treeSitterLanguages {
		expected += len(spec.Extensions)
	}
	if len(got) != expected {
		t.Fatalf("TreeSitterExtensions returned %d entries; registry has %d", len(got), expected)
	}
	assertStrictlySorted(t, "TreeSitterExtensions", got)
	// Every returned ext exists in the registry.
	for _, ext := range got {
		if langSpecByExt(ext) == nil {
			t.Errorf("%q returned but not in registry", ext)
		}
	}
}

// TestCompound_ExtractSymbols_GoFile confirms that the end-to-end pipeline
// produces tree-sitter-quality output for a .go file under auto mode.
func TestCompound_ExtractSymbols_GoFile(t *testing.T) {
	const goSource = `package main

func Greet(name string) string {
	return "hello, " + name
}

type Counter struct {
	value int
}

func (c *Counter) Increment() {
	c.value++
}
`
	e := NewCompoundExtractor(ModeAuto)
	symbols, err := e.ExtractSymbols(context.Background(), "main.go", goSource)
	if err != nil {
		t.Fatalf("ExtractSymbols: %v", err)
	}
	// The tree-sitter extractor should find Greet (function), Counter
	// (class/struct), and Increment (method). The regex extractor would
	// also find these, so this test passes under both — but we additionally
	// confirm dispatch via TestCompound_AutoDispatch above.
	wantNames := map[string]bool{"Greet": false, "Counter": false, "Increment": false}
	for _, s := range symbols {
		if _, expected := wantNames[s.Name]; expected {
			wantNames[s.Name] = true
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("expected symbol %q in output, missing; got %d symbols total", name, len(symbols))
		}
	}
}

// TestCompound_Mode returns the configured mode for downstream display.
func TestCompound_Mode(t *testing.T) {
	for _, m := range []Mode{ModeAuto, ModeFast, ModePrecise} {
		e := NewCompoundExtractor(m)
		if got := e.Mode(); got != string(m) {
			t.Errorf("Mode(): got %q, want %q", got, m)
		}
	}
}

// TestCompound_SupportedLanguages returns the union of both extractors'
// extensions, deduplicated and sorted.
func TestCompound_SupportedLanguages(t *testing.T) {
	e := NewCompoundExtractor(ModeAuto)
	langs := e.SupportedLanguages()
	if len(langs) == 0 {
		t.Fatal("SupportedLanguages: expected non-empty list")
	}
	// Both extractor's exclusive extensions should appear in the union.
	// .go is tree-sitter-backed; .zig and .rs are regex-only languages
	// known to live in patterns.go.
	want := []string{".go", ".zig", ".rs"}
	for _, w := range want {
		found := false
		for _, l := range langs {
			if l == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SupportedLanguages: %s missing from result", w)
		}
	}
	assertStrictlySorted(t, "SupportedLanguages", langs)
}
