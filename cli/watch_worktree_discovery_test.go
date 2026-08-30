package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitDiscovery(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func setupMainRepoForWorktreeDiscovery(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	mainRepo := filepath.Join(root, "main")
	worktreePath := filepath.Join(root, "wt-feature")

	if err := os.MkdirAll(mainRepo, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	runGitDiscovery(t, mainRepo, "init")
	runGitDiscovery(t, mainRepo, "config", "user.email", "test@example.com")
	runGitDiscovery(t, mainRepo, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(mainRepo, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runGitDiscovery(t, mainRepo, "add", "README.md")
	runGitDiscovery(t, mainRepo, "commit", "-m", "init")

	grepaiDir := filepath.Join(mainRepo, ".grepai")
	if err := os.MkdirAll(grepaiDir, 0755); err != nil {
		t.Fatalf("failed to create .grepai: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grepaiDir, "config.yaml"), []byte("watch:\n  debounce_ms: 500\n"), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	runGitDiscovery(t, mainRepo, "worktree", "add", worktreePath, "-b", "feature/test-discovery")
	return mainRepo, worktreePath
}

func TestDiscoverWorktreesForWatch_MainRepoDetectsAndAutoInitsLinkedWorktrees(t *testing.T) {
	mainRepo, worktreePath := setupMainRepoForWorktreeDiscovery(t)

	got := discoverWorktreesForWatch(mainRepo)
	if len(got) != 1 {
		t.Fatalf("discoverWorktreesForWatch() returned %d worktrees, want 1", len(got))
	}
	if canonicalPath(got[0]) != canonicalPath(worktreePath) {
		t.Fatalf("discoverWorktreesForWatch()[0]=%q, want %q", got[0], worktreePath)
	}

	localConfig := filepath.Join(worktreePath, ".grepai", "config.yaml")
	if _, err := os.Stat(localConfig); err != nil {
		t.Fatalf("expected auto-init config at %s: %v", localConfig, err)
	}

	gitignorePath := filepath.Join(worktreePath, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".grepai/") {
		t.Fatalf("expected .gitignore to contain .grepai/, got: %s", string(content))
	}
}

func TestDiscoverWorktreesForWatch_LinkedWorktreeDoesNotDiscoverSiblings(t *testing.T) {
	_, worktreePath := setupMainRepoForWorktreeDiscovery(t)

	got := discoverWorktreesForWatch(worktreePath)
	if len(got) != 0 {
		t.Fatalf("discoverWorktreesForWatch() returned %d worktrees for linked worktree, want 0", len(got))
	}
}

func TestDiscoverWorktreesForWatch_DisabledByConfig(t *testing.T) {
	mainRepo, worktreePath := setupMainRepoForWorktreeDiscovery(t)

	configPath := filepath.Join(mainRepo, ".grepai", "config.yaml")
	configYAML := "watch:\n  debounce_ms: 500\n  discover_worktrees: false\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	got := discoverWorktreesForWatch(mainRepo)
	if len(got) != 0 {
		t.Fatalf("discoverWorktreesForWatch() returned %d worktrees with discovery disabled, want 0", len(got))
	}

	// Discovery must not have auto-initialized the linked worktree either.
	if _, err := os.Stat(filepath.Join(worktreePath, ".grepai")); !os.IsNotExist(err) {
		t.Fatalf("expected no .grepai auto-init in linked worktree when discovery is disabled")
	}
}

func TestDiscoverWorktreesForWatch_UnreadableConfigFailsClosed(t *testing.T) {
	mainRepo, worktreePath := setupMainRepoForWorktreeDiscovery(t)

	// A torn/invalid config (e.g. saved mid-edit between reconcile passes)
	// must not re-enable discovery: auto-init has side effects (creates
	// .grepai/, edits .gitignore) that skipping a pass does not.
	configPath := filepath.Join(mainRepo, ".grepai", "config.yaml")
	if err := os.WriteFile(configPath, []byte("watch: [broken\n"), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	got := discoverWorktreesForWatch(mainRepo)
	if len(got) != 0 {
		t.Fatalf("discoverWorktreesForWatch() returned %d worktrees with unreadable config, want 0 (fail closed)", len(got))
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".grepai")); !os.IsNotExist(err) {
		t.Fatalf("expected no .grepai auto-init in linked worktree when config load fails")
	}
}
