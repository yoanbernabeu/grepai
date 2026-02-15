package cli

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/indexer"
)

type watchLifecycleEvent struct {
	projectRoot string
	state       string
	note        string
}

func waitForWatchLifecycleState(t *testing.T, ch <-chan watchLifecycleEvent, projectRoot, state string, timeout time.Duration) watchLifecycleEvent {
	t.Helper()
	wantProjectRoot := canonicalPath(projectRoot)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev := <-ch:
			if canonicalPath(ev.projectRoot) == wantProjectRoot && ev.state == state {
				return ev
			}
		case <-deadline.C:
			t.Fatalf("timeout waiting for lifecycle state project=%s state=%s", projectRoot, state)
		}
	}
}

func waitForWatchScopeValue(t *testing.T, ch <-chan int, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case got := <-ch:
			if got == want {
				return
			}
		case <-deadline.C:
			t.Fatalf("timeout waiting for scope=%d", want)
		}
	}
}

func steadyWatchSessionRunner(
	ctx context.Context,
	_ string,
	_ embedder.Embedder,
	_ bool,
	onReady func(),
	_ watchSessionEventObserver,
	_ func(current, total int, file string),
	_ func(info indexer.BatchProgressInfo),
	_ func(step string, current, total int),
	_ watchActivityObserver,
	_ watchStatsObserver,
) error {
	onReady()
	<-ctx.Done()
	return ctx.Err()
}

func TestDynamicWatch_AddLinkedWorktreeAfterStart(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-a")

	var mu sync.Mutex
	linked := []string{}
	discover := func(string) []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), linked...)
	}

	lifecycleCh := make(chan watchLifecycleEvent, 128)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDynamicWatchSupervisor(
			ctx,
			mainRoot,
			nil,
			withWatchSupervisorSessionRunner(steadyWatchSessionRunner),
			withWatchSupervisorDiscoverWorktrees(discover),
			withWatchSupervisorReconcileInterval(20*time.Millisecond),
			withWatchSupervisorLifecycleObserver(func(projectRoot, state, note string) {
				lifecycleCh <- watchLifecycleEvent{projectRoot: projectRoot, state: state, note: note}
			}),
		)
	}()

	waitForWatchLifecycleState(t, lifecycleCh, mainRoot, "running", time.Second)

	mu.Lock()
	linked = []string{linkedRoot}
	mu.Unlock()

	waitForWatchLifecycleState(t, lifecycleCh, linkedRoot, "running", 2*time.Second)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runDynamicWatchSupervisor() error: %v", err)
	}
}

func TestDynamicWatch_RemoveLinkedWorktreeDuringRun(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-b")

	var mu sync.Mutex
	linked := []string{linkedRoot}
	discover := func(string) []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), linked...)
	}

	linkedStoppedCh := make(chan struct{}, 1)
	runner := func(
		ctx context.Context,
		projectRoot string,
		_ embedder.Embedder,
		_ bool,
		onReady func(),
		_ watchSessionEventObserver,
		_ func(current, total int, file string),
		_ func(info indexer.BatchProgressInfo),
		_ func(step string, current, total int),
		_ watchActivityObserver,
		_ watchStatsObserver,
	) error {
		onReady()
		<-ctx.Done()
		if projectRoot == linkedRoot {
			linkedStoppedCh <- struct{}{}
		}
		return ctx.Err()
	}

	lifecycleCh := make(chan watchLifecycleEvent, 128)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDynamicWatchSupervisor(
			ctx,
			mainRoot,
			nil,
			withWatchSupervisorSessionRunner(runner),
			withWatchSupervisorDiscoverWorktrees(discover),
			withWatchSupervisorInitialLinkedWorktrees([]string{linkedRoot}),
			withWatchSupervisorReconcileInterval(20*time.Millisecond),
			withWatchSupervisorLifecycleObserver(func(projectRoot, state, note string) {
				lifecycleCh <- watchLifecycleEvent{projectRoot: projectRoot, state: state, note: note}
			}),
		)
	}()

	waitForWatchLifecycleState(t, lifecycleCh, linkedRoot, "running", time.Second)

	mu.Lock()
	linked = nil
	mu.Unlock()

	waitForWatchLifecycleState(t, lifecycleCh, linkedRoot, "removed", 2*time.Second)

	select {
	case <-linkedStoppedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting linked session shutdown after remove")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runDynamicWatchSupervisor() error: %v", err)
	}
}

