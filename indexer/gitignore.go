package indexer

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// nestedMatcher holds a gitignore matcher and its base directory
type nestedMatcher struct {
	matcher *ignore.GitIgnore
	baseDir string // relative path from project root (empty for root .gitignore)
}

// grepaiMatcher holds a pair of matchers for .grepaiignore files.
// "full" uses original patterns (with negations) for the actual decision.
// "any" uses all patterns converted to positive for detecting if the file has an opinion.
type grepaiMatcher struct {
	full    *ignore.GitIgnore // Matcher with original patterns (including negations)
	any     *ignore.GitIgnore // Matcher with all patterns as positive (for detection)
	baseDir string            // relative path from project root
}

type IgnoreMatcher struct {
	projectRoot        string
	nestedMatchers     []nestedMatcher // .gitignore matchers
	extraDirs          []string        // patterns from config
	grepaiMatchers     []grepaiMatcher // .grepaiignore matchers
	hasGrepaiNegations bool            // true if any .grepaiignore has ! patterns
}

func NewIgnoreMatcher(projectRoot string, extraIgnore []string, externalGitignore string) (*IgnoreMatcher, error) {
	m := &IgnoreMatcher{
		projectRoot: projectRoot,
		extraDirs:   extraIgnore,
	}

	// Load external gitignore file if specified
	if externalGitignore != "" {
		expandedPath := expandTilde(externalGitignore)
		gi, err := ignore.CompileIgnoreFile(expandedPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("Warning: external gitignore file not found: %s", expandedPath)
			} else {
				log.Printf("Warning: failed to load external gitignore: %v", err)
			}
		} else {
			m.nestedMatchers = append(m.nestedMatchers, nestedMatcher{
				matcher: gi,
				baseDir: "", // External gitignore applies from root
			})
		}
	}

	// Walk the project to find all .gitignore and .grepaiignore files
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

		baseName := filepath.Base(path)

		// Process .gitignore files
		if baseName == ".gitignore" {
			gi, err := ignore.CompileIgnoreFile(path)
			if err != nil {
				return nil // Skip invalid .gitignore files
			}

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
		}

		// Process .grepaiignore files
		if baseName == ".grepaiignore" {
			gm, hasNegations, err := compileGrepaiIgnoreFile(path)
			if err != nil {
				return nil // Skip invalid .grepaiignore files
			}

			relPath, err := filepath.Rel(projectRoot, filepath.Dir(path))
			if err != nil {
				return nil
			}
			if relPath == "." {
				relPath = ""
			}

			gm.baseDir = relPath
			m.grepaiMatchers = append(m.grepaiMatchers, gm)
			if hasNegations {
				m.hasGrepaiNegations = true
			}
		}

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
	normalizedPath := filepath.ToSlash(path)

	// Phase 1: Check if .grepaiignore has an opinion
	result, hasOpinion, grepaiBaseDir := m.evalGrepaiIgnore(normalizedPath)
	if hasOpinion {
		if result {
			return true // .grepaiignore says ignore → always respect
		}
		// .grepaiignore says don't ignore (negation).
		// Only override .gitignore at the same level or shallower.
		// If a deeper .gitignore ignores this path, respect the .gitignore.
		if ignored, gitBaseDir := m.evalGitIgnoreWithLevel(normalizedPath); ignored && len(gitBaseDir) > len(grepaiBaseDir) {
			return true // Deeper .gitignore wins over shallower .grepaiignore
		}
		return false
	}

	// Phase 2: No .grepaiignore opinion, delegate to .gitignore + extra patterns
	return m.evalGitIgnore(normalizedPath)
}

// ShouldSkipDir determines if a directory can be skipped entirely via filepath.SkipDir.
// If .grepaiignore has negation patterns, we must descend into ignored directories
// because individual files inside may be re-included.
func (m *IgnoreMatcher) ShouldSkipDir(path string) bool {
	if !m.ShouldIgnore(path) {
		return false // Not ignored, don't skip
	}

	normalizedPath := filepath.ToSlash(path)

	// If .grepaiignore explicitly says to ignore this dir → safe to skip
	if result, hasOpinion, _ := m.evalGrepaiIgnore(normalizedPath); hasOpinion {
		return result
	}

	// Ignored by gitignore/extra patterns. If no negations in any .grepaiignore → safe to skip
	if !m.hasGrepaiNegations {
		return true
	}

	// There are negation patterns: files inside might be re-included, don't skip
	return false
}

