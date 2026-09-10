package cli

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

func TestWatchMutationWorkerQuiescesBeforeFatalWithdrawal(t *testing.T) {
	fence := newWatchMutationFence()
	fence.addWatcher(newFakeWatchSource())
	canceled := make(chan struct{})
	release := make(chan struct{})
	worker := startWatchMutationWorker(context.Background(), fence, func(ctx context.Context) {
		<-ctx.Done()
		close(canceled)
		<-release
	})
	awaitWatchTestSignal(t, worker.started, "worker admission")

	withdrawn := make(chan struct{})
	fatalDone := make(chan struct{})
	go func() {
		fence.fail(func() { close(withdrawn) })
		close(fatalDone)
	}()
	awaitWatchTestSignal(t, canceled, "worker cancellation")
	select {
	case <-withdrawn:
		t.Fatal("readiness withdrawn before worker quiesced")
	default:
	}
	select {
	case <-worker.done:
		t.Fatal("worker returned before its cooperative mutation was released")
	default:
	}
	close(release)
	awaitWatchTestSignal(t, worker.done, "worker return")
	awaitWatchTestSignal(t, withdrawn, "worker readiness withdrawal")
	awaitWatchTestSignal(t, fatalDone, "worker fatal completion")
}

type blockingPeriodicStore struct {
	mockVectorStore
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (s *blockingPeriodicStore) Persist(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	close(s.canceled)
	<-s.release
	return ctx.Err()
}

func TestProjectPeriodicPersistenceQuiescesBeforeFatalWithdrawal(t *testing.T) {
	fence := newWatchMutationFence()
	fence.addWatcher(newFakeWatchSource())
	st := &blockingPeriodicStore{started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	persistDone := make(chan error, 1)
	go func() {
		persistDone <- persistProjectPeriodically(context.Background(), fence, st, symbolStore, nil, "/project")
	}()
	awaitWatchTestSignal(t, st.started, "periodic persistence admission")

	withdrawn := make(chan struct{})
	go fence.fail(func() { close(withdrawn) })
	awaitWatchTestSignal(t, st.canceled, "periodic persistence cancellation")
	select {
	case <-withdrawn:
		t.Fatal("readiness withdrawn before periodic persistence quiesced")
	default:
	}
	close(st.release)
	if err := awaitWatchTestValue(t, persistDone, "periodic persistence return"); err != nil {
		t.Fatalf("persistProjectPeriodically() error = %v", err)
	}
	awaitWatchTestSignal(t, withdrawn, "periodic persistence withdrawal")
}

func TestWorkspacePeriodicPersistenceQuiescesBeforeFatalReturn(t *testing.T) {
	fence := newWatchMutationFence()
	fence.addWatcher(newFakeWatchSource())
	st := &blockingPeriodicStore{started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	persistTicks := make(chan time.Time, 1)
	fatals := make(chan error, 1)
	withdrawn := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- runWorkspaceWatchLoop(&workspaceWatchLoop{
			ctx: context.Background(), store: st, runtimes: map[string]*workspaceProjectRuntime{},
			fence: fence, persistTicks: persistTicks, fatals: fatals,
		})
	}()
	persistTicks <- time.Now()
	awaitWatchTestSignal(t, st.started, "workspace periodic persistence admission")
	fatal := &watcher.FatalError{Operation: "watch", Cause: syscall.ENOSPC}
	go func() {
		fence.failWithCause(fatal, nil, func() { close(withdrawn) })
		fatals <- fatal
	}()
	awaitWatchTestSignal(t, st.canceled, "workspace periodic persistence cancellation")
	select {
	case <-withdrawn:
		t.Fatal("workspace readiness withdrawn before periodic persistence quiesced")
	default:
	}
	select {
	case err := <-result:
		t.Fatalf("workspace loop returned before periodic persistence quiesced: %v", err)
	default:
	}
	close(st.release)
	awaitWatchTestSignal(t, withdrawn, "workspace periodic persistence withdrawal")
	if err := awaitWatchTestValue(t, result, "workspace periodic fatal return"); !errors.Is(err, fatal) {
		t.Fatalf("runWorkspaceWatchLoop() error = %v, want fatal", err)
	}
}

func TestWorkspaceWatchLoopFatalWaitsForEventMutation(t *testing.T) {
	root := t.TempDir()
	source := newFakeWatchSource()
	fence := newWatchMutationFence()
	fence.addWatcher(source)
	st := &cancellationBlockingMutationStore{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime := &workspaceProjectRuntime{
		projectIndexRuntime: &projectIndexRuntime{
			cfg:         config.DefaultConfig(),
			idx:         indexer.NewIndexer(root, st, nil, nil, nil, time.Time{}),
			symbolStore: trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob")),
			vectorStore: st,
		},
		project: config.ProjectEntry{Name: "api", Path: root},
		watcher: source,
	}
	runtimes := map[string]*workspaceProjectRuntime{canonicalPath(root): runtime}
	events := make(chan workspaceWatchEvent, 1)
	fatals := make(chan error, 1)
	withdrawn := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		monitorWorkspaceWatcher(context.Background(), runtime, fence, source.Abort, func() { close(withdrawn) }, fatals)
		close(monitorDone)
	}()
	result := make(chan error, 1)
	go func() {
		result <- runWorkspaceWatchLoop(&workspaceWatchLoop{
			ctx: context.Background(), store: st, runtimes: runtimes,
			watchers: []watchSource{source}, fence: fence, events: events, fatals: fatals,
			withdrawReadiness: func() { close(withdrawn) }, scope: "test",
		})
	}()
	events <- workspaceWatchEvent{projectPath: root, event: watcher.FileEvent{Type: watcher.EventDelete, Path: "obsolete.go"}}
	awaitWatchTestSignal(t, st.started, "workspace event mutation")
	fatal := &watcher.FatalError{Operation: "process filesystem events", Cause: syscall.ENOSPC}
	source.errors <- fatal
	awaitWatchTestSignal(t, st.canceled, "workspace event cancellation")
	select {
	case <-withdrawn:
		t.Fatal("workspace readiness withdrawn before event mutation quiesced")
	default:
	}
	select {
	case err := <-result:
		t.Fatalf("workspace loop returned before event mutation quiesced: %v", err)
	default:
	}
	close(st.release)
	err := awaitWatchTestValue(t, result, "workspace fatal return")
	if !errors.Is(err, fatal) {
		t.Fatalf("runWorkspaceWatchLoop() error = %v, want fatal", err)
	}
	awaitWatchTestSignal(t, withdrawn, "workspace readiness withdrawal")
	awaitWatchTestSignal(t, monitorDone, "workspace monitor return")
	if got := st.persists.Load(); got != 0 {
		t.Fatalf("fatal workspace path persisted %d times, want 0", got)
	}
}
