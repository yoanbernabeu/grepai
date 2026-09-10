package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RevalidateCaseRenameWitness verifies a previously observed case-only rename
// against current directory spelling and filesystem identity.
func RevalidateCaseRenameWitness(root, candidate, witness string) (bool, error) {
	oldPath, ok := cleanRelativePath(candidate)
	if !ok {
		return false, fmt.Errorf("invalid candidate path %q", candidate)
	}
	newPath, ok := cleanRelativePath(witness)
	if !ok {
		return false, fmt.Errorf("invalid witness path %q", witness)
	}
	actual, err := actualPathSpellingWithError(root, oldPath, os.ReadDir, make(map[string][]os.DirEntry))
	if err != nil {
		return false, err
	}
	if actual == oldPath {
		return false, nil
	}
	if actual != newPath {
		return false, fmt.Errorf("case rename %q to %q changed to third spelling %q", oldPath, newPath, actual)
	}
	oldInfo, err := os.Stat(filepath.Join(root, filepath.FromSlash(oldPath)))
	if err != nil {
		return false, err
	}
	newInfo, err := os.Stat(filepath.Join(root, filepath.FromSlash(newPath)))
	if err != nil {
		return false, err
	}
	return os.SameFile(oldInfo, newInfo), nil
}

// CanRetireCaseAlias proves that witness is no longer a distinct directory
// entry: it is either absent or resolves to candidate's actual spelling and
// filesystem identity on a case-insensitive filesystem.
func CanRetireCaseAlias(root, candidate, witness string) (bool, error) {
	oldPath, ok := cleanRelativePath(candidate)
	if !ok {
		return false, fmt.Errorf("invalid candidate path %q", candidate)
	}
	newPath, ok := cleanRelativePath(witness)
	if !ok {
		return false, fmt.Errorf("invalid witness path %q", witness)
	}
	witnessAbsolute := filepath.Join(root, filepath.FromSlash(newPath))
	if _, err := os.Lstat(witnessAbsolute); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	actual, err := actualPathSpellingWithError(root, newPath, os.ReadDir, make(map[string][]os.DirEntry))
	if err != nil {
		return false, err
	}
	if actual == newPath {
		return false, nil
	}
	if actual != oldPath {
		return false, fmt.Errorf("case alias %q for %q changed to third spelling %q", newPath, oldPath, actual)
	}
	oldInfo, err := os.Stat(filepath.Join(root, filepath.FromSlash(oldPath)))
	if err != nil {
		return false, err
	}
	newInfo, err := os.Stat(witnessAbsolute)
	if err != nil {
		return false, err
	}
	return os.SameFile(oldInfo, newInfo), nil
}

type caseRenameFS struct {
	stat    func(string) (os.FileInfo, error)
	readDir func(string) ([]os.DirEntry, error)
}

// FindCaseRenameWitnesses identifies stale path spellings that resolve to a
// differently spelled file observed by the current scan. Case folding only
// finds candidates; fresh identity and directory-entry checks authorize them.
func FindCaseRenameWitnesses(root string, candidates []string, scanned []FileMeta) map[string]string {
	return findCaseRenameWitnessesWith(root, candidates, scanned, caseRenameFS{stat: os.Stat, readDir: os.ReadDir})
}

func findCaseRenameWitnessesWith(root string, candidates []string, scanned []FileMeta, filesystem caseRenameFS) map[string]string {
	type observedPath struct {
		path      string
		ambiguous bool
	}
	observed := make(map[string]observedPath, len(scanned))
	for _, file := range scanned {
		path, ok := cleanRelativePath(file.Path)
		if !ok {
			continue
		}
		key := casePathKey(path)
		if prior, exists := observed[key]; exists && prior.path != path {
			prior.ambiguous = true
			observed[key] = prior
		} else if !exists {
			observed[key] = observedPath{path: path}
		}
	}

	directoryCache := make(map[string][]os.DirEntry)
	witnesses := make(map[string]string)
	for _, candidate := range candidates {
		oldPath, ok := cleanRelativePath(candidate)
		if !ok {
			continue
		}
		witness, ok := observed[casePathKey(oldPath)]
		if !ok || witness.ambiguous || witness.path == oldPath {
			continue
		}
		actual, ok := actualPathSpelling(root, oldPath, filesystem.readDir, directoryCache)
		if !ok || actual == oldPath || actual != witness.path {
			continue
		}
		oldInfo, err := filesystem.stat(filepath.Join(root, filepath.FromSlash(oldPath)))
		if err != nil {
			continue
		}
		newInfo, err := filesystem.stat(filepath.Join(root, filepath.FromSlash(witness.path)))
		if err != nil || !os.SameFile(oldInfo, newInfo) {
			continue
		}
		witnesses[candidate] = witness.path
	}
	return witnesses
}

func cleanRelativePath(path string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(clean), true
}

func casePathKey(path string) string { return strings.ToLower(filepath.ToSlash(path)) }

func actualPathSpelling(root, relative string, readDir func(string) ([]os.DirEntry, error), cache map[string][]os.DirEntry) (string, bool) {
	actual, err := actualPathSpellingWithError(root, relative, readDir, cache)
	return actual, err == nil
}

func actualPathSpellingWithError(root, relative string, readDir func(string) ([]os.DirEntry, error), cache map[string][]os.DirEntry) (string, error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	parent := root
	actual := make([]string, 0, len(parts))
	for _, requested := range parts {
		entries, ok := cache[parent]
		if !ok {
			var err error
			entries, err = readDir(parent)
			if err != nil {
				return "", err
			}
			cache[parent] = entries
		}
		name, found := actualEntryName(entries, requested)
		if !found {
			return "", fmt.Errorf("cannot resolve actual spelling of %q", filepath.Join(parent, requested))
		}
		actual = append(actual, name)
		parent = filepath.Join(parent, name)
	}
	return strings.Join(actual, "/"), nil
}

func actualEntryName(entries []os.DirEntry, requested string) (string, bool) {
	for _, entry := range entries {
		if entry.Name() == requested {
			return requested, true
		}
	}
	match := ""
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), requested) {
			if match != "" {
				return "", false
			}
			match = entry.Name()
		}
	}
	return match, match != ""
}
