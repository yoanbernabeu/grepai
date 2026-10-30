package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultNodePath is the Node.js command used when node_path is not set.
const DefaultNodePath = "node"

// nodeCommandName matches the Node.js command names accepted without a path:
// node, nodejs, node20, node.exe...
var nodeCommandName = regexp.MustCompile(`(?i)^node(js)?[0-9]*(\.exe)?$`)

// localDeviceVolume matches Windows local device prefixes such as \\?\C:.
var localDeviceVolume = regexp.MustCompile(`^\\\\[?.]\\[A-Za-z]:$`)

// ValidateNodePath checks a framework_processing.node_path value before
// grepai executes it.
//
// The value comes from the project's .grepai/config.yaml, which can ship
// with a cloned repository. A path into that repository would let it make
// grepai run a file it provides. Allowed values are a Node.js command name
// resolved through PATH (such as "node") and a local absolute path outside
// every directory in roots (the project root, and the enclosing git
// repository when known). Relative paths, network paths, other command names
// and paths that resolve inside one of the roots, including through
// symlinks, are rejected.
func ValidateNodePath(nodePath string, roots ...string) (string, error) {
	if nodePath == "" {
		return DefaultNodePath, nil
	}
	if !strings.ContainsAny(nodePath, `/\:`) {
		// Bare command name: exec resolves it through PATH and refuses
		// results relative to the current directory (exec.ErrDot). A ':'
		// is excluded so that a Windows drive-relative value ("C:evil") is
		// treated as the relative path it is. Only Node.js names are
		// accepted ("node", "nodejs", "node20", "node.exe"): another
		// program (bun, nodemon...) could load project files from the
		// current directory.
		if !nodeCommandName.MatchString(nodePath) {
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
	if clean := filepath.ToSlash(filepath.Clean(nodePath)); strings.HasPrefix(clean, "/proc/") || strings.HasPrefix(clean, "/dev/") {
		// /proc/self/cwd, /proc/self/fd/N, /dev/fd/N: their target depends on
		// the process state when node is started, not when it is checked.
		return "", fmt.Errorf("framework_processing.node_path %q depends on the process state: use \"node\" or a regular absolute path outside the project", nodePath)
	}
	if _, err := filepath.EvalSymlinks(nodePath); err != nil {
		return "", fmt.Errorf("framework_processing.node_path %q cannot be resolved (%v): use \"node\" or an existing absolute path outside the project", nodePath, err)
	}
	for _, root := range roots {
		if isInsideDir(nodePath, root) {
			return "", fmt.Errorf("framework_processing.node_path %q points inside the project or its repository: use \"node\" or an absolute path outside the project", nodePath)
		}
	}
	return nodePath, nil
}

// isNetworkPath reports whether path has a Windows UNC or device volume
// (\\host\share, \\?\UNC\host\share, \\?\GLOBALROOT...). Only the local drive
// device forms \\?\C: and \\.\C: are treated as local.
func isNetworkPath(path string) bool {
	vol := strings.ReplaceAll(filepath.VolumeName(path), "/", `\`)
	if !strings.HasPrefix(vol, `\\`) && !strings.HasPrefix(vol, `\??\`) {
		return false
	}
	return !localDeviceVolume.MatchString(vol)
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

// RepositoryRoots returns the working tree root of the git repository that
// contains dir, plus the main worktree root when dir is in a linked worktree.
// It only looks for a ".git" entry in dir and its parents, without running
// git, so it also works when git is missing or refuses the repository.
func RepositoryRoots(dir string) []string {
	for d := filepath.Clean(dir); ; {
		gitPath := filepath.Join(d, ".git")
		if info, err := os.Lstat(gitPath); err == nil {
			roots := []string{d}
			if info.Mode().IsRegular() {
				if main := mainWorktreeFromGitFile(gitPath); main != "" && main != d {
					roots = append(roots, main)
				}
			}
			return roots
		}
		parent := filepath.Dir(d)
		if parent == d {
			return nil
		}
		d = parent
	}
}

// mainWorktreeFromGitFile reads a linked worktree's ".git" file
// ("gitdir: <main>/.git/worktrees/<name>") and returns <main>.
func mainWorktreeFromGitFile(gitFile string) string {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(gitFile), gitDir)
	}
	sep := string(filepath.Separator)
	marker := sep + ".git" + sep + "worktrees" + sep
	if i := strings.LastIndex(filepath.Clean(gitDir), marker); i > 0 {
		return filepath.Clean(gitDir)[:i]
	}
	return ""
}
