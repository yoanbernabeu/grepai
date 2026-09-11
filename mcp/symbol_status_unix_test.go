//go:build unix

package mcp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestLoadStatusProjectConfigDeniedTraversalDoesNotDefault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are not enforced for the effective root user")
	}

	// Given a project whose .grepai directory denies traversal, so both stat
	// and read of the config fail with permission errors rather than absence.
	root := t.TempDir()
	grepaiDir := filepath.Join(root, config.ConfigDir)
	if err := os.MkdirAll(grepaiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(grepaiDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(grepaiDir, 0o755); err != nil {
			t.Errorf("failed to restore %s permissions: %v", grepaiDir, err)
		}
	})

	// When the status config helper runs.
	cfg, err := loadStatusProjectConfig(root)

	// Then the denial surfaces as a permission error instead of silently
	// falling back to defaults (Exists conflates any stat failure with absence).
	if err == nil {
		t.Fatalf("expected permission error, got defaulted config %#v", cfg)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected wrapped permission error, got %v", err)
	}
}
