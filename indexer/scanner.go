package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxFileSize = 1 * 1024 * 1024 // 1 MB
)

// MinifiedPatterns lists patterns for minified files to skip by default
var MinifiedPatterns = []string{
	".min.js",
	".min.css",
	".bundle.js",
	".bundle.css",
}

// isMinifiedFile checks if a file is a minified file based on naming patterns
func isMinifiedFile(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range MinifiedPatterns {
		if strings.HasSuffix(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// SupportedExtensions lists file extensions to index
var SupportedExtensions = map[string]bool{
	".go":     true,
	".js":     true,
	".ts":     true,
	".jsx":    true,
	".tsx":    true,
	".py":     true,
	".rb":     true,
	".java":   true,
	".c":      true,
	".cpp":    true,
	".cc":     true,
	".h":      true,
	".hpp":    true,
	".cs":     true,
	".php":    true,
	".rs":     true,
	".swift":  true,
	".kt":     true,
	".scala":  true,
	".vue":    true,
	".svelte": true,
	".html":   true,
	".css":    true,
	".scss":   true,
	".less":   true,
	".sql":    true,
	".sh":     true,
	".bash":   true,
	".zsh":    true,
	".yaml":   true,
	".yml":    true,
	".json":   true,
	".xml":    true,
	".md":     true,
	".txt":    true,
	".toml":   true,
	".ini":    true,
	".cfg":    true,
	".conf":   true,
	".env":    true,
	".lua":    true,
	".r":      true,
	".R":      true,
	".dart":   true,
	".ex":     true,
	".exs":    true,
	".erl":    true,
	".clj":    true,
	".hs":     true,
	".ml":     true,
	".fs":     true,
	".elm":    true,
	".nim":    true,
	".zig":    true,
	".proto":  true,
	".tf":     true,
	".hcl":    true,
	".pas":    true, // Pascal source file
	".dpr":    true, // Delphi project file
}

type FileInfo struct {
	Path    string
	Size    int64
	ModTime int64
	Hash    string
	Content string
}

type Scanner struct {
	root   string
	ignore *IgnoreMatcher
}

func NewScanner(root string, ignore *IgnoreMatcher) *Scanner {
	return &Scanner{
		root:   root,
		ignore: ignore,
	}
}

func (s *Scanner) Scan() ([]FileInfo, []string, error) {
	var files []FileInfo
	var skipped []string

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		relPath, err := filepath.Rel(s.root, path)
		if err != nil {
			return nil
		}

		// Skip ignored paths
		if s.ignore.ShouldIgnore(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// Check extension
		ext := strings.ToLower(filepath.Ext(path))
		if !SupportedExtensions[ext] {
			return nil
		}

		// Skip minified files
		if isMinifiedFile(relPath) {
			skipped = append(skipped, relPath+" (minified)")
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Skip large files
		if info.Size() > maxFileSize {
			skipped = append(skipped, relPath+" (too large)")
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Skip binary files
		if !utf8.Valid(content) || containsNull(content) {
			return nil
		}

		// Calculate hash
		hash := sha256.Sum256(content)

		files = append(files, FileInfo{
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Hash:    hex.EncodeToString(hash[:]),
			Content: string(content),
		})

		return nil
	})

	return files, skipped, err
}

func (s *Scanner) ScanFile(relPath string) (*FileInfo, error) {
	absPath := filepath.Join(s.root, relPath)

	// Skip minified files
	if isMinifiedFile(relPath) {
		return nil, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if info.Size() > maxFileSize {
		return nil, nil // Skip large files
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	if !utf8.Valid(content) || containsNull(content) {
		return nil, nil // Skip binary files
	}

	hash := sha256.Sum256(content)

	return &FileInfo{
		Path:    relPath,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		Hash:    hex.EncodeToString(hash[:]),
		Content: string(content),
	}, nil
}

func containsNull(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
