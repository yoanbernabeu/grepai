package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

type EventType int

const (
	EventCreate EventType = iota
	EventModify
	EventDelete
	EventRename
)

type FileEvent struct {
	Type EventType
	Path string
}

type Watcher struct {
	root       string
	watcher    *fsnotify.Watcher
	ignore     *indexer.IgnoreMatcher
	debounceMs int
	events     chan FileEvent
	done       chan struct{}

	// Debouncing state
	pending   map[string]FileEvent
	pendingMu sync.Mutex
	timer     *time.Timer
}

func NewWatcher(root string, ignore *indexer.IgnoreMatcher, debounceMs int) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		root:       root,
		watcher:    fsw,
		ignore:     ignore,
		debounceMs: debounceMs,
		events:     make(chan FileEvent, 100),
		done:       make(chan struct{}),
		pending:    make(map[string]FileEvent),
	}, nil
}

func (w *Watcher) Start(ctx context.Context) error {
	// Add root directory and all subdirectories
	if err := w.addRecursive(w.root); err != nil {
		return err
	}

	// Start event processing
	go w.processEvents(ctx)

	return nil
}

func (w *Watcher) Events() <-chan FileEvent {
	return w.events
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		relPath, err := filepath.Rel(w.root, path)
		if err != nil {
			return nil
		}

		// Handle directories: use ShouldSkipDir to respect .grepaiignore negations
		if info.IsDir() {
			if w.ignore.ShouldSkipDir(relPath) {
				return filepath.SkipDir
			}
			// Directory is not skipped; watch it if not individually ignored
			if !w.ignore.ShouldIgnore(relPath) {
				if err := w.watcher.Add(path); err != nil {
					log.Printf("Failed to watch %s: %v", path, err)
				}
			}
			return nil
		}

		// Skip ignored files
		if w.ignore.ShouldIgnore(relPath) {
			return nil
		}

		return nil
	})
}

func (w *Watcher) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	relPath, err := filepath.Rel(w.root, event.Name)
	if err != nil {
		return
	}

	// Ignore hidden files and ignored paths
	if strings.HasPrefix(filepath.Base(relPath), ".") {
		return
	}
	if w.ignore.ShouldIgnore(relPath) {
		return
	}

	// Check if it's a supported file
	ext := strings.ToLower(filepath.Ext(event.Name))
	if !indexer.SupportedExtensions[ext] {
		// Check if it's a directory (for watching new directories)
		info, err := os.Stat(event.Name)
		if err != nil || !info.IsDir() {
			return
		}

		// New directory created, add to watcher
		if event.Has(fsnotify.Create) {
			if err := w.addRecursive(event.Name); err != nil {
				log.Printf("Failed to add new directory %s: %v", event.Name, err)
			}
		}
		return
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
		return
	}

	w.debounceEvent(FileEvent{
		Type: evType,
		Path: relPath,
	})
}

func (w *Watcher) debounceEvent(event FileEvent) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	// Merge events: delete > create/modify
	existing, exists := w.pending[event.Path]
	if exists && existing.Type == EventDelete && event.Type != EventDelete {
		// Keep delete if file was deleted then recreated quickly
		// This will be handled as delete + create
	} else {
		w.pending[event.Path] = event
	}

	// Reset timer
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(time.Duration(w.debounceMs)*time.Millisecond, w.flush)
}

func (w *Watcher) flush() {
	w.pendingMu.Lock()
	events := make([]FileEvent, 0, len(w.pending))
	for _, event := range w.pending {
		events = append(events, event)
	}
	w.pending = make(map[string]FileEvent)
	w.pendingMu.Unlock()

	for _, event := range events {
		select {
		case w.events <- event:
		default:
			log.Printf("Event channel full, dropping event for %s", event.Path)
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