// evalGrepaiIgnore checks .grepaiignore matchers for the path.
// Returns (result, hasOpinion, baseDir) where baseDir is the directory level of the matching .grepaiignore.
// The most specific matcher (longest baseDir) wins.
func (m *IgnoreMatcher) evalGrepaiIgnore(normalizedPath string) (bool, bool, string) {
	var bestMatch *grepaiMatcher
	bestBaseLen := -1

	for i := range m.grepaiMatchers {
		gm := &m.grepaiMatchers[i]
		relPath := matcherRelPath(normalizedPath, gm.baseDir)
		if relPath == "" && gm.baseDir != "" {
			continue // This matcher doesn't apply to this path
		}

		// Check if the "any" matcher detects this path (any pattern matches, positive or negative)
		if gm.any.MatchesPath(relPath) || gm.any.MatchesPath(relPath+"/") {
			if len(gm.baseDir) > bestBaseLen {
				bestMatch = gm
				bestBaseLen = len(gm.baseDir)
			}
		}
	}

	if bestMatch == nil {
		return false, false, "" // No .grepaiignore has an opinion
	}

	relPath := matcherRelPath(normalizedPath, bestMatch.baseDir)
	// The full matcher (with negations) gives the final answer.
	// Check both with and without trailing slash. The trailing-slash variant
	// is more specific (matches directory patterns), so if it says "not ignored"
	// (negation matched), that takes precedence.
	matchPlain := bestMatch.full.MatchesPath(relPath)
	matchSlash := bestMatch.full.MatchesPath(relPath + "/")
	if matchPlain && !matchSlash {
		// The trailing-slash check negated the match → not ignored
		return false, true, bestMatch.baseDir
	}
	return matchPlain || matchSlash, true, bestMatch.baseDir
}

// evalGitIgnore checks extra dirs and .gitignore matchers (original ShouldIgnore logic).
func (m *IgnoreMatcher) evalGitIgnore(normalizedPath string) bool {
	ignored, _ := m.evalGitIgnoreWithLevel(normalizedPath)
	return ignored
}

// evalGitIgnoreWithLevel checks .gitignore/extra patterns and returns the deepest matching level.
func (m *IgnoreMatcher) evalGitIgnoreWithLevel(normalizedPath string) (bool, string) {
	found := false
	deepestBaseDir := ""

	// Check extra directories (root-level, baseDir="")
	base := filepath.Base(normalizedPath)
	for _, dir := range m.extraDirs {
		if base == dir {
			found = true
			break
		}
	}

	// Check nested gitignore patterns, find the deepest match
	for _, nm := range m.nestedMatchers {
		relPath := matcherRelPath(normalizedPath, nm.baseDir)
		if relPath == "" && nm.baseDir != "" {
			continue
		}

		if nm.matcher.MatchesPath(relPath) || nm.matcher.MatchesPath(relPath+"/") {
			if !found || len(nm.baseDir) > len(deepestBaseDir) {
				deepestBaseDir = nm.baseDir
				found = true
			}
		}
	}

	return found, deepestBaseDir
}

// matcherRelPath computes the path relative to a matcher's base directory.
// Returns empty string if the path is outside the matcher's scope (when baseDir is non-empty).
func matcherRelPath(normalizedPath, baseDir string) string {
	if baseDir == "" {
		return normalizedPath
	}
	normalizedBase := filepath.ToSlash(baseDir)
	if normalizedPath == normalizedBase {
		return "."
	}
	if strings.HasPrefix(normalizedPath, normalizedBase+"/") {
		return strings.TrimPrefix(normalizedPath, normalizedBase+"/")
	}
	return "" // Path is outside this matcher's scope
}

// compileGrepaiIgnoreFile reads a .grepaiignore file and compiles two matchers:
// - full: uses original patterns (with negations) for the actual decision
// - any: uses all patterns converted to positive for detecting if the file has an opinion
// Also returns whether the file contains any negation patterns.
func compileGrepaiIgnoreFile(path string) (grepaiMatcher, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return grepaiMatcher{}, false, err
	}

	lines := strings.Split(string(content), "\n")
	fullLines := make([]string, 0, len(lines))
	anyLines := make([]string, 0, len(lines))
	hasNegations := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fullLines = append(fullLines, trimmed)

		if strings.HasPrefix(trimmed, "!") {
			hasNegations = true
			// Strip the ! to make it a positive match for detection
			anyLines = append(anyLines, strings.TrimPrefix(trimmed, "!"))
		} else {
			anyLines = append(anyLines, trimmed)
		}
	}

	fullMatcher := ignore.CompileIgnoreLines(fullLines...)
	anyMatcher := ignore.CompileIgnoreLines(anyLines...)

	return grepaiMatcher{
		full: fullMatcher,
		any:  anyMatcher,
	}, hasNegations, nil
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
