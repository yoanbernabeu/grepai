//go:build !windows

package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func renameWatchedTreeOut(t *testing.T, inside, outside string) string {
	t.Helper()
	renamed := filepath.Join(outside, "renamed")
	if err := os.Rename(inside, renamed); err != nil {
		t.Fatal(err)
	}
	return renamed
}

func renameFixtureRelativePath() string {
	return filepath.Join("nested", "main.go")
}
