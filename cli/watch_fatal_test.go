package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

type fakeWatchSource struct {
	events   chan watcher.FileEvent
	errors   chan error
	closed   int
	aborted  int
	readyErr error
	readyFn  func(func() error) error
}

func newFakeWatchSource() *fakeWatchSource {
	return &fakeWatchSource{
		events: make(chan watcher.FileEvent),
		errors: make(chan error, 1),
	}
}

func (w *fakeWatchSource) Events() <-chan watcher.FileEvent { return w.events }
func (w *fakeWatchSource) Errors() <-chan error             { return w.errors }

func (w *fakeWatchSource) Ready(publish func() error) error {
	if w.readyFn != nil {
		return w.readyFn(publish)
	}
	if w.readyErr != nil {
		return w.readyErr
	}
	if publish == nil {
		return nil
	}
	return publish()
}

func (w *fakeWatchSource) Close() error {
	w.closed++
	return nil
}

func (w *fakeWatchSource) Abort() { w.aborted++ }

func TestWorkspaceReadinessRefusesPreloadedWatcherFatal(t *testing.T) {
	fatal := &watcher.FatalError{Operation: "watch", Cause: syscall.ENOSPC}
	source := newFakeWatchSource()
	source.readyErr = fatal
	published := false
	err := withWatchSourcesReady([]watchSource{source}, func() error {
		published = true
		return nil
	})
	if !errors.Is(err, fatal) {
		t.Fatalf("withWatchSourcesReady() error = %v, want fatal", err)
	}
	if published {
		t.Fatal("workspace ready callback ran after watcher fatal")
	}
}

func TestFatalWatcherErrorClassificationPreservesWrapping(t *testing.T) {
	registrationErr := &watcher.RegistrationError{Operation: "add watch", Path: "/project", Cause: syscall.EMFILE}
	if !isFatalWatcherError(errors.Join(errors.New("session failed"), registrationErr)) {
		t.Fatal("wrapped registration error was not classified as fatal")
	}
	if isFatalWatcherError(errors.New("optional initialization warning")) {
		t.Fatal("ordinary initialization error was classified as fatal")
	}
}

func TestMonitorWorkspaceWatcherFatalIdentifiesProjectAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := newFakeWatchSource()
	runtime := &workspaceProjectRuntime{
		project: config.ProjectEntry{Name: "api", Path: "/workspace/api"},
		watcher: source,
	}
	fatals := make(chan error, 1)
	fence := newWatchMutationFence()
	fence.addWatcher(source)
	mutationStarted := make(chan struct{})
	mutationCanceled := make(chan struct{})
	releaseMutation := make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- fence.handle(ctx, func(eventCtx context.Context) {
			close(mutationStarted)
			<-eventCtx.Done()
			close(mutationCanceled)
			<-releaseMutation
		})
	}()
	awaitWatchTestSignal(t, mutationStarted, "workspace mutation admission")
	withdrawn := make(chan struct{})
	done := make(chan struct{})
	go func() {
		monitorWorkspaceWatcher(ctx, runtime, fence, nil, func() { close(withdrawn) }, fatals)
		close(done)
	}()
	source.errors <- &watcher.FatalError{Operation: "process filesystem events", Cause: syscall.ENOSPC}
	awaitWatchTestSignal(t, mutationCanceled, "workspace mutation cancellation")
	if err := fence.handle(ctx, func(context.Context) { t.Error("mutation admitted after workspace fatal") }); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("post-fatal handle() error = %v, want ENOSPC", err)
	}
	select {
	case err := <-fatals:
		t.Fatalf("fatal delivered before mutation quiesced: %v", err)
	default:
	}
	select {
	case <-withdrawn:
		t.Fatal("workspace readiness withdrawn before mutation quiesced")
	default:
	}
	close(releaseMutation)
	if err := awaitWatchTestValue(t, mutationDone, "workspace mutation return"); err != nil {
		t.Fatalf("workspace mutation error = %v", err)
	}

	err := awaitWatchTestValue(t, fatals, "workspace fatal delivery")
	var projectErr *workspaceWatcherError
	if !errors.As(err, &projectErr) || projectErr.ProjectName != "api" || projectErr.ProjectPath != "/workspace/api" {
		t.Fatalf("fatal error = %#v, want tagged api project", err)
	}
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("fatal error = %v, want ENOSPC", err)
	}
	awaitWatchTestSignal(t, withdrawn, "workspace readiness withdrawal")
	awaitWatchTestSignal(t, done, "workspace fatal monitor return")
}

