package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealFSNotifyPopulatedMoveInAndRenameAway(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	staged := filepath.Join(outside, "src")
	fixtureFile := renameFixtureRelativePath()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(staged, fixtureFile)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, fixtureFile), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	w.debounceMs = 20
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "src")
	if err := os.Rename(staged, inside); err != nil {
		t.Fatal(err)
	}
	wantChild := filepath.Join("src", fixtureFile)
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return event.Type == EventCreate && event.Path == wantChild
	})

	renamed := renameWatchedTreeOut(t, inside, outside)
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return event.Type == EventRename && event.Path == "src" && event.IsDir
	})
	for _, watched := range w.watcher.WatchList() {
		if watched == inside || strings.HasPrefix(watched, inside+string(filepath.Separator)) {
			t.Fatalf("renamed-out subtree remains watched: %#v", w.watcher.WatchList())
		}
	}
	if err := os.WriteFile(filepath.Join(renamed, "outside.go"), []byte("package outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertNoFileEvent(t, w, filepath.Join("src", "outside.go"), 200*time.Millisecond)
}

func TestRealFSNotifyDirectoryDeletionAndRecreation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "generated.go")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	w.debounceMs = 20
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return event.Type == EventDelete && event.Path == "generated.go" && event.IsDir
	})
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return (event.Type == EventCreate || event.Type == EventModify) && event.Path == filepath.Join("generated.go", "new.go")
	})
}

func TestRealFSNotifyWatchesIgnoredDirectoryWithIncludedChild(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".grepaiignore"), []byte("vendor/\n!vendor/important/keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	important := filepath.Join(root, "vendor", "important")
	if err := os.MkdirAll(important, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	w.debounceMs = 20
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(important, "keep.go"), []byte("package keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	awaitFileEvent(t, w, func(event FileEvent) bool {
		return event.Path == filepath.Join("vendor", "important", "keep.go")
	})
}

func awaitFileEvent(t *testing.T, w *Watcher, matches func(FileEvent) bool) FileEvent {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-w.Events():
			if matches(event) {
				return event
			}
		case <-timer.C:
			t.Fatal("timed out waiting for filesystem event")
		}
	}
}

func assertNoFileEvent(t *testing.T, w *Watcher, path string, duration time.Duration) {
	t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case event := <-w.Events():
			if event.Path == path {
				t.Fatalf("unexpected event for released path: %#v", event)
			}
		case <-timer.C:
			return
		}
	}
}
