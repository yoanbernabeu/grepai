package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyFileIfExists(t *testing.T) {
	t.Run("copies existing file", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.txt")
		dst := filepath.Join(dir, "dst.txt")

		os.WriteFile(src, []byte("hello"), 0644)

		err := copyFileIfExists(src, dst)
		if err != nil {
			t.Fatalf("copyFileIfExists failed: %v", err)
		}

		data, _ := os.ReadFile(dst)
		if string(data) != "hello" {
			t.Errorf("expected 'hello', got %q", string(data))
		}
	})

	t.Run("returns nil for missing file", func(t *testing.T) {
		dir := t.TempDir()
		err := copyFileIfExists(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
		if err != nil {
			t.Fatalf("expected nil error for missing file, got: %v", err)
		}
	})
}

func TestEnsureGitignoreEntry(t *testing.T) {
	t.Run("creates gitignore if missing", func(t *testing.T) {
		dir := t.TempDir()
		ensureGitignoreEntry(dir, ".grepai/")

		data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}
		if !strings.Contains(string(data), ".grepai/") {
			t.Error("expected .grepai/ in .gitignore")
		}
	})

	t.Run("appends to existing gitignore", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n"), 0644)

		ensureGitignoreEntry(dir, ".grepai/")

		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if !strings.Contains(string(data), "node_modules") {
			t.Error("expected existing entries preserved")
		}
		if !strings.Contains(string(data), ".grepai/") {
			t.Error("expected .grepai/ added")
		}
	})

	t.Run("adds newline before appending when missing trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules"), 0644)

		ensureGitignoreEntry(dir, ".grepai/")

		data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}
		if string(data) != "node_modules\n.grepai/\n" {
			t.Errorf("unexpected .gitignore content: %q", string(data))
		}
	})
	t.Run("does not duplicate entry", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".grepai/\n"), 0644)

		ensureGitignoreEntry(dir, ".grepai/")

		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		count := strings.Count(string(data), ".grepai/")
		if count != 1 {
			t.Errorf("expected 1 occurrence, got %d", count)
		}
	})
}

func TestAutoInitFromMainWorktree(t *testing.T) {
	t.Run("copies config and index files", func(t *testing.T) {
		mainDir := t.TempDir()
		worktreeDir := t.TempDir()

		// Set up main worktree .grepai/
		mainGrepai := filepath.Join(mainDir, ".grepai")
		os.MkdirAll(mainGrepai, 0755)
		os.WriteFile(filepath.Join(mainGrepai, "config.yaml"), []byte("version: 1\n"), 0644)
		os.WriteFile(filepath.Join(mainGrepai, "index.gob"), []byte("index-data"), 0644)
		os.WriteFile(filepath.Join(mainGrepai, "symbols.gob"), []byte("symbol-data"), 0644)

		err := autoInitFromMainWorktree(worktreeDir, mainDir)
		if err != nil {
			t.Fatalf("autoInitFromMainWorktree failed: %v", err)
		}

		// Verify files were copied
		localGrepai := filepath.Join(worktreeDir, ".grepai")
		if _, err := os.Stat(filepath.Join(localGrepai, "config.yaml")); os.IsNotExist(err) {
			t.Error("config.yaml not copied")
		}
		if _, err := os.Stat(filepath.Join(localGrepai, "index.gob")); os.IsNotExist(err) {
			t.Error("index.gob not copied")
		}
		if _, err := os.Stat(filepath.Join(localGrepai, "symbols.gob")); os.IsNotExist(err) {
			t.Error("symbols.gob not copied")
		}

		// Verify .gitignore was updated
		gitignore, err := os.ReadFile(filepath.Join(worktreeDir, ".gitignore"))
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}
		if !strings.Contains(string(gitignore), ".grepai/") {
			t.Error(".grepai/ not added to .gitignore")
		}
	})

	t.Run("fails if config.yaml missing in main", func(t *testing.T) {
		mainDir := t.TempDir()
		worktreeDir := t.TempDir()

		// Main has .grepai/ but no config.yaml
		os.MkdirAll(filepath.Join(mainDir, ".grepai"), 0755)

		err := autoInitFromMainWorktree(worktreeDir, mainDir)
		if err == nil {
			t.Error("expected error when config.yaml missing")
		}
	})
}
