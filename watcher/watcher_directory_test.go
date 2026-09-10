package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

func newDirectoryTestWatcher(t *testing.T, root string) *Watcher {
	t.Helper()
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(root, ignore, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestCreatePopulatedDirectoryEmitsSupportedDescendants(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	created := filepath.Join(root, "generated.go")
	child := filepath.Join(created, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "main.go"), []byte("package nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: created, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		if event.Type != EventCreate || event.Path != filepath.Join("generated.go", "nested", "main.go") {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("populated directory emitted no file event")
	}
}

func TestTrackedDirectoryRenameBypassesFileFilters(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	dir := filepath.Join(root, "generated.go")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.addRecursive(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Rename}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	event := <-w.Events()
	if event.Type != EventRename || event.Path != "generated.go" || !event.IsDir {
		t.Fatalf("event = %#v, want directory rename", event)
	}
}

func TestDirectoryRenameReleasesSubtreeWatches(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	dir := filepath.Join(root, "src")
	child := filepath.Join(dir, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.addRecursive(dir); err != nil {
		t.Fatal(err)
	}
	removed := make(map[string]bool)
	w.removeWatch = func(path string) error {
		removed[path] = true
		if path == dir {
			return fsnotify.ErrNonExistentWatch
		}
		return nil
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Rename}); err != nil {
		t.Fatal(err)
	}
	if !removed[dir] || !removed[child] {
		t.Fatalf("removed = %#v", removed)
	}
}

func TestDirectoryWatchRemovalFailureIsReturned(t *testing.T) {
	for _, cause := range []error{syscall.EIO, syscall.EBADF} {
		t.Run(cause.Error(), func(t *testing.T) {
			root := t.TempDir()
			w := newDirectoryTestWatcher(t, root)
			dir := filepath.Join(root, "src")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := w.addRecursive(dir); err != nil {
				t.Fatal(err)
			}
			w.removeWatch = func(string) error { return cause }
			err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Remove})
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want %v", err, cause)
			}
			w.flush()
			select {
			case event := <-w.Events():
				if event.Type != EventDelete || event.Path != "src" || !event.IsDir {
					t.Fatalf("cleanup event = %#v", event)
				}
			default:
				t.Fatal("watch removal failure suppressed directory cleanup")
			}
		})
	}
}

func TestMovedTreeLoadsIgnoreFilesBeforeEmittingDescendants(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	staged := filepath.Join(outside, "src")
	if err := os.MkdirAll(filepath.Join(staged, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".gitignore":                             "*.go\n",
		filepath.Join("nested", ".grepaiignore"): "!keep.go\n",
		"drop.go":                                "package drop",
		filepath.Join("nested", "drop.go"):       "package drop",
		filepath.Join("nested", "keep.go"):       "package keep",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(staged, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inside := filepath.Join(root, "src")
	if err := os.Rename(staged, inside); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: inside, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		if event.Path != filepath.Join("src", "nested", "keep.go") {
			t.Fatalf("event = %#v, want only negated descendant", event)
		}
	default:
		t.Fatal("negated descendant was not emitted")
	}
	select {
	case event := <-w.Events():
		t.Fatalf("ignored descendant was emitted: %#v", event)
	default:
	}
}

func TestWatcherUsesScannerCustomExtensions(t *testing.T) {
	root := t.TempDir()
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore).WithCustomExtensions([]string{".tengo"})
	w, err := NewWatcher(root, ignore, 10, WithFileFilter(scanner.SupportsPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	dir := filepath.Join(root, "generated")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tengo"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.unknown"), []byte("unknown"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	w.flush()
	select {
	case event := <-w.Events():
		if event.Path != filepath.Join("generated", "main.tengo") {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("custom extension was not emitted")
	}
	select {
	case event := <-w.Events():
		t.Fatalf("unsupported extension emitted: %#v", event)
	default:
	}
}

func TestLinuxImplicitDirectoryRemovalEINVAL(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify-specific behavior")
	}
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	dir := filepath.Join(root, "src")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.addRecursive(dir); err != nil {
		t.Fatal(err)
	}
	originalRemove := w.removeWatch
	w.removeWatch = func(path string) error {
		_ = originalRemove(path)
		return syscall.EINVAL
	}
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Remove}); err != nil {
		t.Fatalf("implicitly removed watch: %v", err)
	}
}

func TestLinuxOwnedDirectoryRemovalEINVALIsReturned(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify-specific behavior")
	}
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	dir := filepath.Join(root, "src")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.addRecursive(dir); err != nil {
		t.Fatal(err)
	}
	w.removeWatch = func(string) error { return syscall.EINVAL }
	if err := w.handleEvent(fsnotify.Event{Name: dir, Op: fsnotify.Remove}); !errors.Is(err, syscall.EINVAL) {
		t.Fatalf("error = %v, want EINVAL", err)
	}
}
