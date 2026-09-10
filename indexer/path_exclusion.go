package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type pathInspectionFS struct {
	lstat        func(string) (os.FileInfo, error)
	readDir      func(string) ([]os.DirEntry, error)
	readSnapshot func(string, string) (*FileInfo, error)
}

// PathExclusionReason identifies an intentional scanner policy exclusion.
// An empty reason means the path is eligible or could not be classified safely.
type PathExclusionReason string

const (
	PathExcludedIgnored     PathExclusionReason = "ignored"
	PathExcludedUnsupported PathExclusionReason = "unsupported extension"
	PathExcludedMinified    PathExclusionReason = "minified"
	PathExcludedTooLarge    PathExclusionReason = "too large"
	PathExcludedBinary      PathExclusionReason = "binary"
	PathExcludedNonRegular  PathExclusionReason = "non-regular replacement"
)

// ExistingPathExclusion classifies a currently existing path using the same
// policies as scanning. Filesystem and read errors are returned so callers can
// preserve uncertain index entries rather than treating them as exclusions.
func (s *Scanner) ExistingPathExclusion(relPath string) (PathExclusionReason, error) {
	_, reason, err := s.InspectExistingPath(relPath)
	return reason, err
}

// InspectExistingPath returns either a fresh eligible snapshot or an
// authoritative exclusion. It uses Lstat so a replacement symlink or directory
// retires the old file's index ownership without reading through the link.
func (s *Scanner) InspectExistingPath(relPath string) (*FileInfo, PathExclusionReason, error) {
	return s.inspectExistingPathWith(relPath, pathInspectionFS{lstat: os.Lstat, readDir: os.ReadDir, readSnapshot: s.readSnapshot})
}

func (s *Scanner) inspectExistingPathWith(relPath string, filesystem pathInspectionFS) (*FileInfo, PathExclusionReason, error) {
	relPath = filepath.FromSlash(relPath)
	absPath := filepath.Join(s.root, relPath)
	info, err := filesystem.lstat(absPath)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, PathExcludedNonRegular, nil
	}
	actual, err := actualPathSpellingWithError(s.root, filepath.ToSlash(relPath), filesystem.readDir, make(map[string][]os.DirEntry))
	if err != nil {
		return nil, "", fmt.Errorf("resolve actual spelling of %s: %w", relPath, err)
	}
	relPath = filepath.FromSlash(actual)
	absPath = filepath.Join(s.root, relPath)
	actualInfo, err := filesystem.lstat(absPath)
	if err != nil {
		return nil, "", err
	}
	if !os.SameFile(info, actualInfo) {
		return nil, "", fmt.Errorf("path identity changed while resolving actual spelling of %s", relPath)
	}
	info = actualInfo
	if s.ignore.ShouldIgnore(relPath) {
		return nil, PathExcludedIgnored, nil
	}
	if !s.isSupported(strings.ToLower(filepath.Ext(relPath))) {
		return nil, PathExcludedUnsupported, nil
	}
	if isMinifiedFile(relPath) {
		return nil, PathExcludedMinified, nil
	}
	if info.Size() > maxFileSize {
		return nil, PathExcludedTooLarge, nil
	}
	snapshot, err := filesystem.readSnapshot(absPath, relPath)
	if err != nil {
		return nil, "", err
	}
	// A stable regular in-range file can only be rejected here as binary.
	current, err := filesystem.lstat(absPath)
	if err != nil {
		return nil, "", err
	}
	if !current.Mode().IsRegular() {
		return nil, PathExcludedNonRegular, nil
	}
	if current.Size() > maxFileSize {
		return nil, PathExcludedTooLarge, nil
	}
	if snapshot != nil {
		return snapshot, "", nil
	}
	return nil, PathExcludedBinary, nil
}
