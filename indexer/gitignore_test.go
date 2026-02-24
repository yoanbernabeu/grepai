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

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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

	matcher, err := NewIgnoreMatcher(tmpDir, extraPatterns, "")
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

	matcher, err := NewIgnoreMatcher(tmpDir, extraPatterns, "")
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
	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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
	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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
	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
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

func TestExpandTilde(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("could not get home directory: %v", err)
	}

	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		{"~/foo/bar", filepath.Join(homeDir, "foo/bar"), "tilde with path"},
		{"~", homeDir, "tilde only"},
		{"/absolute/path", "/absolute/path", "absolute path unchanged"},
		{"relative/path", "relative/path", "relative path unchanged"},
		{"", "", "empty string unchanged"},
		{"~username/path", "~username/path", "tilde with username unchanged"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := expandTilde(tt.input)
			if result != tt.expected {
				t.Errorf("expandTilde(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIgnoreMatcher_ExternalGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create external gitignore file
	externalGitignore := filepath.Join(tmpDir, "external-gitignore")
	externalContent := `*.external
external-dir/
`
	if err := os.WriteFile(externalGitignore, []byte(externalContent), 0644); err != nil {
		t.Fatalf("failed to create external gitignore: %v", err)
	}

	// Create project directory
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create project .gitignore
	projectGitignore := `*.log
build/
`
	if err := os.WriteFile(filepath.Join(projectDir, ".gitignore"), []byte(projectGitignore), 0644); err != nil {
		t.Fatalf("failed to create project .gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(projectDir, []string{}, externalGitignore)
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// From external gitignore
		{"file.external", true, "external pattern *.external"},
		{"external-dir", true, "external pattern external-dir/"},
		{"external-dir/file.go", true, "external pattern external-dir/ content"},

		// From project .gitignore
		{"debug.log", true, "project pattern *.log"},
		{"build", true, "project pattern build/"},
		{"build/app.go", true, "project pattern build/ content"},

		// Not ignored
		{"main.go", false, "regular go file"},
		{"src/app.go", false, "go file in src"},
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

func TestIgnoreMatcher_ExternalGitignore_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project directory
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create project .gitignore
	if err := os.WriteFile(filepath.Join(projectDir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("failed to create project .gitignore: %v", err)
	}

	// External gitignore path that doesn't exist
	nonExistentPath := filepath.Join(tmpDir, "non-existent-gitignore")

	// Should not fail, just log a warning
	matcher, err := NewIgnoreMatcher(projectDir, []string{}, nonExistentPath)
	if err != nil {
		t.Fatalf("NewIgnoreMatcher should not fail with non-existent external gitignore: %v", err)
	}

	// Project .gitignore patterns should still work
	if !matcher.ShouldIgnore("debug.log") {
		t.Error("ShouldIgnore(\"debug.log\") = false, expected true")
	}

	// Non-ignored files should not be affected
	if matcher.ShouldIgnore("main.go") {
		t.Error("ShouldIgnore(\"main.go\") = true, expected false")
	}
}

func TestIgnoreMatcher_ExternalGitignore_WithTilde(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project directory
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Test with a path that would have tilde (but we can't actually test ~ expansion
	// without creating files in user's home, so we test that it handles the path correctly)
	// The expandTilde function is tested separately in TestExpandTilde

	// Using a non-tilde path for the actual test
	externalGitignore := filepath.Join(tmpDir, "gitignore-config")
	if err := os.WriteFile(externalGitignore, []byte("*.ignored\n"), 0644); err != nil {
		t.Fatalf("failed to create external gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(projectDir, []string{}, externalGitignore)
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	if !matcher.ShouldIgnore("test.ignored") {
		t.Error("ShouldIgnore(\"test.ignored\") = false, expected true")
	}
}

func TestGrepaiIgnore_BasicExclusion(t *testing.T) {
	tmpDir := t.TempDir()

	// .grepaiignore adds extra exclusion patterns
	grepaiIgnore := `secret-data/
*.generated.go
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"secret-data", true, "directory excluded by .grepaiignore"},
		{"secret-data/keys.go", true, "file inside excluded dir"},
		{"models.generated.go", true, "file matching .grepaiignore wildcard"},
		{"src/models.generated.go", true, "nested file matching .grepaiignore wildcard"},
		{"main.go", false, "regular file not affected"},
		{"src/app.go", false, "nested regular file not affected"},
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

func TestGrepaiIgnore_Negation(t *testing.T) {
	tmpDir := t.TempDir()

	// .gitignore excludes vendor/
	gitignore := `vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// .grepaiignore re-includes vendor/
	grepaiIgnore := `!vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	// Create vendor directory so Walk can find .grepaiignore
	if err := os.MkdirAll(filepath.Join(tmpDir, "vendor", "pkg"), 0755); err != nil {
		t.Fatalf("failed to create vendor dir: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"vendor", false, "vendor dir re-included by .grepaiignore"},
		{"vendor/pkg/main.go", false, "file in vendor re-included"},
		{"main.go", false, "regular file not affected"},
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

func TestGrepaiIgnore_SelectiveNegation(t *testing.T) {
	tmpDir := t.TempDir()

	// .gitignore excludes vendor/
	gitignore := `vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// .grepaiignore re-includes only vendor/important/
	grepaiIgnore := `vendor/
!vendor/important/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	// Create directories
	if err := os.MkdirAll(filepath.Join(tmpDir, "vendor", "important"), 0755); err != nil {
		t.Fatalf("failed to create vendor/important dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "vendor", "other"), 0755); err != nil {
		t.Fatalf("failed to create vendor/other dir: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"vendor/important", false, "vendor/important re-included"},
		{"vendor/important/lib.go", false, "file in vendor/important re-included"},
		{"vendor/other", true, "vendor/other still excluded"},
		{"vendor/other/lib.go", true, "file in vendor/other still excluded"},
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

func TestGrepaiIgnore_OverridesExtraPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Extra patterns exclude vendor
	extraPatterns := []string{"vendor"}

	// .grepaiignore re-includes vendor
	grepaiIgnore := `!vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, extraPatterns, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// .grepaiignore should override the extra pattern
	if matcher.ShouldIgnore("vendor") {
		t.Error("ShouldIgnore(\"vendor\") = true, expected false (.grepaiignore should override extra patterns)")
	}
	if matcher.ShouldIgnore("vendor/pkg/main.go") {
		t.Error("ShouldIgnore(\"vendor/pkg/main.go\") = true, expected false")
	}
}

func TestGrepaiIgnore_Nested(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with its own .grepaiignore
	subDir := filepath.Join(tmpDir, "services")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create services dir: %v", err)
	}

	grepaiIgnore := `*.test.go
`
	if err := os.WriteFile(filepath.Join(subDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create services/.grepaiignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"services/handler.test.go", true, "test file in services excluded by nested .grepaiignore"},
		{"services/handler.go", false, "regular file in services not affected"},
		{"handler.test.go", false, "test file in root NOT affected (nested .grepaiignore doesn't apply)"},
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

func TestGrepaiIgnore_NotPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Only .gitignore, no .grepaiignore
	gitignore := `build/
*.log
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// Behavior should be identical to before: no .grepaiignore means no change
	tests := []struct {
		path     string
		expected bool
	}{
		{"build", true},
		{"build/app.go", true},
		{"debug.log", true},
		{"main.go", false},
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

func TestShouldSkipDir_WithNegation(t *testing.T) {
	tmpDir := t.TempDir()

	// .gitignore excludes vendor/
	gitignore := `vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// .grepaiignore re-includes specific files in vendor
	grepaiIgnore := `!vendor/important.go
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	// Create vendor dir
	if err := os.MkdirAll(filepath.Join(tmpDir, "vendor"), 0755); err != nil {
		t.Fatalf("failed to create vendor dir: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// ShouldSkipDir should return false for vendor/ because .grepaiignore has negations
	// (files inside might be re-included)
	if matcher.ShouldSkipDir("vendor") {
		t.Error("ShouldSkipDir(\"vendor\") = true, expected false (negation patterns exist)")
	}

	// Without .grepaiignore negations, ShouldSkipDir should return true
	tmpDir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir2, ".gitignore"), []byte("vendor/\n"), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	matcher2, err := NewIgnoreMatcher(tmpDir2, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	if !matcher2.ShouldSkipDir("vendor") {
		t.Error("ShouldSkipDir(\"vendor\") = false, expected true (no .grepaiignore)")
	}
}

func TestScanner_RespectsGrepaiIgnoreNegation(t *testing.T) {
	tmpDir := t.TempDir()

	// .gitignore excludes vendor/
	gitignore := `vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// .grepaiignore re-includes vendor/
	grepaiIgnore := `!vendor/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	// Create source files
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create vendor files (normally excluded by .gitignore, but re-included by .grepaiignore)
	vendorDir := filepath.Join(tmpDir, "vendor", "pkg")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("failed to create vendor/pkg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("package pkg"), 0644); err != nil {
		t.Fatalf("failed to create vendor/pkg/lib.go: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, matcher)
	files, _, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should find both src/main.go and vendor/pkg/lib.go
	foundPaths := make(map[string]bool)
	for _, f := range files {
		foundPaths[f.Path] = true
	}

	expectedFiles := []string{
		filepath.Join("src", "main.go"),
		filepath.Join("vendor", "pkg", "lib.go"),
	}

	for _, expected := range expectedFiles {
		if !foundPaths[expected] {
			t.Errorf("expected to find %s in scan results, but it was missing", expected)
		}
	}

	if len(files) != len(expectedFiles) {
		t.Errorf("expected %d files, got %d", len(expectedFiles), len(files))
		for _, f := range files {
			t.Logf("  found: %s", f.Path)
		}
	}
}

func TestGrepaiIgnore_DoesNotOverrideNestedGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Root .grepaiignore re-includes generated/
	grepaiIgnore := `!generated/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	// src/.gitignore excludes generated/
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	srcGitignore := `generated/
`
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte(srcGitignore), 0644); err != nil {
		t.Fatalf("failed to create src/.gitignore: %v", err)
	}

	// Create directories
	if err := os.MkdirAll(filepath.Join(srcDir, "generated"), 0755); err != nil {
		t.Fatalf("failed to create src/generated dir: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Root .grepaiignore !generated/ should NOT override src/.gitignore generated/
		{"src/generated", true, "src/.gitignore generated/ wins over root .grepaiignore"},
		{"src/generated/code.go", true, "file in src/generated still ignored by nested .gitignore"},
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

func TestGrepaiIgnore_OverridesSameLevelGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Root .gitignore excludes generated/
	gitignore := `generated/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}

	// Root .grepaiignore re-includes generated/ (same level)
	grepaiIgnore := `!generated/
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".grepaiignore"), []byte(grepaiIgnore), 0644); err != nil {
		t.Fatalf("failed to create .grepaiignore: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// Root .grepaiignore SHOULD override root .gitignore (same level)
	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"generated", false, "generated re-included by same-level .grepaiignore"},
		{"generated/code.go", false, "file in generated re-included"},
		{"src/generated", false, "src/generated also re-included (root gitignore + root grepaiignore)"},
		{"src/generated/code.go", false, "file in src/generated re-included"},
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

func TestGrepaiIgnore_NestedOverridesNestedGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// src/.gitignore excludes generated/
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte("generated/\n"), 0644); err != nil {
		t.Fatalf("failed to create src/.gitignore: %v", err)
	}

	// src/.grepaiignore re-includes generated/ (same level as the .gitignore)
	if err := os.WriteFile(filepath.Join(srcDir, ".grepaiignore"), []byte("!generated/\n"), 0644); err != nil {
		t.Fatalf("failed to create src/.grepaiignore: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(srcDir, "generated"), 0755); err != nil {
		t.Fatalf("failed to create src/generated dir: %v", err)
	}

	matcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	// src/.grepaiignore SHOULD override src/.gitignore (same level)
	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"src/generated", false, "src/generated re-included by same-level src/.grepaiignore"},
		{"src/generated/code.go", false, "file in src/generated re-included"},
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
