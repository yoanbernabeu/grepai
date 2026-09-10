//go:build windows

package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func renameWatchedTreeOut(t *testing.T, inside, _ string) string {
	t.Helper()
	// Windows can reject renames while fsnotify holds separately watched
	// descendant directory handles. The Windows fixture intentionally has no
	// nested watched directory; nested cleanup remains covered by unit tests.
	renamed := filepath.Join(filepath.Dir(inside), "renamed")
	if err := os.Rename(inside, renamed); err != nil {
		t.Fatal(err)
	}
	return renamed
}

func renameFixtureRelativePath() string {
	return "main.go"
}
