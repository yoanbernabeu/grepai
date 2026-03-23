package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveReal(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "resolves real directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			checkFunc: func(t *testing.T, result string) {
				if !filepath.IsAbs(result) {
					t.Errorf("expected absolute path, got %q", result)
				}
			},
		},
		{
			name: "returns error for nonexistent path",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent", "deep", "path")
			},
			wantErr: true,
		},
		{
			name: "resolves symlink to real path",
			setup: func(t *testing.T) string {
				realDir := t.TempDir()
				linkDir := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(realDir, linkDir); err != nil {
					t.Skipf("symlink creation not supported: %v", err)
				}
				return linkDir
			},
			checkFunc: func(t *testing.T, result string) {
				if filepath.Base(result) == "link" {
					t.Errorf("expected resolved path, got symlink path %q", result)
				}
			},
		},
		{
			name: "returns clean path",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				return dir + string(filepath.Separator) + "." + string(filepath.Separator)
			},
			checkFunc: func(t *testing.T, result string) {
				if result != filepath.Clean(result) {
					t.Errorf("expected clean path, got %q", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			result, err := ResolveReal(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveReal(%q) error = %v, wantErr %v", path, err, tt.wantErr)
				return
			}
			if err == nil && tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestResolveReal_FallbackToAbs(t *testing.T) {
	dir := t.TempDir()

	result, err := ResolveReal(dir)
	if err != nil {
		t.Fatalf("ResolveReal(%q) unexpected error: %v", dir, err)
	}

	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %q", result)
	}
	if result != filepath.Clean(result) {
		t.Errorf("expected clean path, got %q", result)
	}
}

func TestResolveReal_SymlinkChain(t *testing.T) {
	realDir := t.TempDir()

	// Create a chain: link2 -> link1 -> realDir
	link1 := filepath.Join(t.TempDir(), "link1")
	if err := os.Symlink(realDir, link1); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	link2 := filepath.Join(t.TempDir(), "link2")
	if err := os.Symlink(link1, link2); err != nil {
		t.Skipf("symlink chain creation not supported: %v", err)
	}

	result, err := ResolveReal(link2)
	if err != nil {
		t.Fatalf("ResolveReal(%q) unexpected error: %v", link2, err)
	}

	// Should resolve all the way to realDir
	expected, _ := filepath.EvalSymlinks(realDir)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
