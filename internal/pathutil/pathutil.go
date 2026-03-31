package pathutil

import (
	"os"
	"path/filepath"
)

// ResolveReal resolves a path to its canonical form. It tries
// filepath.EvalSymlinks first; if that fails (common with Windows
// junctions and reparse points), it verifies the path exists and
// falls back to filepath.Abs. Returns an error only if the path
// truly does not exist.
//
// Pattern based on golangci-lint PR #5245.
func ResolveReal(path string) (string, error) {
	// Try full symlink resolution first.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	}

	// EvalSymlinks failed — verify the path actually exists.
	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	// Path exists but EvalSymlinks couldn't resolve it.
	// Fall back to making it absolute.
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.Clean(abs), nil
}
