package framework

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNodePath_Allowed(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "bin", "node")
	writeExecutable(t, outside)

	allowed := []string{"", "node", "nodejs", "node20", "Node.exe", outside}
	if runtime.GOOS == "windows" {
		allowed = append(allowed, `\\?\`+outside)
	}
	for _, nodePath := range allowed {
		got, err := ValidateNodePath(nodePath, root)
		if err != nil {
			t.Errorf("ValidateNodePath(%q) unexpected error: %v", nodePath, err)
			continue
		}
		want := nodePath
		if want == "" {
			want = "node"
		}
		if got != want {
			t.Errorf("ValidateNodePath(%q) = %q, want %q", nodePath, got, want)
		}
	}
}

func TestValidateNodePath_RejectsProjectControlledPaths(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "tools", "node")
	writeExecutable(t, inside)

	rejected := []string{
		"./tools/node",
		"tools/node",
		"../node",
		"~/bin/node",
		inside,
		filepath.Join(root, "tools", "..", "tools", "node"),
		filepath.Join(root, "does-not-exist", "node"),
	}
	// On a case-insensitive file system, another spelling of the same path.
	if upper := strings.ToUpper(inside); upper != inside {
		a, errA := os.Stat(upper)
		b, errB := os.Stat(inside)
		if errA == nil && errB == nil && os.SameFile(a, b) {
			rejected = append(rejected, upper)
		}
	}
	if runtime.GOOS == "windows" {
		rejected = append(rejected, `C:evil`, `\\attacker.example\share\node.exe`, `\\?\UNC\attacker.example\share\node.exe`, `\\?\GLOBALROOT\Device\Mup\h\s\node.exe`, `\tools\node`)
	} else {
		rejected = append(rejected, `C:evil`, `tools\node`)
	}
	rejected = append(rejected, "bun", "deno", "sh", "nodemon", "node-dev", "nodex")
	if runtime.GOOS != "windows" {
		rejected = append(rejected, "/proc/self/cwd/tools/node", "/dev/fd/9", filepath.Join(t.TempDir(), "missing", "node"))
	}
	for _, nodePath := range rejected {
		if _, err := ValidateNodePath(nodePath, root); err == nil {
			t.Errorf("ValidateNodePath(%q) should be rejected", nodePath)
		} else if !strings.Contains(err.Error(), "framework_processing.node_path") {
			t.Errorf("ValidateNodePath(%q) error is not actionable: %v", nodePath, err)
		}
	}
}

func TestValidateNodePath_RejectsSymlinkIntoProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	inside := filepath.Join(root, "tools", "node")
	writeExecutable(t, inside)

	// An absolute path outside the project that links into it.
	link := filepath.Join(t.TempDir(), "node")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateNodePath(link, root); err == nil {
		t.Errorf("symlink into the project root should be rejected")
	}

	// The project root itself reached through a symlink.
	rootLink := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateNodePath(filepath.Join(rootLink, "tools", "node"), root); err == nil {
		t.Errorf("path through a symlinked root should be rejected")
	}
	if _, err := ValidateNodePath(inside, rootLink); err == nil {
		t.Errorf("project root given as a symlink should still contain the path")
	}
}

// Paths inside the enclosing repository, outside the grepai project root,
// are rejected when that root is passed too.
func TestValidateNodePath_RejectsOtherRoots(t *testing.T) {
	repo := t.TempDir()
	project := filepath.Join(repo, "frontend")
	script := filepath.Join(repo, "tools", "node")
	writeExecutable(t, script)
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateNodePath(script, project); err != nil {
		t.Fatalf("outside the project root only: unexpected error %v", err)
	}
	if _, err := ValidateNodePath(script, project, repo); err == nil {
		t.Fatal("a path inside the repository root should be rejected")
	}
}

func TestRepositoryRoots(t *testing.T) {
	main := t.TempDir()
	if err := os.MkdirAll(filepath.Join(main, ".git", "worktrees", "wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(main, "frontend", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepositoryRoots(sub); len(got) != 1 || got[0] != main {
		t.Fatalf("RepositoryRoots(sub) = %v, want [%s]", got, main)
	}

	wt := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := "gitdir: " + filepath.Join(main, ".git", "worktrees", "wt") + "\n"
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte(gitFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RepositoryRoots(wt); len(got) != 2 || got[0] != wt || got[1] != main {
		t.Fatalf("RepositoryRoots(worktree) = %v, want [%s %s]", got, wt, main)
	}

	// A bare repository's worktree must not ban the bare repo's parent.
	bare := filepath.Join(t.TempDir(), "repo.git")
	wt2 := filepath.Join(t.TempDir(), "wt2")
	if err := os.MkdirAll(wt2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt2, ".git"), []byte("gitdir: "+filepath.Join(bare, "worktrees", "wt2")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RepositoryRoots(wt2); len(got) != 1 || got[0] != wt2 {
		t.Fatalf("RepositoryRoots(bare worktree) = %v, want [%s]", got, wt2)
	}
}