func TestDynamicWatch_RemoveAllLinkedWorktrees(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedA := canonicalPath("/tmp/wt-c")
	linkedB := canonicalPath("/tmp/wt-d")

	var mu sync.Mutex
	linked := []string{linkedA, linkedB}
	discover := func(string) []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), linked...)
	}

	scopeCh := make(chan int, 32)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDynamicWatchSupervisor(
			ctx,
			mainRoot,
			nil,
			withWatchSupervisorSessionRunner(steadyWatchSessionRunner),
			withWatchSupervisorDiscoverWorktrees(discover),
			withWatchSupervisorInitialLinkedWorktrees([]string{linkedA, linkedB}),
			withWatchSupervisorReconcileInterval(20*time.Millisecond),
			withWatchSupervisorScopeObserver(func(totalProjects int) {
				scopeCh <- totalProjects
			}),
		)
	}()

	waitForWatchScopeValue(t, scopeCh, 3, time.Second)

	mu.Lock()
	linked = nil
	mu.Unlock()

	waitForWatchScopeValue(t, scopeCh, 1, 2*time.Second)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runDynamicWatchSupervisor() error: %v", err)
	}
}

func TestDynamicWatch_LinkedFailureIsIsolatedWithRetry(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-e")

	discover := func(string) []string {
		return []string{linkedRoot}
	}

	var mu sync.Mutex
	attempts := 0
	runner := func(
		ctx context.Context,
		projectRoot string,
		_ embedder.Embedder,
		_ bool,
		onReady func(),
		_ watchSessionEventObserver,
		_ func(current, total int, file string),
		_ func(info indexer.BatchProgressInfo),
		_ func(step string, current, total int),
		_ watchActivityObserver,
		_ watchStatsObserver,
	) error {
		if projectRoot == mainRoot {
			onReady()
			<-ctx.Done()
			return ctx.Err()
		}

		mu.Lock()
		attempts++
		attempt := attempts
		mu.Unlock()

		if attempt == 1 {
			return errors.New("linked boom")
		}

		onReady()
		<-ctx.Done()
		return ctx.Err()
	}

	lifecycleCh := make(chan watchLifecycleEvent, 128)
	initialReadyCh := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDynamicWatchSupervisor(
			ctx,
			mainRoot,
			nil,
			withWatchSupervisorSessionRunner(runner),
			withWatchSupervisorDiscoverWorktrees(discover),
			withWatchSupervisorInitialLinkedWorktrees([]string{linkedRoot}),
			withWatchSupervisorReconcileInterval(20*time.Millisecond),
			withWatchSupervisorRetryBackoff(func(attempt int) time.Duration {
				return time.Duration(attempt) * 20 * time.Millisecond
			}),
			withWatchSupervisorInitialReadyObserver(func(totalProjects int) {
				initialReadyCh <- totalProjects
			}),
			withWatchSupervisorLifecycleObserver(func(projectRoot, state, note string) {
				lifecycleCh <- watchLifecycleEvent{projectRoot: projectRoot, state: state, note: note}
			}),
		)
	}()

	type lifecycleKey struct {
		projectRoot string
		state       string
	}
	pending := map[lifecycleKey]bool{
		{projectRoot: canonicalPath(mainRoot), state: "running"}:    true,
		{projectRoot: canonicalPath(linkedRoot), state: "error"}:    true,
		{projectRoot: canonicalPath(linkedRoot), state: "retrying"}: true,
		{projectRoot: canonicalPath(linkedRoot), state: "running"}:  true,
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for len(pending) > 0 {
		select {
		case ev := <-lifecycleCh:
			delete(pending, lifecycleKey{
				projectRoot: canonicalPath(ev.projectRoot),
				state:       ev.state,
			})
		case <-deadline.C:
			t.Fatalf("timeout waiting lifecycle states, missing=%v", pending)
		}
	}
	waitForWatchScopeValue(t, initialReadyCh, 2, 2*time.Second)

	select {
	case err := <-errCh:
		t.Fatalf("supervisor stopped unexpectedly: %v", err)
	default:
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runDynamicWatchSupervisor() error: %v", err)
	}
}

