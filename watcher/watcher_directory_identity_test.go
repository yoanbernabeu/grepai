package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

func TestFailedChildWatchStillEmitsDirectoryCleanup(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	w.addWatch = func(path string) error {
		if path == child {
			return syscall.EIO
		}
		return nil
	}
	removed := make(map[string]bool)
	w.removeWatch = func(path string) error {
		removed[path] = true
		return nil
	}
	if err := w.addRecursive(parent); !errors.Is(err, syscall.EIO) {
		t.Fatalf("addRecursive error = %v, want fail-closed EIO", err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: child, Op: fsnotify.Remove}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		if event.Type != EventDelete || event.Path != filepath.Join("parent", "child") || !event.IsDir {
			t.Fatalf("cleanup event = %#v", event)
		}
	default:
		t.Fatal("failed watch directory lost logical identity")
	}
	if removed[child] {
		t.Fatal("failed Add was treated as a registered watch")
	}
}

func TestHiddenPopulatedDirectoryCreateAndRename(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	dir := filepath.Join(root, ".github")
	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantFile := filepath.Join(".github", "workflows", "ci.go")
	if err := os.WriteFile(filepath.Join(root, wantFile), []byte("package workflows"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	if event := receiveDirectoryTestEvent(t, w); event.Type != EventCreate || event.Path != wantFile {
		t.Fatalf("create event = %#v", event)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Rename}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	if event := receiveDirectoryTestEvent(t, w); event.Type != EventRename || event.Path != ".github" || !event.IsDir {
		t.Fatalf("rename event = %#v", event)
	}
}

func receiveDirectoryTestEvent(t *testing.T, w *Watcher) FileEvent {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case event := <-w.Events():
		return event
	case <-timer.C:
		t.Fatal("timed out waiting for directory event")
		return FileEvent{}
	}
}

func TestImportedIgnoredRootLoadsOwnNegationBeforeSkip(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".grepaiignore"), []byte("incoming/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	incoming := filepath.Join(root, "incoming")
	if err := os.Mkdir(incoming, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incoming, ".grepaiignore"), []byte("!keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incoming, "keep.go"), []byte("package keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: incoming, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		if event.Path != filepath.Join("incoming", "keep.go") {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("imported subtree negation was not loaded before skip decision")
	}
}

func TestWrappedMissingWatchErrorIsBenign(t *testing.T) {
	w := newDirectoryTestWatcher(t, t.TempDir())
	err := &os.PathError{Op: "GetFileAttributes", Path: `C:\missing`, Err: syscall.ENOENT}
	if !w.benignRemoveWatchError("missing", err) {
		t.Fatalf("wrapped missing-path error was not benign: %v", err)
	}
	if w.benignRemoveWatchError("missing", syscall.EIO) || w.benignRemoveWatchError("missing", syscall.EBADF) {
		t.Fatal("I/O or descriptor failure was incorrectly classified as absence")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("test prerequisite: wrapped error must classify as not-exist")
	}
}

func TestImportedMetadataDirectoryRemainsHardExcluded(t *testing.T) {
	root := t.TempDir()
	ignore, err := indexer.NewIgnoreMatcher(root, []string{".git", ".grepai"}, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(root, ignore, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	metadata := filepath.Join(root, ".git")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, ".grepaiignore"), []byte("!keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "keep.go"), []byte("package keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: metadata, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		t.Fatalf("metadata directory emitted event: %#v", event)
	default:
	}
	if _, tracked := w.directories[metadata]; tracked {
		t.Fatal("metadata directory was tracked")
	}
}

func TestNegationScopeDoesNotWatchUnrelatedIgnoredTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".grepaiignore"), []byte("vendor/\n!vendor/keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "large", "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	added := make(map[string]bool)
	w.addWatch = func(path string) error {
		added[path] = true
		return nil
	}
	if err := w.addRecursive(root); err != nil {
		t.Fatal(err)
	}
	if !added[filepath.Join(root, "vendor")] {
		t.Fatal("relevant negation directory was not watched")
	}
	for path := range added {
		if path == filepath.Join(root, "node_modules") || strings.HasPrefix(path, filepath.Join(root, "node_modules")+string(filepath.Separator)) {
			t.Fatalf("unrelated ignored tree was watched: %s", path)
		}
	}
}
