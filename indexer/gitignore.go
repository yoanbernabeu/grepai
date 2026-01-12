package indexer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// nestedMatcher holds a gitignore matcher and its base directory
type nestedMatcher struct {
	matcher *ignore.GitIgnore
	baseDir string // relative path from project root (empty for root .gitignore)
}

type IgnoreMatcher struct {
	projectRoot    string
	nestedMatchers []nestedMatcher
	extraDirs      []string
}

func NewIgnoreMatcher(projectRoot string, extraIgnore []string) (*IgnoreMatcher, error) {
	m := &IgnoreMatcher{
		projectRoot: projectRoot,
		extraDirs:   extraIgnore,
	}

	// Walk the project to find all .gitignore files
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		// Skip directories that should be ignored by default
		if info.IsDir() {
			base := filepath.Base(path)
			for _, dir := range extraIgnore {
				if base == dir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only process .gitignore files
		if filepath.Base(path) != ".gitignore" {
			return nil
		}

		gi, err := ignore.CompileIgnoreFile(path)
		if err != nil {
			return nil // Skip invalid .gitignore files
		}

		// Get relative base directory
		relPath, err := filepath.Rel(projectRoot, filepath.Dir(path))
		if err != nil {
			return nil
		}
		if relPath == "." {
			relPath = ""
		}

		m.nestedMatchers = append(m.nestedMatchers, nestedMatcher{
			matcher: gi,
			baseDir: relPath,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Add extra ignore patterns as a root-level matcher
	if len(extraIgnore) > 0 {
		gi := ignore.CompileIgnoreLines(extraIgnore...)
		m.nestedMatchers = append(m.nestedMatchers, nestedMatcher{
			matcher: gi,
			baseDir: "",
		})
	}

	return m, nil
}

func (m *IgnoreMatcher) ShouldIgnore(path string) bool {
	// Normalize path separators for cross-platform compatibility
	normalizedPath := filepath.ToSlash(path)

	// Check extra directories first (exact match for efficiency)
	base := filepath.Base(path)
	for _, dir := range m.extraDirs {
		if base == dir {
			return true
		}
	}

	// Check nested gitignore patterns
	for _, nm := range m.nestedMatchers {
		// Determine the relative path from this matcher's base directory
		var relPath string
		if nm.baseDir == "" {
			// Root-level matcher applies to all paths
			relPath = normalizedPath
		} else {
			// Nested matcher only applies to paths within its directory
			normalizedBase := filepath.ToSlash(nm.baseDir)
			if !strings.HasPrefix(normalizedPath, normalizedBase+"/") && normalizedPath != normalizedBase {
				continue // This matcher doesn't apply to this path
			}
			// Get path relative to the matcher's base directory
			relPath = strings.TrimPrefix(normalizedPath, normalizedBase+"/")
		}

		if nm.matcher.MatchesPath(relPath) {
			return true
		}
		// Also check with trailing slash to match directory patterns like "build/"
		if nm.matcher.MatchesPath(relPath + "/") {
			return true
		}
	}

	return false
}

// AddToGitignore appends a pattern to .gitignore if not already present
func AddToGitignore(projectRoot string, pattern string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	// Check if pattern already exists
	if exists, err := patternExists(gitignorePath, pattern); err != nil {
		return err
	} else if exists {
		return nil
	}

	// Append pattern
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() > 0 {
		// Check if file ends with newline
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			return err
		}
		if len(content) > 0 && content[len(content)-1] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}

	if _, err := f.WriteString(pattern + "\n"); err != nil {
		return err
	}

	return nil
}

func patternExists(gitignorePath string, pattern string) (bool, error) {
	f, err := os.Open(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == pattern {
			return true, nil
		}
	}

	return false, scanner.Err()
}
