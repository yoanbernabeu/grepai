package watcher

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

func TestIgnoreRefreshEmitsReconcileScope(t *testing.T) {
	tests := []struct {
		scope string
		name  string
	}{
		{scope: ".", name: ".gitignore"},
		{scope: filepath.Join("nested", "policy"), name: ".grepaiignore"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.scope, func(t *testing.T) {
			root := t.TempDir()
			w := newDirectoryTestWatcher(t, root)
			dir := root
			if tt.scope != "." {
				dir = filepath.Join(root, tt.scope)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, tt.name)
			if err := os.WriteFile(path, []byte("ignored.go\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := w.handleEvent(fsnotify.Event{Name: path, Op: fsnotify.Write}); err != nil {
				t.Fatal(err)
			}
			w.flush()
			event := receiveReconcileEvent(t, w)
			if event.Type != EventReconcile || event.Path != tt.scope || !event.IsDir {
				t.Fatalf("event = %#v", event)
			}
		})
	}
}

func TestFailedIgnoreRefreshDoesNotEmitReconcile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".grepaiignore")
	if err := os.WriteFile(path, []byte("secret.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := newDirectoryTestWatcher(t, root)
	w.refreshIgnore = func(string) error { return syscall.EIO }
	if err := w.handleEvent(fsnotify.Event{Name: path, Op: fsnotify.Write}); err == nil {
		t.Fatal("refresh unexpectedly succeeded")
	}
	w.flush()
	select {
	case event := <-w.Events():
		t.Fatalf("failed refresh emitted event: %#v", event)
	default:
	}
	if !w.ignore.ShouldIgnore("secret.go") {
		t.Fatal("failed refresh replaced the old policy")
	}
}

func TestReconcileAndPhysicalEventsCoalesceIndependently(t *testing.T) {
	w := newDirectoryTestWatcher(t, t.TempDir())
	w.debounceMs = int(time.Hour / time.Millisecond)
	w.debounceEvent(FileEvent{Type: EventDelete, Path: ".", IsDir: true})
	w.debounceEvent(FileEvent{Type: EventReconcile, Path: ".", IsDir: true})
	if w.timer != nil {
		w.timer.Stop()
	}
	w.flush()
	seen := map[EventType]FileEvent{}
	for i := 0; i < 2; i++ {
		event := receiveReconcileEvent(t, w)
		seen[event.Type] = event
	}
	if !seen[EventDelete].IsDir || !seen[EventReconcile].IsDir {
		t.Fatalf("coalesced events = %#v", seen)
	}
}

func TestIgnoreRefreshRegistersNewlyUnignoredDirectoryForFutureEvents(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, ".grepaiignore")
	if err := os.WriteFile(ignorePath, []byte("hidden/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(root, "hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(root, ignore, 5)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	reconcile := receiveReconcileEvent(t, w)
	if reconcile.Type != EventReconcile || reconcile.Path != "." {
		t.Fatalf("refresh event = %#v", reconcile)
	}
	w.directoriesMu.Lock()
	_, registered := w.registered[hidden]
	w.directoriesMu.Unlock()
	if !registered {
		t.Fatal("newly unignored directory was not registered")
	}

	futurePath := filepath.Join(hidden, "future.go")
	if err := os.WriteFile(futurePath, []byte("package hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := receiveReconcileEvent(t, w)
	if (future.Type != EventCreate && future.Type != EventModify) || future.Path != filepath.Join("hidden", "future.go") {
		t.Fatalf("future event = %#v", future)
	}
}

func receiveReconcileEvent(t *testing.T, w *Watcher) FileEvent {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case event := <-w.Events():
		return event
	case <-timer.C:
		t.Fatal("timed out waiting for watcher event")
		return FileEvent{}
	}
}
