package trace

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// langSymbolCase describes a per-language extraction test: a fixture file
// under trace/testdata/ and a sorted list of "kind:name" tuples that the
// extractor must surface (extra symbols are allowed). The Kind in the
// expectation matches NamedQuery.Kind verbatim (free-form per the PR 1
// design — no fixed taxonomy).
type langSymbolCase struct {
	name        string
	fixturePath string // relative to trace/testdata/
	want        []string
}

func TestExtractor_LanguageFixtures(t *testing.T) {
	cases := []langSymbolCase{
		{
			name:        "ruby",
			fixturePath: "ruby.rb",
			want: []string{
				"class:Hello",
				"class:Standalone",
				"method:say",
				"module:Greeter",
				"singleton_method:banner",
			},
		},
		{
			name:        "rust",
			fixturePath: "rust.rs",
			want: []string{
				"constant:MAX",
				"enum:Color",
				"function:add",
				"function:new",
				"static:GLOBAL",
				"struct:Point",
				"trait:Greet",
				"type:Pair",
			},
		},
		{
			name:        "java",
			fixturePath: "java.java",
			want: []string{
				"class:Greeter",
				"constructor:Greeter",
				"enum:Color",
				"interface:Greet",
				"method:compute",
				"method:hello",
				"record:Point",
			},
		},
		{
			name:        "scala",
			fixturePath: "scala.scala",
			want: []string{
				"class:Hello",
				"class:Point",
				"enum:Color",
				"function:hello",
				"function:say",
				"object:Greeter",
				"trait:Greet",
				"val:MAX",
			},
		},
		{
			name:        "c",
			fixturePath: "cmod.c",
			want: []string{
				"enum:Color",
				"function:add",
				"macro:ADD",
				"macro:MAX",
				"struct:Point",
			},
		},
		{
			name:        "cpp",
			fixturePath: "cppmod.cpp",
			want: []string{
				"class:Greeter",
				"enum:Color",
				"namespace:foo",
				"struct:Point",
			},
		},
		{
			name:        "bash",
			fixturePath: "bash.sh",
			want: []string{
				"function:greet",
				"function:helper",
				"variable:GREETING",
				"variable:VERSION",
			},
		},
		{
			name:        "lua",
			fixturePath: "lua.lua",
			want: []string{
				"function:M.method",
				"function:private_fn",
				"function:public_fn",
				"variable:M",
			},
		},
		{
			name:        "kotlin",
			fixturePath: "kotlin.kt",
			want: []string{
				"class:Color",
				"class:Greet",
				"class:Greeter",
				"function:hello",
				"function:standalone",
				"function:work",
				"object:Singleton",
			},
		},
		{
			name:        "swift",
			fixturePath: "swift.swift",
			want: []string{
				"class:Greeter",
				"class:Point",
				"function:hello",
				"function:standalone",
				"protocol:Greet",
			},
		},
		{
			name:        "sql",
			fixturePath: "sample.sql",
			want: []string{
				"function:add",
				"index:users_name_idx",
				"table:users",
				"view:active_users",
			},
		},
		{
			name:        "protobuf",
			fixturePath: "sample.proto",
			want: []string{
				"enum:Color",
				"message:Greeter",
				"rpc:Say",
				"service:Hello",
			},
		},
		{
			name:        "hcl",
			fixturePath: "sample.hcl",
			want: []string{
				"block:locals",
				"block:resource",
				"block:variable",
			},
		},
		{
			name:        "elm",
			fixturePath: "sample.elm",
			want: []string{
				"function:greet",
				"type:Color",
				"type_alias:Point",
			},
		},
		{
			name:        "toml",
			fixturePath: "sample.toml",
			want: []string{
				"key:name",
				"key:version",
				"table:deps",
				"table:deps.optional",
			},
		},
		{
			name:        "elisp",
			fixturePath: "sample.el",
			// defalias is intentionally not asserted — the grammar parses
			// its target as a `quote` node, not a bare `symbol`, and our
			// query set doesn't yet cover that shape. See queries_elisp.go.
			want: []string{
				"cl-defmethod:foo",
				"cl-defun:fancy-fn",
				"defconst:max-retries",
				"defcustom:user-name",
				"defface:my-face",
				"define-mode:my-major",
				"define-mode:my-mode",
				"defmacro:when-not",
				"defun:greet",
				"defvar:my-counter",
			},
		},
	}

	ts, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", tc.fixturePath))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			symbols, err := ts.ExtractSymbols(context.Background(), tc.fixturePath, string(content))
			if err != nil {
				t.Fatalf("ExtractSymbols: %v", err)
			}

			got := make([]string, 0, len(symbols))
			for _, s := range symbols {
				got = append(got, string(s.Kind)+":"+s.Name)
			}
			sort.Strings(got)

			missing := setDifference(tc.want, got)
			if len(missing) > 0 {
				t.Errorf("missing expected symbols: %v\n  got: %v", missing, got)
			}
		})
	}
}

// setDifference returns elements in want that are absent from got. Both
// inputs must be sorted.
func setDifference(want, got []string) []string {
	gotIdx := make(map[string]struct{}, len(got))
	for _, g := range got {
		gotIdx[g] = struct{}{}
	}
	var missing []string
	for _, w := range want {
		if _, ok := gotIdx[w]; !ok {
			missing = append(missing, w)
		}
	}
	return missing
}
