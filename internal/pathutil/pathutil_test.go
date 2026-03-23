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
