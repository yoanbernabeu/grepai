package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestNormalizeProjectPathPrefix(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "proj")
	insideDir := filepath.Join(projectRoot, "src")
	insideFile := filepath.Join(insideDir, "main.go")
	outside := filepath.Join(t.TempDir(), "other", "x.go")

	tests := []struct {
		name       string
		pathPrefix string
		want       string
		wantErr    bool
	}{
		{
			name:       "empty",
			pathPrefix: "",
			want:       "",
		},
		{
			name:       "relative passthrough",
			pathPrefix: "src/handlers/",
			want:       "src/handlers/",
		},
		{
			name:       "absolute inside project",
			pathPrefix: insideFile,
			want:       "src/main.go",
		},
		{
			name:       "absolute project root",
			pathPrefix: projectRoot,
			want:       "",
		},
		{
			name:       "absolute outside project",
			pathPrefix: outside,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeProjectPathPrefix(tt.pathPrefix, projectRoot)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeProjectPathPrefix() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeProjectPathPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeForPathMatch_RelativePath(t *testing.T) {
	// Branche filepath.Abs : projectRoot est un chemin relatif.
	// On obtient le workdir courant pour construire le pathPrefix absolu attendu.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// projectRoot relatif pointant vers le répertoire courant
	projectRoot := "."
	// pathPrefix absolu sous le workdir courant
	pathPrefix := filepath.Join(wd, "path_normalizer.go")

	got, err := NormalizeProjectPathPrefix(pathPrefix, projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "path_normalizer.go" {
		t.Fatalf("got %q, want %q", got, "path_normalizer.go")
	}
}

func TestNormalizeForPathMatch_NonExistentRoot(t *testing.T) {
	// Branche EvalSymlinks error : root does not exist, fallback to unresolved path.
	projectRoot := "/nonexistent/root"
	pathPrefix := "/nonexistent/root/src/foo.go"

	got, err := NormalizeProjectPathPrefix(pathPrefix, projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "src/foo.go" {
		t.Fatalf("got %q, want %q", got, "src/foo.go")
	}
}

func TestNormalizeWorkspacePathPrefix(t *testing.T) {
	root := t.TempDir()
	projA := filepath.Join(root, "projA")
	projB := filepath.Join(root, "projB")
	projNested := filepath.Join(projA, "nested")

	ws := &config.Workspace{
		Name: "ws",
		Projects: []config.ProjectEntry{
			{Name: "a", Path: projA},
			{Name: "b", Path: projB},
			{Name: "nested", Path: projNested},
		},
	}

	tests := []struct {
		name             string
		pathPrefix       string
		selectedProjects []string
		wantPrefix       string
		wantProjects     []string
		wantErr          bool
	}{
		{
			name:       "relative passthrough",
			pathPrefix: "src/",
			wantPrefix: "src/",
		},
		{
			name:         "absolute in project a",
			pathPrefix:   filepath.Join(projA, "src", "main.go"),
			wantPrefix:   "src/main.go",
			wantProjects: []string{"a"},
		},
		{
			name:         "absolute in nested project picks longest match",
			pathPrefix:   filepath.Join(projNested, "pkg", "x.go"),
			wantPrefix:   "pkg/x.go",
			wantProjects: []string{"nested"},
		},
		{
			name:             "absolute path narrowed from selected projects",
			pathPrefix:       filepath.Join(projB, "src"),
			selectedProjects: []string{"a", "b"},
			wantPrefix:       "src",
			wantProjects:     []string{"b"},
		},
		{
			name:             "absolute path not in selected projects",
			pathPrefix:       filepath.Join(projB, "src"),
			selectedProjects: []string{"a"},
			wantErr:          true,
		},
		{
			name:       "absolute path outside workspace",
			pathPrefix: filepath.Join(root, "outside", "z.go"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrefix, gotProjects, err := NormalizeWorkspacePathPrefix(tt.pathPrefix, ws, tt.selectedProjects)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeWorkspacePathPrefix() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotPrefix != tt.wantPrefix {
				t.Fatalf("NormalizeWorkspacePathPrefix() prefix = %q, want %q", gotPrefix, tt.wantPrefix)
			}
			if len(gotProjects) != len(tt.wantProjects) {
				t.Fatalf("NormalizeWorkspacePathPrefix() projects = %#v, want %#v", gotProjects, tt.wantProjects)
			}
			for i := range gotProjects {
				if gotProjects[i] != tt.wantProjects[i] {
					t.Fatalf("NormalizeWorkspacePathPrefix() projects = %#v, want %#v", gotProjects, tt.wantProjects)
				}
			}
		})
	}
}
