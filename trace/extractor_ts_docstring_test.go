//go:build treesitter

package trace

import (
	"context"
	"testing"
)

func TestTreeSitterExtractor_ExtractGoDocstrings(t *testing.T) {
	extractor, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor failed: %v", err)
	}

	ctx := context.Background()
	content := `package main

// Helper function
// Performs some work
func helper() {}

// User struct
type User struct {}

// NewUser creates a user
func NewUser() *User { return &User{} }

// Method on User
func (u *User) Do() {}

// Grouped type
type (
    // Config struct
    Config struct {}
)
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	expectedDocs := map[string]string{
		"helper":  "// Helper function\n// Performs some work",
		"User":    "// User struct",
		"NewUser": "// NewUser creates a user",
		"Do":      "// Method on User",
		"Config":  "// Config struct",
	}

	found := make(map[string]string)
	for _, sym := range symbols {
		found[sym.Name] = sym.Docstring
	}

	for name, expected := range expectedDocs {
		got, ok := found[name]
		if !ok {
			t.Errorf("Symbol %s not found", name)
			continue
		}

		// Normalize whitespace for easier comparison if needed, but exact match is better if logic works
		// My logic joins with \n.
		if got != expected {
			t.Errorf("Symbol %s: expected docstring %q, got %q", name, expected, got)
		}
	}
}
