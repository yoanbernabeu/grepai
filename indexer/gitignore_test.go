package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreMatcher_GitignorePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .gitignore with various patterns
	gitignore := `# Build artifacts
build/
dist/

# Dependencies
node_modules/
vendor/

# Logs
*.log

# Specific file
secret.txt
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
	if err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Files that should NOT be ignored
		{"main.go", false, "regular go file"},
		{"src/app.go", false, "go file in src"},
		{"README.md", false, "readme file"},

		// Directory patterns (build/)
		{"build", true, "build directory itself"},
		{"build/app.go", true, "file inside build"},
		{"build/sub/file.go", true, "nested file inside build"},

		// Directory patterns (node_modules/)
		{"node_modules", true, "node_modules directory itself"},
		{"node_modules/lodash/index.js", true, "file inside node_modules"},

		// Directory patterns (vendor/)
		{"vendor", true, "vendor directory itself"},
		{"vendor/github.com/pkg/main.go", true, "file inside vendor"},

		// Wildcard patterns (*.log)
		{"debug.log", true, "log file in root"},
		{"logs/app.log", true, "log file in subdirectory"},

		// Specific file pattern
		{"secret.txt", true, "specific ignored file"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := matcher.ShouldIgnore(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldIgnore(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIgnoreMatcher_ExtraPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// No .gitignore file, only extra patterns
	extraPatterns := []string{".git", ".grepai", "node_modules", "__pycache__"}

	matcher, err := NewIgnoreMatcher(tmpDir, extraPatterns)
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{".git", true},
		{".grepai", true},
		{"node_modules", true},
		{"__pycache__", true},
		{"src/main.go", false},
		{".git/config", true}, // basename is "config", but parent is .git
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := matcher.ShouldIgnore(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldIgnore(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIgnoreMatcher_CombinedPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .gitignore
	gitignore := `build/
*.log
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
	if err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// Extra patterns from config
	extraPatterns := []string{".git", "vendor"}

	matcher, err := NewIgnoreMatcher(tmpDir, extraPatterns)
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		source   string
	}{
		// From .gitignore
		{"build", true, ".gitignore"},
		{"build/main.go", true, ".gitignore"},
		{"app.log", true, ".gitignore"},

		// From extra patterns
		{".git", true, "extra"},
		{"vendor", true, "extra"},

		// Not ignored
		{"src/main.go", false, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := matcher.ShouldIgnore(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldIgnore(%q) = %v, expected %v (source: %s)", tt.path, result, tt.expected, tt.source)
			}
		})
	}
}

func TestIgnoreMatcher_NoGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// No .gitignore, no extra patterns
	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// Nothing should be ignored
	paths := []string{"main.go", "build/app.go", "node_modules/pkg/index.js", ".git"}
	for _, path := range paths {
		if matcher.ShouldIgnore(path) {
			t.Errorf("ShouldIgnore(%q) = true, expected false (no patterns configured)", path)
		}
	}
}

func TestIgnoreMatcher_DirectoryPatternWithTrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()

	// Test that patterns like "build/" properly ignore the directory itself
	gitignore := `build/
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
	if err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// The directory "build" (without trailing slash) should be ignored
	// when the pattern is "build/" (with trailing slash)
	if !matcher.ShouldIgnore("build") {
		t.Error("ShouldIgnore(\"build\") = false, expected true for pattern \"build/\"")
	}

	// Files inside should also be ignored
	if !matcher.ShouldIgnore("build/main.go") {
		t.Error("ShouldIgnore(\"build/main.go\") = false, expected true for pattern \"build/\"")
	}
}

func TestScanner_RespectsGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .gitignore
	gitignore := `build/
