package watcher

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

func (w *Watcher) releaseDirectory(path string) (bool, error) {
	w.directoriesMu.Lock()
	if _, ok := w.directories[path]; !ok {
		w.directoriesMu.Unlock()
		return false, nil
	}
	separator := string(filepath.Separator)
	trackedPaths := make([]string, 0)
	registeredPaths := make(map[string]bool)
	for tracked := range w.directories {
		if tracked == path || strings.HasPrefix(tracked, path+separator) {
			trackedPaths = append(trackedPaths, tracked)
			if _, registered := w.registered[tracked]; registered {
				registeredPaths[tracked] = true
			}
		}
	}
	w.directoriesMu.Unlock()

	sort.Slice(trackedPaths, func(i, j int) bool { return len(trackedPaths[i]) > len(trackedPaths[j]) })
	var removeErr error
	for _, tracked := range trackedPaths {
		if !registeredPaths[tracked] {
			continue
		}
		if err := w.removeWatch(tracked); err != nil && !w.benignRemoveWatchError(tracked, err) {
			removeErr = errors.Join(removeErr, fmt.Errorf("remove directory watch %s: %w", tracked, err))
		}
	}

	w.directoriesMu.Lock()
	for _, tracked := range trackedPaths {
		delete(w.directories, tracked)
		delete(w.registered, tracked)
	}
	w.directoriesMu.Unlock()
	return true, removeErr
}

func (w *Watcher) benignRemoveWatchError(path string, err error) bool {
	if errors.Is(err, fsnotify.ErrNonExistentWatch) || errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if runtime.GOOS != "linux" || !errors.Is(err, syscall.EINVAL) {
		return false
	}
	path = filepath.Clean(path)
	for _, watched := range w.watcher.WatchList() {
		if filepath.Clean(watched) == path {
			return false
		}
	}
	return true
}
