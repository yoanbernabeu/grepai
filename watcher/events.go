package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
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
	// Indexed paths are slash-separated on every OS (see Scanner); event
	// paths must match them, or deletes and renames miss on Windows.
	relPath = filepath.ToSlash(relPath)
	if relPath == "." && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
		return &FatalError{Operation: "watch root", Path: w.root, Cause: errWatchRootLost}
	}

	if strings.HasPrefix(filepath.Base(relPath), ".") || w.ignore.ShouldIgnore(relPath) {
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
			if err := w.addRecursive(event.Name, false); err != nil {
				return &FatalError{Operation: "register new directory", Path: event.Name, Cause: err}
			}
			return nil
		}
	}

	ext := strings.ToLower(filepath.Ext(event.Name))
	if !indexer.SupportedExtensions[ext] {
		return nil
	}

	var evType EventType
	switch {
	case event.Has(fsnotify.Create):
		evType = EventCreate
	case event.Has(fsnotify.Write):
		evType = EventModify
	case event.Has(fsnotify.Remove):
		evType = EventDelete
	case event.Has(fsnotify.Rename):
		evType = EventRename
	default:
		return nil
	}
	w.debounceEvent(FileEvent{Type: evType, Path: relPath})
	return nil
}

func (w *Watcher) debounceEvent(event FileEvent) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	if w.stopped() {
		return
	}
	existing, exists := w.pending[event.Path]
	if !exists || existing.Type != EventDelete || event.Type == EventDelete {
		w.pending[event.Path] = event
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(time.Duration(w.debounceMs)*time.Millisecond, w.flush)
}

func (w *Watcher) flush() {
	w.flushMu.Lock()
	defer w.flushMu.Unlock()

	w.pendingMu.Lock()
	events := make([]FileEvent, 0, len(w.pending))
	for _, event := range w.pending {
		events = append(events, event)
	}
	w.pending = make(map[string]FileEvent)
	w.pendingMu.Unlock()

	for _, event := range events {
		select {
		case <-w.done:
			return
		case w.events <- event:
		}
	}
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
	default:
		return "UNKNOWN"
	}
}
