// Package git provides utilities for detecting git worktree information.
package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DetectInfo holds git worktree detection results.
type DetectInfo struct {
	GitRoot      string // Worktree root from: git rev-parse --show-toplevel
	GitCommonDir string // Shared .git directory (absolute): git rev-parse --git-common-dir
	IsWorktree   bool   // true if this is a linked worktree (not the main one)
	MainWorktree string // Path to main worktree (parent of GitCommonDir)
	WorktreeID   string // Stable ID: hex(sha256(GitCommonDir))[:12]
}

// Detect detects git worktree information for the given path.
// Returns an error if git is not installed or path is not a git repository.
func Detect(path string) (*DetectInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get worktree root
	gitRootCmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	gitRootOutput, err := gitRootCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("not a git repository or git command failed: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute git command (is git installed?): %w", err)
	}
	gitRoot := strings.TrimSpace(string(gitRootOutput))

	// Get git common directory
	gitCommonDirCmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--git-common-dir")
	gitCommonDirOutput, err := gitCommonDirCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git common directory: %w", err)
	}
	gitCommonDir := strings.TrimSpace(string(gitCommonDirOutput))

	// If relative, resolve relative to GitRoot
	if !filepath.IsAbs(gitCommonDir) {
		gitCommonDir = filepath.Join(gitRoot, gitCommonDir)
	}

	// Make absolute and clean
	gitCommonDir, err = filepath.Abs(gitCommonDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for git common dir: %w", err)
	}
	gitCommonDir = filepath.Clean(gitCommonDir)

	// Determine if this is a linked worktree
	// Main repo: GitCommonDir == <repo>/.git
	// Linked worktree: GitCommonDir == <main-repo>/.git (but GitRoot is different)
	mainRepoGitDir := filepath.Join(gitRoot, ".git")
	isWorktree := gitCommonDir != mainRepoGitDir

	// Calculate MainWorktree
	// GitCommonDir should end with ".git" for both main and linked worktrees
	var mainWorktree string
	if strings.HasSuffix(gitCommonDir, string(filepath.Separator)+".git") || gitCommonDir == ".git" {
		mainWorktree = filepath.Dir(gitCommonDir)
	} else {
		// Worktrees have GitCommonDir like: /path/to/repo/.git/worktrees/<name>
		// So we need to go up two levels
		mainWorktree = filepath.Dir(filepath.Dir(gitCommonDir))
	}

	// Calculate WorktreeID as hex(sha256(GitCommonDir))[:12]
	hash := sha256.Sum256([]byte(gitCommonDir))
	worktreeID := hex.EncodeToString(hash[:])[:12]

	return &DetectInfo{
		GitRoot:      gitRoot,
		GitCommonDir: gitCommonDir,
		IsWorktree:   isWorktree,
		MainWorktree: mainWorktree,
		WorktreeID:   worktreeID,
	}, nil
}

// IsGitRepo returns true if the given path is within a git repository.
// Returns false on any error (git not installed, not a repo, etc.).
func IsGitRepo(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--git-dir")
	err := cmd.Run()
	return err == nil
}