*.log
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
	if err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// Create files that should be indexed
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create files that should be ignored
	buildDir := filepath.Join(tmpDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("failed to create build dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "app.go"), []byte("package build"), 0644); err != nil {
		t.Fatalf("failed to create app.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "debug.log"), []byte("log content"), 0644); err != nil {
		t.Fatalf("failed to create debug.log: %v", err)
	}

	// Create scanner
	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, matcher)
	files, _, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only find src/main.go
	expectedPath := filepath.Join("src", "main.go")
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
		for _, f := range files {
			t.Logf("  found: %s", f.Path)
		}
	}

	if len(files) > 0 && files[0].Path != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, files[0].Path)
	}
}

func TestIgnoreMatcher_NestedGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root .gitignore
	rootGitignore := `*.log
build/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(rootGitignore), 0644); err != nil {
		t.Fatalf("failed to create root .gitignore: %v", err)
	}

	// Create src directory with its own .gitignore
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	srcGitignore := `*.tmp
generated/
`
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte(srcGitignore), 0644); err != nil {
		t.Fatalf("failed to create src/.gitignore: %v", err)
	}

	// Create docs directory with its own .gitignore
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}
	docsGitignore := `_draft/
`
	if err := os.WriteFile(filepath.Join(docsDir, ".gitignore"), []byte(docsGitignore), 0644); err != nil {
		t.Fatalf("failed to create docs/.gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Root .gitignore patterns apply everywhere
		{"debug.log", true, "root pattern *.log in root"},
		{"src/app.log", true, "root pattern *.log in src"},
		{"docs/notes.log", true, "root pattern *.log in docs"},
		{"build", true, "root pattern build/ directory"},
		{"build/app.go", true, "root pattern build/ content"},

		// src/.gitignore patterns only apply in src/
		{"src/temp.tmp", true, "src pattern *.tmp in src"},
		{"src/generated", true, "src pattern generated/ in src"},
		{"src/generated/code.go", true, "src pattern generated/ content"},
		{"temp.tmp", false, "src pattern *.tmp should NOT apply in root"},
		{"docs/temp.tmp", false, "src pattern *.tmp should NOT apply in docs"},

		// docs/.gitignore patterns only apply in docs/
		{"docs/_draft", true, "docs pattern _draft/ in docs"},
		{"docs/_draft/article.md", true, "docs pattern _draft/ content"},
		{"_draft", false, "docs pattern _draft/ should NOT apply in root"},
		{"src/_draft", false, "docs pattern _draft/ should NOT apply in src"},

		// Files that should NOT be ignored
		{"src/main.go", false, "regular go file in src"},
		{"docs/README.md", false, "regular md file in docs"},
		{"main.go", false, "regular go file in root"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := matcher.ShouldIgnore(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldIgnore(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestScanner_RespectsNestedGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root .gitignore
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("failed to create root .gitignore: %v", err)
	}

	// Create src directory with its own .gitignore
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte("generated/\n"), 0644); err != nil {
		t.Fatalf("failed to create src/.gitignore: %v", err)
	}

	// Create files that should be indexed
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create files that should be ignored by root .gitignore
	if err := os.WriteFile(filepath.Join(tmpDir, "debug.log"), []byte("log"), 0644); err != nil {
		t.Fatalf("failed to create debug.log: %v", err)
	}

	// Create files that should be ignored by nested .gitignore
	generatedDir := filepath.Join(srcDir, "generated")
	if err := os.MkdirAll(generatedDir, 0755); err != nil {
		t.Fatalf("failed to create generated dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "code.go"), []byte("package generated"), 0644); err != nil {
		t.Fatalf("failed to create generated/code.go: %v", err)
	}

	// Create scanner
	matcher, err := NewIgnoreMatcher(tmpDir, []string{})
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, matcher)
	files, _, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only find src/main.go (not debug.log or src/generated/code.go)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
		for _, f := range files {
			t.Logf("  found: %s", f.Path)
		}
	}

	expectedPath := filepath.Join("src", "main.go")
	if len(files) > 0 && files[0].Path != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, files[0].Path)
	}
}
