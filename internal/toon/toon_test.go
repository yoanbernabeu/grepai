package toon

import (
	"strings"
	"testing"
)

type row struct {
	FilePath   string `json:"file_path"`
	SymbolName string `json:"symbol_name,omitempty"`
	Hidden     string `json:"-"`
}

func TestEncode_UsesJSONFieldNames(t *testing.T) {
	out, err := Encode([]row{{FilePath: "a.go", SymbolName: "Run", Hidden: "x"}, {FilePath: "b.go", SymbolName: "Stop"}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(out, "omitempty") {
		t.Errorf("output contains a raw tag option:\n%s", out)
	}
	if strings.Contains(out, "Hidden") {
		t.Errorf("output contains a json:\"-\" field:\n%s", out)
	}
	for _, want := range []string{"file_path", "symbol_name", "a.go", "Stop"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestEncode_OmitsEmptyFields(t *testing.T) {
	out, err := Encode(row{FilePath: "a.go"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(out, "symbol_name") {
		t.Errorf("empty omitempty field was encoded:\n%s", out)
	}
}
