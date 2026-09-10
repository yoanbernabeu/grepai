package watcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// addRecursive walks the tree rooted at root and registers an fsnotify watch
// on every directory that cannot be skipped. It uses filepath.WalkDir so large
// repositories do not incur an extra Lstat syscall per file during startup.
// rootRequired reports whether the walk root itself must exist and register;
// registration failures fail closed with a RegistrationError.
func (w *Watcher) addRecursive(root string, required ...bool) error {
	rootRequired := len(required) > 0 && required[0]
	return w.addRecursiveWithFiles(root, rootRequired, false)
}

func (w *Watcher) addRecursiveWithFiles(root string, rootRequired, emitFiles bool) error {
	if emitFiles {
		// A moved-in tree may bring nested ignore files. Load all of them before
		// considering descendants because ScanFile does not reapply ignores.
		relRoot, err := filepath.Rel(w.root, root)
		if err != nil {
			return fmt.Errorf("resolve ignore refresh subtree: %w", err)
		}
		if err := w.ignore.RefreshSubtree(relRoot); err != nil {
			return fmt.Errorf("refresh ignore files: %w", err)
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && (!rootRequired || filepath.Clean(path) != filepath.Clean(root)) {
				return nil
			}
			return &RegistrationError{Operation: "walk watch tree", Path: path, Cause: err}
		}

		relPath, err := w.relPath(w.root, path)
		if err != nil {
			return &RegistrationError{Operation: "resolve watch path", Path: path, Cause: err}
		}
		if d.IsDir() {
			if w.ignore.ShouldSkipDir(relPath) {
				return filepath.SkipDir
			}
			// An ignored directory may contain a negated child, so every directory
			// that must be traversed also needs a watch.
			addErr := w.addWatch(path)
			w.directoriesMu.Lock()
			// A failed child Add still has directory identity from this walk and
			// may later be reported removed by its watched parent. Keep failed root
			// registration behavior unchanged; it has no watched parent.
			if addErr == nil || filepath.Clean(path) != filepath.Clean(w.root) {
				w.directories[path] = struct{}{}
			}
			if addErr == nil {
				w.registered[path] = struct{}{}
			}
			w.directoriesMu.Unlock()
			if addErr != nil {
				if os.IsNotExist(addErr) && (!rootRequired || filepath.Clean(path) != filepath.Clean(root)) {
					return nil
				}
				return &RegistrationError{Operation: "add watch", Path: path, Cause: addErr}
			}
			return nil
		}

		if w.ignore.ShouldIgnore(relPath) {
			return nil
		}
		if emitFiles && w.supportsFile(path) {
			w.debounceEvent(FileEvent{Type: EventCreate, Path: relPath})
		}
		return nil
	})
}
