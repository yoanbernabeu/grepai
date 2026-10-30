package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultNodePath is the Node.js command used when node_path is not set.
const DefaultNodePath = "node"

// ValidateNodePath checks a framework_processing.node_path value before
// grepai executes it.
//
// The value comes from the project's .grepai/config.yaml, which can ship
// with a cloned repository. A path into the project would let that
// repository make grepai run a file it provides. Allowed values are a bare
// command name resolved through PATH (such as "node") and an absolute path
// outside projectRoot. Relative paths and paths that resolve inside
// projectRoot, including through symlinks, are rejected.
func ValidateNodePath(nodePath, projectRoot string) (string, error) {
	if nodePath == "" {
		return DefaultNodePath, nil
	}
	if !strings.ContainsAny(nodePath, `/\`) {
		// Bare command name: exec resolves it through PATH and refuses
		// results relative to the current directory (exec.ErrDot).
		return nodePath, nil
	}
	if !filepath.IsAbs(nodePath) {
		return "", fmt.Errorf("framework_processing.node_path %q is a relative path: use \"node\" or an absolute path outside the project", nodePath)
	}
	if isInsideDir(nodePath, projectRoot) {
		return "", fmt.Errorf("framework_processing.node_path %q points inside the project: use \"node\" or an absolute path outside the project", nodePath)
	}
	return nodePath, nil
}

// isInsideDir reports whether path is dir or below it, comparing both the
// cleaned paths and, when they exist, the files they resolve to. Comparing
// with os.SameFile also covers symlinks and case-insensitive file systems.
func isInsideDir(path, dir string) bool {
	if dir == "" {
		return false
	}
	candidates := []string{filepath.Clean(path)}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, resolved)
	}
	dirs := []string{filepath.Clean(dir)}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dirs = append(dirs, resolved)
	}
	dirInfo, dirErr := os.Stat(dir)

	for _, candidate := range candidates {
		for _, d := range dirs {
			if rel, err := filepath.Rel(d, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return true
			}
		}
		if dirErr != nil {
			continue
		}
		for p := candidate; ; {
			if info, err := os.Stat(p); err == nil && os.SameFile(info, dirInfo) {
				return true
			}
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p = parent
		}
	}
	return false
}
