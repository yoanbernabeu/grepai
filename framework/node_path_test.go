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
		rejected = append(rejected, `C:evil`, `\\attacker.example\share\node.exe`, `\\?\UNC\attacker.example\share\node.exe`, `\tools\node`)
	} else {
		rejected = append(rejected, `C:evil`, `tools\node`)
	}
	rejected = append(rejected, "bun", "deno", "sh")
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
