package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSupportedExtensions(t *testing.T) {
	supported := []string{".go", ".js", ".ts", ".py", ".rs", ".java"}
	unsupported := []string{".exe", ".bin", ".png", ".jpg", ".mp3"}

	for _, ext := range supported {
		if !SupportedExtensions[ext] {
			t.Errorf("expected %s to be supported", ext)
		}
	}

	for _, ext := range unsupported {
		if SupportedExtensions[ext] {
			t.Errorf("expected %s to be unsupported", ext)
		}
	}
}

func TestScanner_Scan(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	goFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	jsFile := filepath.Join(tmpDir, "app.js")
	err = os.WriteFile(jsFile, []byte("console.log('hello');"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create ignore matcher
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	files, skipped, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped files, got %d", len(skipped))
	}

	// Verify file info
	for _, file := range files {
		if file.Path == "" {
			t.Error("file path should not be empty")
		}
		if file.Hash == "" {
			t.Error("file hash should not be empty")
		}
		if file.Content == "" {
			t.Error("file content should not be empty")
		}
	}
}

func TestScanner_IgnoreBinaryFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a binary file (contains null bytes)
	binaryFile := filepath.Join(tmpDir, "binary.go")
	err := os.WriteFile(binaryFile, []byte("package main\x00\x00\x00"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	files, _, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files (binary should be ignored), got %d", len(files))
	}
}

func TestScanner_IgnoreUnsupportedExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create unsupported file
	pngFile := filepath.Join(tmpDir, "image.png")
	err := os.WriteFile(pngFile, []byte("fake png data"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	files, _, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files (unsupported extension), got %d", len(files))
	}
}

func TestScanner_ScanFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc main() {}"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	fileInfo, err := scanner.ScanFile("test.go")
	if err != nil {
		t.Fatalf("scan file failed: %v", err)
	}

	if fileInfo == nil {
		t.Fatal("expected file info, got nil")
	}

	if fileInfo.Path != "test.go" {
		t.Errorf("expected path 'test.go', got '%s'", fileInfo.Path)
	}

	if fileInfo.Content != content {
		t.Errorf("content mismatch")
	}
}

func TestContainsNull(t *testing.T) {
	tests := []struct {
		data     []byte
		expected bool
	}{
		{[]byte("hello world"), false},
		{[]byte("hello\x00world"), true},
		{[]byte{}, false},
		{[]byte{0}, true},
	}

	for _, tt := range tests {
		result := containsNull(tt.data)
		if result != tt.expected {
			t.Errorf("containsNull(%v) = %v, expected %v", tt.data, result, tt.expected)
		}
	}
}

func TestHashFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	hash1, err := HashFile(testFile)
	if err != nil {
		t.Fatalf("failed to hash file: %v", err)
	}

	if hash1 == "" {
		t.Error("hash should not be empty")
	}

	// Same content should produce same hash
	hash2, err := HashFile(testFile)
	if err != nil {
		t.Fatalf("failed to hash file: %v", err)
	}

	if hash1 != hash2 {
		t.Error("same file should produce same hash")
	}
}

func TestIsMinifiedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"jquery.min.js", true},
		{"styles.min.css", true},
		{"app.bundle.js", true},
		{"main.bundle.css", true},
		{"JQUERY.MIN.JS", true},  // Case insensitive
		{"App.Bundle.CSS", true}, // Case insensitive
		{"app.js", false},
		{"styles.css", false},
		{"minified.js", false},          // "min" in name but not pattern
		{"bundle.go", false},            // "bundle" in name but not pattern
		{"src/lib/jquery.min.js", true}, // With path
	}

	for _, tt := range tests {
		result := isMinifiedFile(tt.path)
		if result != tt.expected {
			t.Errorf("isMinifiedFile(%q) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestScanner_SkipsMinifiedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create regular JS file
	regularJS := filepath.Join(tmpDir, "app.js")
	err := os.WriteFile(regularJS, []byte("console.log('regular');"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create minified JS file
	minifiedJS := filepath.Join(tmpDir, "app.min.js")
	err = os.WriteFile(minifiedJS, []byte("console.log('minified');"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create bundle JS file
	bundleJS := filepath.Join(tmpDir, "app.bundle.js")
	err = os.WriteFile(bundleJS, []byte("console.log('bundle');"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	files, skipped, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only have the regular JS file
	if len(files) != 1 {
		t.Errorf("expected 1 file (regular js), got %d", len(files))
	}

	if len(files) > 0 && files[0].Path != "app.js" {
		t.Errorf("expected 'app.js', got '%s'", files[0].Path)
	}

	// Should have 2 skipped files (minified)
	if len(skipped) != 2 {
		t.Errorf("expected 2 skipped files, got %d: %v", len(skipped), skipped)
	}
}

func TestScanner_ScanFile_SkipsMinified(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minified file
	minifiedJS := filepath.Join(tmpDir, "jquery.min.js")
	err := os.WriteFile(minifiedJS, []byte("minified content"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	fileInfo, err := scanner.ScanFile("jquery.min.js")
	if err != nil {
		t.Fatalf("scan file failed: %v", err)
	}

	// Should return nil for minified files
	if fileInfo != nil {
		t.Error("expected nil for minified file, got file info")
	}
}
