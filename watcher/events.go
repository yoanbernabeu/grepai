package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func (w *Watcher) processEvents(ctx context.Context) {
	defer close(w.processingDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case event, ok := <-w.backendEvents:
			if !ok {
				if ctx.Err() == nil && !w.stopped() {
					w.publishFatal(&FatalError{Operation: "filesystem event channel closed", Path: w.root, Cause: errBackendClosed})
				}
				return
			}
			if err := w.handleEvent(event); err != nil {
				w.publishFatal(err)
				return
			}
		case err, ok := <-w.backendErrors:
			if !ok {
				if ctx.Err() == nil && !w.stopped() {
					w.publishFatal(&FatalError{Operation: "filesystem error channel closed", Path: w.root, Cause: errBackendClosed})
				}
				return
			}
			w.publishFatal(&FatalError{Operation: "process filesystem events", Path: w.root, Cause: err})
			return
		}
	}
}

func (w *Watcher) publishFatal(err error) {
	w.fatalOnce.Do(func() {
		w.stateMu.Lock()
		if w.ownerStopped {
			w.stateMu.Unlock()
			return
		}
		w.fatalErr = err
		select {
		case w.errors <- err:
		default:
		}
		w.stateMu.Unlock()
		w.Abort()
	})
}

func (w *Watcher) handleEvent(event fsnotify.Event) error {
	relPath, err := w.relPath(w.root, event.Name)
	if err != nil {
		return &FatalError{Operation: "resolve filesystem event path", Path: event.Name, Cause: err}
	}
	if relPath == "." && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
		return &FatalError{Operation: "watch root", Path: w.root, Cause: errWatchRootLost}
	}

	// Ignore files affect later siblings and imported descendants. Reload them
	// as data before applying the hidden-path filter.
	base := filepath.Base(relPath)
	if base == ".gitignore" || base == ".grepaiignore" {
		scope := filepath.Dir(relPath)
		if err := w.refreshIgnore(scope); err != nil {
			return &FatalError{Operation: "refresh ignore policy", Path: relPath, Cause: err}
		}
		root := w.root
		if scope != "." {
			root = filepath.Join(w.root, scope)
		}
		if err := w.addRecursive(root, false); err != nil {
			return &FatalError{Operation: "register refreshed ignore scope", Path: root, Cause: err}
		}
		w.debounceEvent(FileEvent{Type: EventReconcile, Path: scope, IsDir: true})
		return nil
	}

	if event.Has(fsnotify.Create) {
		info, err := w.statPath(event.Name)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return &FatalError{Operation: "stat created path", Path: event.Name, Cause: err}
		}
		if info.IsDir() {
			if err := w.addRecursiveWithFiles(event.Name, false, true); err != nil {
				return &FatalError{Operation: "register new directory", Path: event.Name, Cause: err}
			}
			return nil
		}
	}

	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		isDir, err := w.releaseDirectory(event.Name)
		if isDir {
			eventType := EventDelete
			if event.Has(fsnotify.Rename) {
				eventType = EventRename
			}
			w.debounceEvent(FileEvent{Type: eventType, Path: relPath, IsDir: true})
			if err != nil {
				return &FatalError{Operation: "release directory watches", Path: event.Name, Cause: err}
			}
			return nil
		}
		if err != nil {
			return &FatalError{Operation: "release directory watches", Path: event.Name, Cause: err}
		}
	}

	// Hidden files are not indexed, but indexable dot-directories must reach the
	// directory lifecycle above. Configured metadata directories are rejected by
	// ShouldSkipDir during recursive registration.
	if strings.HasPrefix(base, ".") || w.ignore.ShouldIgnore(relPath) || !w.supportsFile(event.Name) {
		return nil
	}

	var eventType EventType
	switch {
	case event.Has(fsnotify.Create):
		eventType = EventCreate
	case event.Has(fsnotify.Write):
		eventType = EventModify
	case event.Has(fsnotify.Remove):
		eventType = EventDelete
	case event.Has(fsnotify.Rename):
		eventType = EventRename
	default:
		return nil
	}
	w.debounceEvent(FileEvent{Type: eventType, Path: relPath})
	return nil
}

func (e EventType) String() string {
	switch e {
	case EventCreate:
		return "CREATE"
	case EventModify:
		return "MODIFY"
	case EventDelete:
		return "DELETE"
	case EventRename:
		return "RENAME"
	case EventReconcile:
		return "RECONCILE"
	default:
		return fmt.Sprintf("EventType(%d)", int(e))
	}
}
