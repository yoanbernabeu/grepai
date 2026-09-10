package indexer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScanMetadataScope scans indexable files below a project-relative directory.
// Unlike the full-project scan, it reports traversal errors so policy
// reconciliation can preserve existing index state when the filesystem is
// unreadable.
func (s *Scanner) ScanMetadataScope(relRoot string) ([]FileMeta, []string, error) {
	relRoot = filepath.Clean(relRoot)
	if filepath.IsAbs(relRoot) || relRoot == ".." || strings.HasPrefix(relRoot, ".."+string(filepath.Separator)) {
		return nil, nil, fmt.Errorf("scan scope %s is outside project root", relRoot)
	}
	absRoot := filepath.Join(s.root, relRoot)
	info, err := os.Lstat(absRoot)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("scan scope %s is not a directory", relRoot)
	}
	return s.scanMetadata(absRoot, true, true)
}

func (s *Scanner) scanMetadata(root string, preserveErrors, regularOnly bool) ([]FileMeta, []string, error) {
	var files []FileMeta
	var skipped []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if preserveErrors {
				return err
			}
			return nil // Skip files we can't access
		}

		relPath, err := filepath.Rel(s.root, path)
		if err != nil {
			if preserveErrors {
				return err
			}
			return nil
		}

		// Handle directories: use ShouldSkipDir to respect .grepaiignore negations
		if d.IsDir() {
			if s.ignore.ShouldSkipDir(relPath) {
				return filepath.SkipDir
			}
			return nil // Descend into the directory
		}
		// Skip ignored files
		if s.ignore.ShouldIgnore(relPath) {
			return nil
		}

		// Check extension
		ext := strings.ToLower(filepath.Ext(path))
		if !s.isSupported(ext) {
			return nil
		}

		// Skip minified files
		if isMinifiedFile(relPath) {
			skipped = append(skipped, relPath+" (minified)")
			return nil
		}

		var info fs.FileInfo
		if regularOnly {
			info, err = d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
		}
		if info == nil {
			info, err = d.Info()
			if err != nil {
				if preserveErrors {
					return err
				}
				return nil
			}
		}

		// Skip large files
		if info.Size() > maxFileSize {
			skipped = append(skipped, relPath+" (too large)")
			return nil
		}

		files = append(files, FileMeta{
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})

		return nil
	})

	return files, skipped, err
}
