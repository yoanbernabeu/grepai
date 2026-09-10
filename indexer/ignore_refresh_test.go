package indexer

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRefreshSubtreeDoesNotWalkUnrelatedDirectories(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, filepath.Join("old", ".gitignore"), "old.go\n")
	writeIgnoreTestFile(t, root, filepath.Join("incoming", ".gitignore"), "before.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	walked := make(chan string, 1)
	m.walkIgnoreFiles = func(path string, fn filepath.WalkFunc) error {
		walked <- path
		return filepath.Walk(path, fn)
	}
	writeIgnoreTestFile(t, root, filepath.Join("incoming", ".gitignore"), "after.go\n")
	if err := m.RefreshSubtree("incoming"); err != nil {
		t.Fatal(err)
	}
	if got := <-walked; got != filepath.Join(root, "incoming") {
		t.Fatalf("walk root = %q, want only incoming subtree", got)
	}
	select {
	case extra := <-walked:
		t.Fatalf("unexpected additional walk of %q", extra)
	default:
	}
	if !m.ShouldIgnore(filepath.Join("old", "old.go")) {
		t.Fatal("refresh discarded unrelated subtree rules")
	}
	if !m.ShouldIgnore(filepath.Join("incoming", "after.go")) {
		t.Fatal("refresh did not load replacement subtree rule")
	}
}

func TestRefreshSubtreeReplacesRemovedNestedRules(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join("tree", "nested", ".gitignore")
	writeIgnoreTestFile(t, root, ignorePath, "old.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	writeIgnoreTestFile(t, root, ignorePath, "new.go\n")
	if err := m.RefreshSubtree("tree"); err != nil {
		t.Fatal(err)
	}
	if m.ShouldIgnore(filepath.Join("tree", "nested", "old.go")) {
		t.Fatal("removed nested rule remained active")
	}
	if !m.ShouldIgnore(filepath.Join("tree", "nested", "new.go")) {
		t.Fatal("replacement nested rule was not loaded")
	}
}

func TestRefreshSubtreePreservesRootNegation(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".gitignore", "vendor/\n")
	writeIgnoreTestFile(t, root, ".grepaiignore", "!vendor/keep.go\n")
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RefreshSubtree("vendor"); err != nil {
		t.Fatal(err)
	}
	if m.ShouldIgnore(filepath.Join("vendor", "keep.go")) {
		t.Fatal("scoped refresh discarded root negation")
	}
	if !m.ShouldIgnore(filepath.Join("vendor", "drop.go")) {
		t.Fatal("scoped refresh discarded root ignore rule")
	}
}

func TestRefreshIsTransactionalOnReadFailure(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, ".gitignore")
	writeIgnoreTestFile(t, root, ".gitignore", "secret.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	m.readIgnoreFile = func(path string) ([]byte, error) {
		if path == ignorePath {
			return nil, syscall.EIO
		}
		return os.ReadFile(path)
	}
	if err := m.Refresh(); !errors.Is(err, syscall.EIO) {
		t.Fatalf("Refresh error = %v, want EIO", err)
	}
	if !m.ShouldIgnore("secret.go") {
		t.Fatal("failed refresh weakened the existing root rule")
	}
}

func TestRefreshSubtreeIsTransactionalOnWalkFailure(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, filepath.Join("nested", ".gitignore"), "secret.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	m.walkIgnoreFiles = func(string, filepath.WalkFunc) error { return syscall.EIO }
	if err := m.RefreshSubtree("nested"); !errors.Is(err, syscall.EIO) {
		t.Fatalf("RefreshSubtree error = %v, want EIO", err)
	}
	if !m.ShouldIgnore(filepath.Join("nested", "secret.go")) {
		t.Fatal("failed subtree refresh weakened the existing rule")
	}
}

func TestRefreshIsTransactionalOnWalkFailure(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".gitignore", "secret.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	m.walkIgnoreFiles = func(string, filepath.WalkFunc) error { return syscall.EIO }
	if err := m.Refresh(); !errors.Is(err, syscall.EIO) {
		t.Fatalf("Refresh error = %v, want EIO", err)
	}
	if !m.ShouldIgnore("secret.go") {
		t.Fatal("failed root walk weakened the existing rule")
	}
}

func TestRefreshSubtreeIsTransactionalOnReadFailure(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, "nested", ".gitignore")
	writeIgnoreTestFile(t, root, filepath.Join("nested", ".gitignore"), "secret.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	m.readIgnoreFile = func(path string) ([]byte, error) {
		if path == ignorePath {
			return nil, syscall.EIO
		}
		return os.ReadFile(path)
	}
	if err := m.RefreshSubtree("nested"); !errors.Is(err, syscall.EIO) {
		t.Fatalf("RefreshSubtree error = %v, want EIO", err)
	}
	if !m.ShouldIgnore(filepath.Join("nested", "secret.go")) {
		t.Fatal("failed subtree read weakened the existing rule")
	}
}

func TestRefreshSubtreeRemovesDeletedIgnoreFileRule(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, "nested", ".gitignore")
	writeIgnoreTestFile(t, root, filepath.Join("nested", ".gitignore"), "secret.go\n")
	m, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	if err := m.RefreshSubtree("nested"); err != nil {
		t.Fatal(err)
	}
	if m.ShouldIgnore(filepath.Join("nested", "secret.go")) {
		t.Fatal("deleted ignore file rule remained active")
	}
}

func TestScannerTraversesPositiveParentForSameFileNegation(t *testing.T) {
	root := t.TempDir()
	writeIgnoreTestFile(t, root, ".grepaiignore", "vendor/\n!vendor/important/keep.go\n")
	writeIgnoreTestFile(t, root, filepath.Join("vendor", "important", "keep.go"), "package keep")
	writeIgnoreTestFile(t, root, filepath.Join("vendor", "drop.go"), "package drop")
	m, err := NewIgnoreMatcher(root, []string{".git", ".grepai"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldIgnore("vendor") {
		t.Fatal("positive parent pattern should still ignore the parent")
	}
	if m.ShouldSkipDir("vendor") {
		t.Fatal("parent with a negated descendant must remain traversable")
	}
	if !m.ShouldSkipDir(".git") || !m.ShouldSkipDir(".grepai") {
		t.Fatal("configured metadata directories must remain hard excludes")
	}
	files, _, err := NewScanner(root, m).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != filepath.Join("vendor", "important", "keep.go") {
		t.Fatalf("scanned files = %#v", files)
	}
}

func writeIgnoreTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