func TestDynamicWatch_MainFailureStopsAll(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-f")
	linkedStoppedCh := make(chan struct{}, 1)

	runner := func(
		ctx context.Context,
		projectRoot string,
		_ embedder.Embedder,
		_ bool,
		onReady func(),
		_ watchSessionEventObserver,
		_ func(current, total int, file string),
		_ func(info indexer.BatchProgressInfo),
		_ func(step string, current, total int),
		_ watchActivityObserver,
		_ watchStatsObserver,
	) error {
		if projectRoot == mainRoot {
			onReady()
			return errors.New("main session failed")
		}

		onReady()
		<-ctx.Done()
		linkedStoppedCh <- struct{}{}
		return ctx.Err()
	}

	err := runDynamicWatchSupervisor(
		context.Background(),
		mainRoot,
		nil,
		withWatchSupervisorSessionRunner(runner),
		withWatchSupervisorDiscoverWorktrees(func(string) []string {
			return []string{linkedRoot}
		}),
		withWatchSupervisorInitialLinkedWorktrees([]string{linkedRoot}),
		withWatchSupervisorReconcileInterval(20*time.Millisecond),
	)
	if err == nil {
		t.Fatal("expected main failure error")
	}
	if !strings.Contains(err.Error(), "main session failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-linkedStoppedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("linked session was not stopped after main failure")
	}
}

func TestDynamicWatch_InitialReadySelector_MainOnly(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-g")

	runner := func(
		ctx context.Context,
		projectRoot string,
		_ embedder.Embedder,
		_ bool,
		onReady func(),
		_ watchSessionEventObserver,
		_ func(current, total int, file string),
		_ func(info indexer.BatchProgressInfo),
		_ func(step string, current, total int),
		_ watchActivityObserver,
		_ watchStatsObserver,
	) error {
		if projectRoot == mainRoot {
			onReady()
			<-ctx.Done()
			return ctx.Err()
		}
		return errors.New("linked startup failure")
	}

	initialReadyCh := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDynamicWatchSupervisor(
			ctx,
			mainRoot,
			nil,
			withWatchSupervisorSessionRunner(runner),
			withWatchSupervisorDiscoverWorktrees(func(string) []string {
				return []string{linkedRoot}
			}),
			withWatchSupervisorInitialLinkedWorktrees([]string{linkedRoot}),
			withWatchSupervisorReconcileInterval(20*time.Millisecond),
			withWatchSupervisorRetryBackoff(func(int) time.Duration { return 20 * time.Millisecond }),
			withWatchSupervisorInitialReadySelector(func(main, project string) bool {
				return main == project
			}),
			withWatchSupervisorInitialReadyObserver(func(totalProjects int) {
				initialReadyCh <- totalProjects
			}),
		)
	}()

	waitForWatchScopeValue(t, initialReadyCh, 1, time.Second)

	select {
	case err := <-errCh:
		t.Fatalf("supervisor stopped unexpectedly: %v", err)
	default:
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runDynamicWatchSupervisor() error: %v", err)
	}
}

func TestDynamicWatch_ContextCancelRemainsGracefulUnderRace(t *testing.T) {
	mainRoot := canonicalPath("/tmp/main")
	linkedRoot := canonicalPath("/tmp/wt-h")

	for i := 0; i < 120; i++ {
		lifecycleCh := make(chan watchLifecycleEvent, 64)
		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)

		go func() {
			errCh <- runDynamicWatchSupervisor(
				ctx,
				mainRoot,
				nil,
				withWatchSupervisorSessionRunner(steadyWatchSessionRunner),
				withWatchSupervisorDiscoverWorktrees(func(string) []string {
					return []string{linkedRoot}
				}),
				withWatchSupervisorInitialLinkedWorktrees([]string{linkedRoot}),
				withWatchSupervisorReconcileInterval(20*time.Millisecond),
				withWatchSupervisorLifecycleObserver(func(projectRoot, state, note string) {
					lifecycleCh <- watchLifecycleEvent{projectRoot: projectRoot, state: state, note: note}
				}),
			)
		}()

		waitForWatchLifecycleState(t, lifecycleCh, mainRoot, "running", time.Second)

		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("iteration %d: expected graceful shutdown, got %v", i, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: timeout waiting supervisor shutdown", i)
		}
	}
}
