package watcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestCreateDirectoryNamedLikeSourceRegistersDescendants(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	defer w.Close()
	generated := filepath.Join(root, "generated.go")
	child := filepath.Join(generated, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	added := make(map[string]bool)
	w.addWatch = func(path string) error { added[path] = true; return nil }
	if err := w.handleEvent(fsnotify.Event{Name: generated, Op: fsnotify.Create}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	if !added[generated] || !added[child] {
		t.Fatalf("registered paths = %#v, want generated.go and child directories", added)
	}
}

func TestRuntimeVanishedDirectoryAddENOENTIsIgnored(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	created := filepath.Join(root, "vanished")
	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}
	w.addWatch = func(string) error { return syscall.ENOENT }
	if err := w.handleEvent(fsnotify.Event{Name: created, Op: fsnotify.Create}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
}

func TestCreateStatFailuresPublishFatal(t *testing.T) {
	for _, cause := range []error{syscall.EACCES, syscall.EIO} {
		t.Run(cause.Error(), func(t *testing.T) {
			root := t.TempDir()
			w := newTestWatcher(t, root)
			w.statPath = func(string) (fs.FileInfo, error) { return nil, cause }
			events := make(chan fsnotify.Event)
			w.backendEvents, w.backendErrors = events, make(chan error)
			if err := w.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			events <- fsnotify.Event{Name: filepath.Join(root, "newdir"), Op: fsnotify.Create}
			err := <-w.Errors()
			var fatalErr *FatalError
			if !errors.As(err, &fatalErr) || !errors.Is(err, cause) {
				t.Fatalf("Errors() = %T %v, want FatalError preserving %v", err, err, cause)
			}
			<-w.processingDone
			_ = w.Close()
		})
	}
}

func TestCreateStatENOENTIsIgnored(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	w.statPath = func(string) (fs.FileInfo, error) { return nil, syscall.ENOENT }
	if err := w.handleEvent(fsnotify.Event{Name: filepath.Join(root, "gone"), Op: fsnotify.Create}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	_ = w.Close()
}

func TestRelativePathFailurePublishesFatal(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	events := make(chan fsnotify.Event)
	w.backendEvents, w.backendErrors = events, make(chan error)
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.relPath = func(string, string) (string, error) { return "", syscall.EIO }
	events <- fsnotify.Event{Name: filepath.Join(root, "new.go"), Op: fsnotify.Create}
	err := <-w.Errors()
	var fatalErr *FatalError
	if !errors.As(err, &fatalErr) || !errors.Is(err, syscall.EIO) {
		t.Fatalf("Errors() = %T %v, want EIO FatalError", err, err)
	}
	<-w.processingDone
	_ = w.Close()
}

func TestFullEventQueueBackpressuresWithoutFatal(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < cap(w.events); i++ {
		w.events <- FileEvent{Path: "barrier"}
	}
	w.pending["queued.go"] = FileEvent{Type: EventModify, Path: "queued.go"}
	flushed := make(chan struct{})
	go func() { w.flush(); close(flushed) }()
	select {
	case <-flushed:
		t.Fatal("flush bypassed event backpressure")
	default:
	}
	<-w.Events()
	<-flushed
	select {
	case err := <-w.Errors():
		t.Fatalf("backpressure published fatal error: %v", err)
	default:
	}
}

func TestUnexpectedBackendChannelClosureIsFatal(t *testing.T) {
	setups := []struct {
		name string
		set  func(*Watcher) func()
	}{
		{"events", func(w *Watcher) func() {
			ch := make(chan fsnotify.Event)
			w.backendEvents, w.backendErrors = ch, make(chan error)
			return func() { close(ch) }
		}},
		{"errors", func(w *Watcher) func() {
			ch := make(chan error)
			w.backendEvents, w.backendErrors = make(chan fsnotify.Event), ch
			return func() { close(ch) }
		}},
	}
	for _, tc := range setups {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWatcher(t, t.TempDir())
			closeBackend := tc.set(w)
			if err := w.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			closeBackend()
			var fatalErr *FatalError
			if err := <-w.Errors(); !errors.As(err, &fatalErr) {
				t.Fatalf("Errors() = %T %v", err, err)
			}
			<-w.processingDone
			_ = w.Close()
		})
	}
}

func TestBackendClosureAfterContextCancellationIsClean(t *testing.T) {
	w := newTestWatcher(t, t.TempDir())
	events := make(chan fsnotify.Event)
	w.backendEvents, w.backendErrors = events, make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-w.processingDone
	close(events)
	select {
	case err, ok := <-w.Errors():
		if ok {
			t.Fatalf("Errors() = %v", err)
		}
	default:
	}
	_ = w.Close()
}

func TestUnderlyingFSNotifyErrorPublishesFatalAndStops(t *testing.T) {
	w := newTestWatcher(t, t.TempDir())
	backendErrors := make(chan error)
	w.backendEvents, w.backendErrors = make(chan fsnotify.Event), backendErrors
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	backendErrors <- fsnotify.ErrEventOverflow
	err := <-w.Errors()
	var fatalErr *FatalError
	if !errors.As(err, &fatalErr) || !errors.Is(err, fsnotify.ErrEventOverflow) {
		t.Fatalf("Errors() = %T %v, want overflow FatalError", err, err)
	}
	<-w.processingDone
}

func TestReadyRefusesAlreadyPublishedFatal(t *testing.T) {
	w := newTestWatcher(t, t.TempDir())
	defer w.Close()
	fatal := &FatalError{Operation: "watch", Cause: syscall.ENOSPC}
	w.publishFatal(fatal)
	called := false
	err := w.Ready(func() error {
		called = true
		return nil
	})
	if !errors.Is(err, fatal) {
		t.Fatalf("Ready() error = %v, want fatal error", err)
	}
	if called {
		t.Fatal("Ready() invoked callback after fatal publication")
	}
}