func TestInitializeWorkspaceRuntimesRegistrationFailureCleansPriorRuntime(t *testing.T) {
	first := newNonCooperativeCloseWatchSource()
	defer close(first.blockClose)
	symbolPath := filepath.Join(t.TempDir(), "symbols.gob")
	symbolStore := trace.NewGOBSymbolStore(symbolPath)
	ws := &config.Workspace{Projects: []config.ProjectEntry{
		{Name: "first", Path: "/first"},
		{Name: "second", Path: "/second"},
	}}
	initCalls := 0
	initFn := func(context.Context, *config.Workspace, config.ProjectEntry, embedder.Embedder, store.VectorStore, bool) (*workspaceProjectRuntime, watchSource, error) {
		initCalls++
		if initCalls == 1 {
			return &workspaceProjectRuntime{project: ws.Projects[0], watcher: first, symbolStore: symbolStore}, first, nil
		}
		return nil, nil, &watcher.RegistrationError{Operation: "add watch", Path: "/second", Cause: syscall.ENOSPC}
	}

	result := make(chan error, 1)
	go func() {
		_, _, err := initializeWorkspaceRuntimes(context.Background(), ws, nil, nil, false, initFn)
		result <- err
	}()
	var err error
	select {
	case <-first.closeStarted:
		t.Fatal("registration failure invoked non-cooperative watcher Close")
	case err = <-result:
	}
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, want ENOSPC", err)
	}
	if first.aborted != 1 || first.closed != 0 {
		t.Fatalf("prior watcher aborts/closes = %d/%d, want 1/0", first.aborted, first.closed)
	}
	if _, statErr := os.Stat(symbolPath); !os.IsNotExist(statErr) {
		t.Fatalf("fatal workspace startup serialized symbol store: %v", statErr)
	}
}

func TestInitializeWorkspaceRuntimesRequiredSymbolStoreFailureAbortsPriorWatcher(t *testing.T) {
	// Given a healthy first project and a second whose Postgres symbol store
	// factory/load fails with a required init error.
	first := newNonCooperativeCloseWatchSource()
	defer close(first.blockClose)
	symbolPath := filepath.Join(t.TempDir(), "symbols.gob")
	symbolStore := trace.NewGOBSymbolStore(symbolPath)
	loadErr := errors.New("postgres migration failed")
	ws := &config.Workspace{Projects: []config.ProjectEntry{
		{Name: "first", Path: "/first"},
		{Name: "second", Path: "/second"},
	}}
	initFn := func(_ context.Context, _ *config.Workspace, project config.ProjectEntry, _ embedder.Embedder, _ store.VectorStore, _ bool) (*workspaceProjectRuntime, watchSource, error) {
		if project.Name == "first" {
			return &workspaceProjectRuntime{project: ws.Projects[0], watcher: first, symbolStore: symbolStore}, first, nil
		}
		return nil, nil, &requiredSymbolStoreInitError{
			cause: fmt.Errorf("failed to load Postgres symbol index for %s: %w", project.Path, loadErr),
		}
	}

	// When workspace runtimes initialize.
	result := make(chan error, 1)
	go func() {
		runtimes, watchers, err := initializeWorkspaceRuntimes(context.Background(), ws, nil, nil, false, initFn)
		if runtimes != nil || watchers != nil {
			t.Errorf("partial runtimes/watchers returned after required init failure: %d/%d", len(runtimes), len(watchers))
		}
		result <- err
	}()
	var err error
	select {
	case <-first.closeStarted:
		t.Fatal("required init failure invoked non-cooperative watcher Close")
	case err = <-result:
	}

	// Then startup fails before readiness with the original cause retained,
	// the prior watcher aborted (never closed), and no store persisted.
	if !errors.Is(err, loadErr) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, want errors.Is(%v)", err, loadErr)
	}
	if !isRequiredSymbolStoreInitError(err) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, want required init classification", err)
	}
	var registrationErr *watcher.RegistrationError
	if errors.As(err, &registrationErr) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, must not be mislabeled as RegistrationError", err)
	}
	if first.aborted != 1 || first.closed != 0 {
		t.Fatalf("prior watcher aborts/closes = %d/%d, want 1/0", first.aborted, first.closed)
	}
	if _, statErr := os.Stat(symbolPath); !os.IsNotExist(statErr) {
		t.Fatalf("fatal workspace startup serialized symbol store: %v", statErr)
	}
}

