package indexer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

type IgnoreMatcher struct {
	matchers  []*ignore.GitIgnore
	extraDirs []string
}

func NewIgnoreMatcher(projectRoot string, extraIgnore []string) (*IgnoreMatcher, error) {
	m := &IgnoreMatcher{
		extraDirs: extraIgnore,
	}

	// Load .gitignore if it exists
	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		gi, err := ignore.CompileIgnoreFile(gitignorePath)
		if err != nil {
			return nil, err
		}
		m.matchers = append(m.matchers, gi)
	}

	// Add extra ignore patterns
	if len(extraIgnore) > 0 {
		gi := ignore.CompileIgnoreLines(extraIgnore...)
		m.matchers = append(m.matchers, gi)
	}

	return m, nil
}

func (m *IgnoreMatcher) ShouldIgnore(path string) bool {
	// Check extra directories first (exact match for efficiency)
	base := filepath.Base(path)
	for _, dir := range m.extraDirs {
		if base == dir {
			return true
		}
	}

	// Check gitignore patterns
	for _, matcher := range m.matchers {
		if matcher.MatchesPath(path) {
			return true
		}
		// Also check with trailing slash to match directory patterns like "build/"
		// This ensures patterns ending with "/" properly match directory names
		if matcher.MatchesPath(path + "/") {
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
