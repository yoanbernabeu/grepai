package cli

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/watcher"
)

type watchSource interface {
	Events() <-chan watcher.FileEvent
	Errors() <-chan error
	Ready(func() error) error
	Abort()
	Close() error
}

func closeUnlessAborted(ctx context.Context, aborted *bool, closeFn func() error) {
	if !*aborted && !isFatalWatcherError(context.Cause(ctx)) {
		_ = closeFn()
	}
}

func closeWithMutationFence(ctx context.Context, fence *watchMutationFence, aborted *bool, closeFn func() error) {
	if *aborted {
		return
	}
	fence.cleanup(ctx, func() {
		if !*aborted {
			_ = closeFn()
		}
	})
}

func abortWatcherReadiness(abortStores *bool, watchers []watchSource, scope string, err error, onFatal func()) error {
	*abortStores = true
	if onFatal != nil {
		onFatal()
	}
	abortWatchSources(watchers)
	if isFatalWatcherError(err) {
		return err
	}
	return &watcher.FatalError{Operation: "publish watcher readiness", Path: scope, Cause: err}
}

func publishWorkspaceReadiness(fence *watchMutationFence, watchers []watchSource, scope string, publish func() error, withdraw func(), abortStores, abortWatcherClose *bool) error {
	err := fence.ready(publish)
	if err == nil {
		return nil
	}
	*abortStores = true
	*abortWatcherClose = true
	fatalErr := err
	if !isFatalWatcherError(fatalErr) {
		fatalErr = &watcher.FatalError{Operation: "publish watcher readiness", Path: scope, Cause: err}
	}
	fence.failWithCause(fatalErr, func() { abortWatchSources(watchers) }, withdraw)
	return fmt.Errorf("failed to publish workspace readiness: %w", fatalErr)
}

func abortWatchSources(watchers []watchSource) {
	for _, w := range watchers {
		w.Abort()
	}
}

type workspaceWatcherError struct {
	ProjectName string
	ProjectPath string
	Cause       error
}

func (e *workspaceWatcherError) Error() string {
	return fmt.Sprintf("filesystem watcher failed for project %s (%s): %v", e.ProjectName, e.ProjectPath, e.Cause)
}

func (e *workspaceWatcherError) Unwrap() error { return e.Cause }

func isFatalWatcherError(err error) bool {
	var registrationErr *watcher.RegistrationError
	var fatalErr *watcher.FatalError
	return errors.As(err, &registrationErr) || errors.As(err, &fatalErr)
}

func forwardWorkspaceWatcher(ctx context.Context, runtime *workspaceProjectRuntime, events chan<- workspaceWatchEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-runtime.watcher.Events():
			select {
			case events <- workspaceWatchEvent{projectPath: runtime.project.Path, event: event}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func monitorWorkspaceWatcher(ctx context.Context, runtime *workspaceProjectRuntime, fence *watchMutationFence, abort, withdraw func(), fatals chan<- error) {
	select {
	case <-ctx.Done():
		return
	case err := <-runtime.watcher.Errors():
		fatal := &workspaceWatcherError{ProjectName: runtime.project.Name, ProjectPath: runtime.project.Path, Cause: err}
		fence.failWithCause(fatal, abort, withdraw)
		select {
		case fatals <- fatal:
		default:
		}
	}
}

type workspaceRuntimeInitializer func(context.Context, *config.Workspace, config.ProjectEntry, embedder.Embedder, store.VectorStore, bool) (*workspaceProjectRuntime, watchSource, error)

func initializeWorkspaceRuntimes(ctx context.Context, ws *config.Workspace, emb embedder.Embedder, sharedStore store.VectorStore, isBackgroundChild bool, initialize workspaceRuntimeInitializer) (map[string]*workspaceProjectRuntime, []watchSource, error) {
	runtimes := make(map[string]*workspaceProjectRuntime, len(ws.Projects))
	watchers := make([]watchSource, 0, len(ws.Projects))
	for _, project := range ws.Projects {
		if !isBackgroundChild {
			fmt.Printf("\nIndexing project: %s (%s)\n", project.Name, project.Path)
		} else {
			log.Printf("Indexing project: %s (%s)", project.Name, project.Path)
		}
		runtime, w, err := initialize(ctx, ws, project, emb, sharedStore, isBackgroundChild)
		if err != nil {
			var registrationErr *watcher.RegistrationError
			if errors.As(err, &registrationErr) {
				abortWatchSources(watchers)
				return nil, nil, fmt.Errorf("failed to initialize watcher for project %s (%s): %w", project.Name, project.Path, err)
			}
			if isRequiredSymbolStoreInitError(err) {
				abortWatchSources(watchers)
				return nil, nil, fmt.Errorf("failed to initialize required symbol store for project %s (%s): %w", project.Name, project.Path, err)
			}
			log.Printf("Warning: failed to initialize runtime for %s: %v", project.Name, err)
			continue
		}
		runtimes[canonicalPath(project.Path)] = runtime
		watchers = append(watchers, w)
	}
	return runtimes, watchers, nil
}

func closeWorkspaceRuntimes(runtimes map[string]*workspaceProjectRuntime, watchers []watchSource) {
	closeWatchSources(watchers)
	closeWorkspaceStores(runtimes)
}

func closeWatchSources(watchers []watchSource) {
	for _, w := range watchers {
		_ = w.Close()
	}
}

func withWatchSourcesReady(watchers []watchSource, publish func() error) error {
	if len(watchers) == 0 {
		return publish()
	}
	return watchers[0].Ready(func() error {
		return withWatchSourcesReady(watchers[1:], publish)
	})
}

func closeWorkspaceStores(runtimes map[string]*workspaceProjectRuntime) {
	for _, runtime := range runtimes {
		if runtime.symbolStore != nil {
			if err := runtime.symbolStore.Close(); err != nil {
				log.Printf("Warning: failed to close symbol store for %s: %v", runtime.project.Path, err)
			}
		}
		if runtime.rpgStore != nil {
			if err := runtime.rpgStore.Close(); err != nil {
				log.Printf("Warning: failed to close RPG store for %s: %v", runtime.project.Path, err)
			}
		}
	}
}

func persistWorkspaceStores(ctx context.Context, st store.VectorStore, runtimes map[string]*workspaceProjectRuntime) error {
	var errs []error
	if err := st.Persist(ctx); err != nil {
		errs = append(errs, fmt.Errorf("persist workspace vector index: %w", err))
	}
	for _, runtime := range runtimes {
		if err := runtime.symbolStore.Persist(ctx); err != nil {
			errs = append(errs, fmt.Errorf("persist symbol index for %s: %w", runtime.project.Name, err))
		}
		if runtime.rpgStore != nil {
			if err := runtime.rpgStore.Persist(ctx); err != nil {
				errs = append(errs, fmt.Errorf("persist RPG graph for %s: %w", runtime.project.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}
