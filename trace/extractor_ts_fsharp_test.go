//go:build treesitter

package trace

import (
	"context"
	"testing"
)

func TestTreeSitterExtractor_ExtractSymbols_FSharp(t *testing.T) {
	extractor, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor failed: %v", err)
	}

	ctx := context.Background()
	content := `module MyModule

open System

type Shape =
  | Circle of float
  | Rectangle of float * float

type Person = { Name: string; Age: int }

type Alias = int

type ILogger =
  abstract member Log: string -> unit
  abstract member Flush: unit -> unit

type Greeter(name: string) =
  let mutable greeting = "Hello"

  member this.SayHello() =
    sprintf "%s, %s!" greeting name

  static member Create(name) =
    Greeter(name)

  override this.ToString() =
    sprintf "Greeter(%s)" name

let add x y = x + y

let rec factorial n =
  if n <= 1 then 1
  else n * factorial (n - 1)

let private helper x = x + 1

let inline square x = x * x

module Nested =
  let nestedFunc a b = a + b
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.fs", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundFunctions := make(map[string]bool)
	foundMethods := make(map[string]bool)
	foundTypes := make(map[string]bool)
	foundClasses := make(map[string]bool)
	foundInterfaces := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindFunction:
			foundFunctions[sym.Name] = true
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		case KindClass:
			foundClasses[sym.Name] = true
		case KindInterface:
			foundInterfaces[sym.Name] = true
		}
	}

	// Functions
	for _, name := range []string{"add", "factorial", "helper", "square", "nestedFunc"} {
		if !foundFunctions[name] {
			t.Errorf("missing function: %s", name)
		}
	}

	// Types (records, DUs, abbreviations)
	for _, name := range []string{"Shape", "Person", "Alias"} {
		if !foundTypes[name] {
			t.Errorf("missing type: %s", name)
		}
	}

	// Classes (types with constructors) and modules
	for _, name := range []string{"Greeter", "Nested"} {
		if !foundClasses[name] {
			t.Errorf("missing class/module: %s", name)
		}
	}

	// Interfaces
	if !foundInterfaces["ILogger"] {
		t.Error("missing interface: ILogger")
	}

	// Methods
	for _, name := range []string{"SayHello", "Create", "ToString"} {
		if !foundMethods[name] {
			t.Errorf("missing method: %s", name)
		}
	}
}

func TestTreeSitterExtractor_ExtractReferences_FSharp(t *testing.T) {
	extractor, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor failed: %v", err)
	}

	ctx := context.Background()
	content := `let processData input =
  let result = helper input
  let formatted = String.Format("{0}", result)
  Logger.log "done"
  result

let main () =
  let data = getData()
  processData data
`

	refs, err := extractor.ExtractReferences(ctx, "test.fs", content)
	if err != nil {
		t.Fatalf("ExtractReferences failed: %v", err)
	}

	type callerCallee struct{ caller, callee string }
	found := make(map[callerCallee]bool)
	for _, ref := range refs {
		found[callerCallee{ref.CallerName, ref.SymbolName}] = true
	}

	expected := []callerCallee{
		{"processData", "helper"},
		{"processData", "Format"},
		{"processData", "log"},
		{"main", "getData"},
		{"main", "processData"},
	}

	for _, pair := range expected {
		if !found[pair] {
			t.Errorf("missing reference: %s -> %s", pair.caller, pair.callee)
		}
	}
}
