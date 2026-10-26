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
)

type FileEvent struct {
	Type EventType
	Path string
	// Dir is set on a delete or rename of a watched directory: every indexed
	// file under Path is gone from that location.
	Dir bool
}

type Watcher struct {
	root          string
	watcher       *fsnotify.Watcher
	addWatch      func(string) error
	statPath      func(string) (fs.FileInfo, error)
	relPath       func(string, string) (string, error)
	backendEvents <-chan fsnotify.Event
	backendErrors <-chan error
	ignore        *indexer.IgnoreMatcher
	isSupported   func(ext string) bool
	debounceMs    int
	events        chan FileEvent
	errors        chan error
	done          chan struct{}
	stopOnce      sync.Once
	closeOnce     sync.Once
	closeErr      error
	stateMu       sync.Mutex
	ownerStopped  bool
	fatalErr      error
	fatalOnce     sync.Once

	processingDone chan struct{}

	// watchedDirs holds the slash-separated relative paths of the watched
	// directories, so the removal of a directory can be told apart from the
	// removal of a file that no longer exists to be checked.
	watchedDirs   map[string]bool
	watchedDirsMu sync.Mutex

	// Debouncing state
	pending   map[string]FileEvent
	pendingMu sync.Mutex
	// flushMu serializes flushes so events from an older batch are never
	// delivered after events from a newer one while the queue is full.
	flushMu sync.Mutex
	timer   *time.Timer
}

func NewWatcher(root string, ignore *indexer.IgnoreMatcher, debounceMs int) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, &RegistrationError{Operation: "create filesystem watcher", Path: root, Cause: err}
	}

	w := &Watcher{
		root:        root,
		watcher:     fsw,
		ignore:      ignore,
		debounceMs:  debounceMs,
		events:      make(chan FileEvent, 100),
		errors:      make(chan error, 1),
		done:        make(chan struct{}),
		pending:     make(map[string]FileEvent),
		watchedDirs: make(map[string]bool),
	}
	w.isSupported = func(ext string) bool { return indexer.SupportedExtensions[ext] }
	w.addWatch = fsw.Add
	w.statPath = os.Stat
	w.relPath = filepath.Rel
	w.backendEvents = fsw.Events
	w.backendErrors = fsw.Errors
	return w, nil
}

// WithSupportedExtensions sets which lowercase file extensions produce events,
// so the watcher follows the scanner's custom extensions. It must be called
// before Start.
func (w *Watcher) WithSupportedExtensions(isSupported func(ext string) bool) *Watcher {
	w.isSupported = isSupported
	return w
}

func (w *Watcher) Start(ctx context.Context) error {
	// Add root directory and all subdirectories
	if err := w.addRecursive(w.root, true, nil); err != nil {
		w.Abort()
		return err
	}

	// Start event processing
	w.processingDone = make(chan struct{})
	go w.processEvents(ctx)

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

// addRecursive walks the tree rooted at root and registers an fsnotify watch
// on every directory that isn't ignored. It uses filepath.WalkDir (not
// filepath.Walk) so that directory entries are read directly from the
// readdir results instead of an extra Lstat syscall per file -- on repos with
// 100k+ files this roughly halves the syscall count of the initial/restart
// tree walk, which matters because watch startup blocks on this before it
// starts serving fsnotify events.
func (w *Watcher) addRecursive(root string, rootRequired bool, onFile func(relPath string)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && (!rootRequired || path != root) {
				return nil
			}
			return &RegistrationError{Operation: "walk watch tree", Path: path, Cause: err}
		}

		relPath, err := w.relPath(w.root, path)
		if err != nil {
			return &RegistrationError{Operation: "resolve watch path", Path: path, Cause: err}
		}
		relPath = filepath.ToSlash(relPath)

		// Handle directories: use ShouldSkipDir to respect .grepaiignore negations
		if d.IsDir() {
			if w.ignore.ShouldSkipDir(relPath) {
				return filepath.SkipDir
			}
			// Directory is not skipped; watch it if not individually ignored
			if !w.ignore.ShouldIgnore(relPath) {
				if err := w.addWatch(path); err != nil {
					if os.IsNotExist(err) && (!rootRequired || path != root) {
						return nil
					}
					return &RegistrationError{Operation: "add watch", Path: path, Cause: err}
				}
				w.watchedDirsMu.Lock()
				w.watchedDirs[relPath] = true
				w.watchedDirsMu.Unlock()
			}
			return nil
		}

		// Skip ignored files
		if w.ignore.ShouldIgnore(relPath) {
			return nil
		}
		if onFile != nil && w.isSupported(strings.ToLower(filepath.Ext(relPath))) {
			onFile(relPath)
		}
		return nil
	})
}

// forgetWatchedDir reports whether relPath was a watched directory and drops
// it and its subdirectories from the set.
func (w *Watcher) forgetWatchedDir(relPath string) bool {
	w.watchedDirsMu.Lock()
	defer w.watchedDirsMu.Unlock()
	if relPath == "" || relPath == "." || !w.watchedDirs[relPath] {
		return false
	}
	prefix := relPath + "/"
	for dir := range w.watchedDirs {
		if dir == relPath || strings.HasPrefix(dir, prefix) {
			delete(w.watchedDirs, dir)
		}
	}
	return true
}
