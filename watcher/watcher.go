package watcher

import (
	"context"
	"io/fs"
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
	EventReconcile
)

type FileEvent struct {
	Type EventType
	Path string
	// IsDir records directory identity before a removed path becomes unstatable.
	IsDir bool
}

type Watcher struct {
	root          string
	watcher       *fsnotify.Watcher
	backendEvents <-chan fsnotify.Event
	backendErrors <-chan error
	addWatch      func(string) error
	removeWatch   func(string) error
	statPath      func(string) (fs.FileInfo, error)
	relPath       func(string, string) (string, error)
	ignore        *indexer.IgnoreMatcher
	refreshIgnore func(string) error
	supportsFile  func(string) bool
	debounceMs    int
	events        chan FileEvent
	errors        chan error
	done          chan struct{}

	directories   map[string]struct{}
	registered    map[string]struct{}
	directoriesMu sync.Mutex

	// Debouncing state
	pending          map[string]FileEvent
	reconcilePending map[string]FileEvent
	pendingMu        sync.Mutex
	timer            *time.Timer
	flushReady       chan struct{}

	stopOnce       sync.Once
	closeOnce      sync.Once
	closeErr       error
	outputsOnce    sync.Once
	workers        sync.WaitGroup
	eventSenders   sync.WaitGroup
	processingDone chan struct{}

	stateMu      sync.Mutex
	ownerStopped bool
	fatalErr     error
	fatalOnce    sync.Once
}

// Option customizes watcher file selection.
type Option func(*Watcher)

// WithFileFilter sets the same path filter used by the scanner.
func WithFileFilter(filter func(string) bool) Option {
	return func(w *Watcher) {
		if filter != nil {
			w.supportsFile = filter
		}
	}
}

func NewWatcher(root string, ignore *indexer.IgnoreMatcher, debounceMs int, opts ...Option) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, &RegistrationError{Operation: "create filesystem watcher", Path: root, Cause: err}
	}

	w := &Watcher{
		root:    root,
		watcher: fsw,
		ignore:  ignore,
		supportsFile: func(path string) bool {
			return indexer.SupportedExtensions[strings.ToLower(filepath.Ext(path))]
		},
		debounceMs:       debounceMs,
		events:           make(chan FileEvent, 100),
		errors:           make(chan error, 1),
		done:             make(chan struct{}),
		pending:          make(map[string]FileEvent),
		reconcilePending: make(map[string]FileEvent),
		directories:      make(map[string]struct{}),
		registered:       make(map[string]struct{}),
		flushReady:       make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(w)
	}
	w.addWatch = fsw.Add
	w.removeWatch = fsw.Remove
	w.statPath = os.Stat
	w.relPath = filepath.Rel
	w.backendEvents = fsw.Events
	w.backendErrors = fsw.Errors
	w.refreshIgnore = func(scope string) error {
		if scope == "." {
			return ignore.Refresh()
		}
		return ignore.RefreshSubtree(scope)
	}
	return w, nil
}

func (w *Watcher) Start(ctx context.Context) error {
	// Add root directory and all subdirectories
	if err := w.addRecursive(w.root, true); err != nil {
		w.Abort()
		return err
	}

	// Keep backend draining independent from potentially backpressured delivery.
	w.processingDone = make(chan struct{})
	w.workers.Add(2)
	go func() {
		defer w.workers.Done()
		w.processEvents(ctx)
	}()
	go func() {
		defer w.workers.Done()
		w.processDelivery(ctx)
	}()

	return nil
}

func (w *Watcher) Events() <-chan FileEvent {
	return w.events
}

// Errors returns fatal errors that stop event processing.
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// Ready invokes publish while fatal publication is excluded.
func (w *Watcher) Ready(publish func() error) error {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.fatalErr != nil {
		return w.fatalErr
	}
	if w.ownerStopped {
		return errWatcherStopped
	}
	if publish == nil {
		return nil
	}
	return publish()
}

func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		w.Abort()
		w.closeErr = w.watcher.Close()
		w.workers.Wait()
		w.eventSenders.Wait()
		w.closeOutputs()
	})
	return w.closeErr
}

// Abort synchronously stops event ownership without closing the fsnotify
// backend. Fatal CLI paths rely on immediate process exit to reclaim its file
// descriptor; embedded callers may call Close after handling the fatal error.
func (w *Watcher) Abort() {
	w.stateMu.Lock()
	w.ownerStopped = true
	w.stateMu.Unlock()
	w.stop()
	w.pendingMu.Lock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.pendingMu.Unlock()
}

func (w *Watcher) stop() {
	w.stopOnce.Do(func() { close(w.done) })
}

func (w *Watcher) stopped() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

// closeOutputs closes the event and error channels after all senders finish.
func (w *Watcher) closeOutputs() {
	w.outputsOnce.Do(func() {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
		close(w.events)
		close(w.errors)
	})
}
