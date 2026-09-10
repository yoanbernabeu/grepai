package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRealFSNotifyProcessesIndexableDotDirectory(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	w.debounceMs = 20
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".github")
	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflows", "ci.go"), []byte("package workflows"), 0o644); err != nil {
		t.Fatal(err)
	}
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return event.Path == filepath.Join(".github", "workflows", "ci.go")
	})
}
