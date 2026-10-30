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
	if !strings.ContainsAny(nodePath, `/\:`) {
		// Bare command name: exec resolves it through PATH and refuses
		// results relative to the current directory (exec.ErrDot). A ':'
		// is excluded so that a Windows drive-relative value ("C:evil") is
		// treated as the relative path it is. Only Node.js names are
		// accepted ("node", "nodejs", "node20", "node.exe"): another
		// runtime could load project files from the current directory.
		if !strings.HasPrefix(strings.ToLower(nodePath), DefaultNodePath) {
			return "", fmt.Errorf("framework_processing.node_path %q is not a Node.js command: use \"node\" or an absolute path outside the project", nodePath)
		}
		return nodePath, nil
	}
	if isNetworkPath(nodePath) {
		// Windows UNC path: do not run, or even stat, a file on a network share.
		return "", fmt.Errorf("framework_processing.node_path %q is a network path: use \"node\" or a local absolute path outside the project", nodePath)
	}
	if !filepath.IsAbs(nodePath) {
		return "", fmt.Errorf("framework_processing.node_path %q is a relative path: use \"node\" or an absolute path outside the project", nodePath)
	}
	if isInsideDir(nodePath, projectRoot) {
		return "", fmt.Errorf("framework_processing.node_path %q points inside the project: use \"node\" or an absolute path outside the project", nodePath)
	}
	return nodePath, nil
}

// isNetworkPath reports whether path is a Windows UNC path (\\host\share or
// \\?\UNC\host\share). Local device paths such as \\?\C:\ are not.
func isNetworkPath(path string) bool {
	vol := strings.ToUpper(strings.ReplaceAll(filepath.VolumeName(path), "/", `\`))
	if !strings.HasPrefix(vol, `\\`) {
		return false
	}
	if strings.HasPrefix(vol, `\\?\UNC\`) || strings.HasPrefix(vol, `\\.\UNC\`) {
		return true
	}
	return !strings.HasPrefix(vol, `\\?\`) && !strings.HasPrefix(vol, `\\.\`)
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
			if rel, err := filepath.Rel(d, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
