package fileutil

import (
	"os"
	"path/filepath"
)

// EnsureParentDir creates parent directories for the given path if they do not exist.
func EnsureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

// ReplaceFileAtomically renames tempPath to targetPath. On systems where
// cross-device rename fails, it falls back to remove-then-rename.
func ReplaceFileAtomically(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return os.Rename(tempPath, targetPath)
}