func TestInitializeWorkspaceRuntimesPostgresScanFailureAbortsPriorWatcher(t *testing.T) {
	// Given a healthy first project and a second whose Postgres initial scan
	// fails after a successful load (marked required by the symbol loader).
	first := newNonCooperativeCloseWatchSource()
	defer close(first.blockClose)
	scanErr := errors.New("postgres initial scan failed")
	ws := &config.Workspace{Projects: []config.ProjectEntry{
		{Name: "first", Path: "/first"},
		{Name: "second", Path: "/second"},
	}}
	initFn := func(_ context.Context, _ *config.Workspace, project config.ProjectEntry, _ embedder.Embedder, _ store.VectorStore, _ bool) (*workspaceProjectRuntime, watchSource, error) {
		if project.Name == "first" {
			return &workspaceProjectRuntime{project: ws.Projects[0], watcher: first}, first, nil
		}
		return nil, nil, &requiredSymbolStoreInitError{cause: scanErr}
	}

	// When workspace runtimes initialize.
	result := make(chan error, 1)
	go func() {
		runtimes, watchers, err := initializeWorkspaceRuntimes(context.Background(), ws, nil, nil, false, initFn)
		if runtimes != nil || watchers != nil {
			t.Errorf("partial runtimes/watchers returned after required scan failure: %d/%d", len(runtimes), len(watchers))
		}
		result <- err
	}()
	var err error
	select {
	case <-first.closeStarted:
		t.Fatal("required scan failure invoked non-cooperative watcher Close")
	case err = <-result:
	}

	// Then startup fails before readiness with the original cause retained
	// and the prior watcher aborted exactly once, never closed.
	if !errors.Is(err, scanErr) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, want errors.Is(%v)", err, scanErr)
	}
	if !isRequiredSymbolStoreInitError(err) {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v, want required init classification", err)
	}
	if first.aborted != 1 || first.closed != 0 {
		t.Fatalf("prior watcher aborts/closes = %d/%d, want 1/0", first.aborted, first.closed)
	}
}

func TestInitializeWorkspaceRuntimesKeepsOptionalInitializationWarningBehavior(t *testing.T) {
	ws := &config.Workspace{Projects: []config.ProjectEntry{
		{Name: "optional-failure", Path: "/optional"},
		{Name: "healthy", Path: "/healthy"},
	}}
	healthy := newFakeWatchSource()
	initFn := func(_ context.Context, _ *config.Workspace, project config.ProjectEntry, _ embedder.Embedder, _ store.VectorStore, _ bool) (*workspaceProjectRuntime, watchSource, error) {
		if project.Name == "optional-failure" {
			return nil, nil, errors.New("optional index initialization failed")
		}
		return &workspaceProjectRuntime{project: project, watcher: healthy}, healthy, nil
	}

	runtimes, watchers, err := initializeWorkspaceRuntimes(context.Background(), ws, nil, nil, true, initFn)
	if err != nil {
		t.Fatalf("initializeWorkspaceRuntimes() error = %v", err)
	}
	if len(runtimes) != 1 || len(watchers) != 1 {
		t.Fatalf("initialized %d runtimes and %d watchers, want 1 each", len(runtimes), len(watchers))
	}
	closeWorkspaceRuntimes(runtimes, watchers)
}
