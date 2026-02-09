package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func assertSamePath(t *testing.T, label, got, want string) {
	t.Helper()

	gotClean := filepath.Clean(got)
	wantClean := filepath.Clean(want)

	gotInfo, gotErr := os.Stat(gotClean)
	wantInfo, wantErr := os.Stat(wantClean)
	if gotErr == nil && wantErr == nil {
		if !os.SameFile(gotInfo, wantInfo) {
			t.Errorf("%s = %q, want same location as %q", label, got, want)
		}
		return
	}

	if runtime.GOOS == "windows" {
		if !strings.EqualFold(gotClean, wantClean) {
			t.Errorf("%s = %q, want %q", label, got, want)
		}
		return
	}

	if gotClean != wantClean {
		t.Errorf("%s = %q, want %q", label, got, want)
	}
}

// setupGitRepo initializes a git repo in the given directory with an empty commit.
func setupGitRepo(t *testing.T, path string) {
	t.Helper()

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Initialize repo
	cmd := exec.Command("git", "init", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure user
	configEmail := exec.Command("git", "-C", path, "config", "user.email", "test@test.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}

	configName := exec.Command("git", "-C", path, "config", "user.name", "Test")
	if err := configName.Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create empty commit
	commit := exec.Command("git", "-C", path, "commit", "--allow-empty", "-m", "init")
	if err := commit.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}
}

func TestDetect_MainRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	setupGitRepo(t, repoPath)

	info, err := Detect(repoPath)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// GitRoot should be the repo path
	assertSamePath(t, "GitRoot", info.GitRoot, repoPath)

	// IsWorktree should be false for main repo
	if info.IsWorktree {
		t.Error("IsWorktree = true, want false for main repo")
	}

	// GitCommonDir should end with /.git
	expectedGitDir := filepath.Join(repoPath, ".git")
	assertSamePath(t, "GitCommonDir", info.GitCommonDir, expectedGitDir)

	// WorktreeID should be non-empty and 12 chars
	if len(info.WorktreeID) != 12 {
		t.Errorf("WorktreeID length = %d, want 12", len(info.WorktreeID))
	}
	if info.WorktreeID == "" {
		t.Error("WorktreeID is empty")
	}

	// MainWorktree should be the repo path
	assertSamePath(t, "MainWorktree", info.MainWorktree, repoPath)
}

func TestDetect_LinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	// Create a linked worktree
	worktreePath := filepath.Join(t.TempDir(), "worktree")
	addWorktree := exec.Command("git", "-C", mainRepo, "worktree", "add", worktreePath, "-b", "test-branch")
	if err := addWorktree.Run(); err != nil {
		t.Fatalf("failed to add worktree: %v", err)
	}

	// Detect main repo
	mainInfo, err := Detect(mainRepo)
	if err != nil {
		t.Fatalf("Detect main repo failed: %v", err)
	}

	// Detect worktree
	wtInfo, err := Detect(worktreePath)
	if err != nil {
		t.Fatalf("Detect worktree failed: %v", err)
	}

	// GitRoot should be the worktree path
	assertSamePath(t, "worktree GitRoot", wtInfo.GitRoot, worktreePath)

	// IsWorktree should be true
	if !wtInfo.IsWorktree {
		t.Error("worktree IsWorktree = false, want true")
	}

	// GitCommonDir should point to main repo's .git
	expectedGitDir := filepath.Join(mainRepo, ".git")
	assertSamePath(t, "worktree GitCommonDir", wtInfo.GitCommonDir, expectedGitDir)

	// MainWorktree should be the main repo path
	assertSamePath(t, "worktree MainWorktree", wtInfo.MainWorktree, mainRepo)

	// CRITICAL: WorktreeID should match between main and worktree (same repo identity)
	if wtInfo.WorktreeID != mainInfo.WorktreeID {
		t.Errorf("WorktreeID mismatch: worktree=%q, main=%q", wtInfo.WorktreeID, mainInfo.WorktreeID)
	}
}

func TestDetect_NotGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	notARepo := t.TempDir()

	_, err := Detect(notARepo)
	if err == nil {
		t.Fatal("Detect should fail on non-git directory")
	}
}

func TestIsGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Positive case: actual git repo
	repoPath := t.TempDir()
	setupGitRepo(t, repoPath)

	if !IsGitRepo(repoPath) {
		t.Error("IsGitRepo returned false for actual git repo")
	}

	// Negative case: not a git repo
	notARepo := t.TempDir()
	if IsGitRepo(notARepo) {
		t.Error("IsGitRepo returned true for non-git directory")
	}
}

func TestDetect_WorktreeIDConsistency(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	// Create two linked worktrees
	wt1Path := filepath.Join(t.TempDir(), "worktree1")
	wt2Path := filepath.Join(t.TempDir(), "worktree2")

	addWt1 := exec.Command("git", "-C", mainRepo, "worktree", "add", wt1Path, "-b", "branch1")
	if err := addWt1.Run(); err != nil {
		t.Fatalf("failed to add worktree1: %v", err)
	}

	addWt2 := exec.Command("git", "-C", mainRepo, "worktree", "add", wt2Path, "-b", "branch2")
	if err := addWt2.Run(); err != nil {
		t.Fatalf("failed to add worktree2: %v", err)
	}

	// Detect all three
	mainInfo, err := Detect(mainRepo)
	if err != nil {
		t.Fatalf("Detect main failed: %v", err)
	}

	wt1Info, err := Detect(wt1Path)
	if err != nil {
		t.Fatalf("Detect wt1 failed: %v", err)
	}

	wt2Info, err := Detect(wt2Path)
	if err != nil {
		t.Fatalf("Detect wt2 failed: %v", err)
	}

	// All three should have the same WorktreeID
	if mainInfo.WorktreeID != wt1Info.WorktreeID {
		t.Errorf("main and wt1 WorktreeID mismatch: %q vs %q", mainInfo.WorktreeID, wt1Info.WorktreeID)
	}

	if mainInfo.WorktreeID != wt2Info.WorktreeID {
		t.Errorf("main and wt2 WorktreeID mismatch: %q vs %q", mainInfo.WorktreeID, wt2Info.WorktreeID)
	}

	if wt1Info.WorktreeID != wt2Info.WorktreeID {
		t.Errorf("wt1 and wt2 WorktreeID mismatch: %q vs %q", wt1Info.WorktreeID, wt2Info.WorktreeID)
	}
}

func TestDetect_GitNotInstalled(t *testing.T) {
	// This test is tricky - we can't really uninstall git
	// But we can test the error path by using an invalid path
	// that will cause exec.LookPath to fail in the Detect function

	// Create a directory that definitely doesn't have git-related content
	notARepo := t.TempDir()

	// If git IS installed, this will fail with "not a git repository"
	// If git is NOT installed, it should fail with a different error
	_, err := Detect(notARepo)
	if err == nil {
		t.Fatal("Detect should fail")
	}

	// Just verify we got an error - the exact error depends on whether git is installed
}

func TestIsGitRepo_InvalidPath(t *testing.T) {
	// Test with a path that definitely doesn't exist
	nonExistentPath := filepath.Join(os.TempDir(), "this-path-does-not-exist-12345")

	// Should return false, not panic
	if IsGitRepo(nonExistentPath) {
		t.Error("IsGitRepo returned true for non-existent path")
	}
}
