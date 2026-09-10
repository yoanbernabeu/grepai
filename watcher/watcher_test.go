package watcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

func newTestWatcher(t *testing.T, root string) *Watcher {
	t.Helper()
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatalf("NewIgnoreMatcher() error = %v", err)
	}
	w, err := NewWatcher(root, ignore, 0)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	return w
}

func TestStartAddENOSPCFailsAndAbortsWatcher(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newTestWatcher(t, root)
	originalAdd := w.addWatch
	w.addWatch = func(path string) error {
		if path == child {
			return syscall.ENOSPC
		}
		return originalAdd(path)
	}

	err := w.Start(context.Background())
	var registrationErr *RegistrationError
	if !errors.As(err, &registrationErr) {
		t.Fatalf("Start() error = %T %v, want *RegistrationError", err, err)
	}
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("Start() error = %v, want errors.Is(ENOSPC)", err)
	}
	if registrationErr.Operation != "add watch" || registrationErr.Path != child {
		t.Fatalf("registration error = %#v", registrationErr)
	}
	if w.processingDone != nil {
		t.Fatal("Start() launched event processing after registration failure")
	}
	if err := w.watcher.Add(root); err != nil {
		t.Fatalf("fatal abort closed fsnotify backend: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() after failed Start = %v", err)
	}
}

func TestRegistrationErrorPreservesResourceErrors(t *testing.T) {
	for _, cause := range []error{syscall.ENOSPC, syscall.EMFILE, syscall.ENFILE} {
		err := &RegistrationError{Operation: "add watch", Path: "/project", Cause: cause}
		var registrationErr *RegistrationError
		if !errors.As(err, &registrationErr) || !errors.Is(err, cause) {
			t.Errorf("RegistrationError with %v does not preserve type and cause", cause)
		}
	}
}

func TestStartIgnoresVanishedDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "vanished")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newTestWatcher(t, root)
	originalAdd := w.addWatch
	w.addWatch = func(path string) error {
		if path == child {
			return syscall.ENOENT
		}
		return originalAdd(path)
	}

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	<-w.processingDone
}

func TestStartRootAddENOENTIsFatal(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)
	defer w.Close()
	w.addWatch = func(string) error { return syscall.ENOENT }

	err := w.Start(context.Background())
	var registrationErr *RegistrationError
	if !errors.As(err, &registrationErr) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Start() error = %T %v, want root ENOENT RegistrationError", err, err)
	}
	if registrationErr.Path != root {
		t.Fatalf("registration path = %q, want %q", registrationErr.Path, root)
	}
}

func TestStartMissingRootFailsAndClosesWatcher(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	w := newTestWatcher(t, root)

	err := w.Start(context.Background())
	var registrationErr *RegistrationError
	if !errors.As(err, &registrationErr) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Start() error = %T %v, want missing-root RegistrationError", err, err)
	}
	if registrationErr.Operation != "walk watch tree" || registrationErr.Path != root {
		t.Fatalf("registration error = %#v", registrationErr)
	}
	if w.processingDone != nil {
		t.Fatal("Start() launched event processing for missing root")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() after failed Start = %v", err)
	}
}

func TestRuntimeDirectoryAddFailurePublishesFatalAndStops(t *testing.T) {
	root := t.TempDir()
	created := filepath.Join(root, "created")
	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newTestWatcher(t, root)
	backendEvents := make(chan fsnotify.Event)
	w.backendEvents, w.backendErrors = backendEvents, make(chan error)
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Close()
	originalAdd := w.addWatch
	w.addWatch = func(path string) error {
		if path == created {
			return syscall.ENOSPC
		}
		return originalAdd(path)
	}
	backendEvents <- fsnotify.Event{Name: created, Op: fsnotify.Create}

	err := <-w.Errors()
	var registrationErr *RegistrationError
	if !errors.As(err, &registrationErr) || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("Errors() = %T %v, want ENOSPC registration error", err, err)
	}
	<-w.processingDone
}

// TestContextCancelKeepsOutputsOpenUntilClose pins the owner-managed output
// lifecycle: context cancellation joins the workers but must not close the
// Events/Errors channels; only an explicit Close releases them. Closing early
// lets graceful CLI drains observe a closed Errors channel and mistake the
// nil receive for a fatal error.
func TestContextCancelKeepsOutputsOpenUntilClose(t *testing.T) {
	w := newTestWatcher(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	cancel()

	workersDone := make(chan struct{})
	go func() {
		w.workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not exit after context cancellation")
	}

	select {
	case err, ok := <-w.Errors():
		t.Fatalf("Errors() closed before Close(); received err=%v ok=%v", err, ok)
	default:
	}
	select {
	case event, ok := <-w.Events():
		t.Fatalf("Events() closed before Close(); received event=%v ok=%v", event, ok)
	default:
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, ok := <-w.Errors(); ok {
		t.Fatal("Errors() still open after Close()")
	}
	if _, ok := <-w.Events(); ok {
		t.Fatal("Events() still open after Close()")
	}
}
