package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipDirScopesNegationsToPossibleDescendants(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".gitignore", "node_modules/\ncache/\n")
	writeIgnoreTestFile(t, root, ".grepaiignore", "vendor/\n!vendor/keep.go\n")
	m, err := NewIgnoreMatcher(root, []string{".git", ".grepai"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkipDir("node_modules") || !m.ShouldSkipDir("cache") {
		t.Fatal("unrelated ignored directories were kept traversable")
	}
	if m.ShouldSkipDir("vendor") {
		t.Fatal("literal negation descendant was pruned")
	}
}

func TestShouldSkipDirScopesAnchoredNegation(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".gitignore", "vendor/\nnode_modules/\n")
	writeIgnoreTestFile(t, root, ".grepaiignore", "!/vendor/keep.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.ShouldSkipDir("vendor") {
		t.Fatal("anchored negation descendant was pruned")
	}
	if !m.ShouldSkipDir("node_modules") {
		t.Fatal("anchored negation affected unrelated directory")
	}
}

func TestShouldSkipDirScopesNestedAndWildcardNegations(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".gitignore", "packages/\nnode_modules/\nvendor/\n")
	writeIgnoreTestFile(t, root, filepath.Join("packages", "app", ".grepaiignore"), "!generated/keep.go\n")
	writeIgnoreTestFile(t, root, ".grepaiignore", "!vendor/**/keep.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"packages", filepath.Join("packages", "app"), filepath.Join("packages", "app", "generated"), "vendor", filepath.Join("vendor", "deep")} {
		if m.ShouldSkipDir(dir) {
			t.Fatalf("valid negation path %q was pruned", dir)
		}
	}
	if !m.ShouldSkipDir("node_modules") {
		t.Fatal("nested/wildcard negations affected unrelated node_modules")
	}
}

func TestShouldSkipDirConservativelyHandlesBroadNegations(t *testing.T) {
	for _, pattern := range []string{"!keep.go\n", "!*/keep.go\n", "!**/keep.go\n", "!file[0-9].go\n"} {
		t.Run(pattern, func(t *testing.T) {
			root := t.TempDir()
			writeIgnoreTestFile(t, root, ".gitignore", "ignored/\n")
			writeIgnoreTestFile(t, root, ".grepaiignore", pattern)
			m, err := NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if m.ShouldSkipDir("ignored") {
				t.Fatalf("broad valid negation %q was falsely pruned", pattern)
			}
		})
	}
}

func TestRefreshRecomputesScopedNegationRules(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".grepaiignore")
	writeIgnoreTestFile(t, root, ".gitignore", "node_modules/\nvendor/\n")
	writeIgnoreTestFile(t, root, ".grepaiignore", "!vendor/keep.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("!node_modules/keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkipDir("vendor") || m.ShouldSkipDir("node_modules") {
		t.Fatal("refresh retained stale negation scope")
	}
}
