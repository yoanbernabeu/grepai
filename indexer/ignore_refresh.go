package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// RefreshSubtree replaces ignore rules sourced from relRoot without walking
// unrelated project directories. Rules inherited from ancestors are retained.
func (m *IgnoreMatcher) RefreshSubtree(relRoot string) error {
	relRoot = filepath.Clean(relRoot)
	if relRoot == "." || relRoot == "" {
		return m.Refresh()
	}
	separator := string(filepath.Separator)
	if filepath.IsAbs(relRoot) || relRoot == ".." || strings.HasPrefix(relRoot, ".."+separator) {
		return fmt.Errorf("ignore refresh subtree escapes project root: %s", relRoot)
	}

	m.mu.RLock()
	root := m.projectRoot
	extraDirs := append([]string(nil), m.extraDirs...)
	walk := m.walkIgnoreFiles
	read := m.readIgnoreFile
	m.mu.RUnlock()
	gitMatchers, grepaiMatchers, err := loadScopedIgnoreMatchers(root, filepath.Join(root, relRoot), extraDirs, walk, read)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.nestedMatchers = replaceNestedMatchers(m.nestedMatchers, relRoot, gitMatchers)
	m.grepaiMatchers = replaceGrepaiMatchers(m.grepaiMatchers, relRoot, grepaiMatchers)
	m.mu.Unlock()
	return nil
}

// Refresh transactionally reloads all ignore policy. Collection errors leave
// the previous policy intact.
func (m *IgnoreMatcher) Refresh() error {
	m.mu.RLock()
	root := m.projectRoot
	externalGitignore := m.externalGitignore
	extraDirs := append([]string(nil), m.extraDirs...)
	walk := m.walkIgnoreFiles
	read := m.readIgnoreFile
	m.mu.RUnlock()

	gitMatchers, grepaiMatchers, err := loadScopedIgnoreMatchers(root, root, extraDirs, walk, read)
	if err != nil {
		return err
	}
	if externalGitignore != "" {
		path := expandTilde(externalGitignore)
		content, err := read(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read external gitignore %s: %w", path, err)
		}
		if err == nil {
			matcher := ignore.CompileIgnoreLines(strings.Split(string(content), "\n")...)
			gitMatchers = append([]nestedMatcher{{matcher: matcher}}, gitMatchers...)
		}
	}
	if len(extraDirs) > 0 {
		gitMatchers = append(gitMatchers, nestedMatcher{
			matcher: ignore.CompileIgnoreLines(extraDirs...),
		})
	}

	m.mu.Lock()
	m.nestedMatchers = gitMatchers
	m.grepaiMatchers = grepaiMatchers
	m.mu.Unlock()
	return nil
}

func loadScopedIgnoreMatchers(projectRoot, scanRoot string, extraDirs []string, walk func(string, filepath.WalkFunc) error, read func(string) ([]byte, error)) ([]nestedMatcher, []grepaiMatcher, error) {
	var gitMatchers []nestedMatcher
	var grepaiMatchers []grepaiMatcher
	err := walk(scanRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			for _, dir := range extraDirs {
				if filepath.Base(path) == dir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		baseDir, err := filepath.Rel(projectRoot, filepath.Dir(path))
		if err != nil {
			return nil
		}
		if baseDir == "." {
			baseDir = ""
		}
		switch filepath.Base(path) {
		case ".gitignore":
			content, err := read(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return fmt.Errorf("read gitignore %s: %w", path, err)
			}
			matcher := ignore.CompileIgnoreLines(strings.Split(string(content), "\n")...)
			gitMatchers = append(gitMatchers, nestedMatcher{matcher: matcher, baseDir: baseDir})
		case ".grepaiignore":
			content, err := read(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return fmt.Errorf("read grepaiignore %s: %w", path, err)
			}
			matcher, _, err := compileGrepaiIgnoreContent(content)
			if err != nil {
				return fmt.Errorf("parse grepaiignore %s: %w", path, err)
			}
			matcher.baseDir = baseDir
			grepaiMatchers = append(grepaiMatchers, matcher)
		}
		return nil
	})
	return gitMatchers, grepaiMatchers, err
}

func replaceNestedMatchers(current []nestedMatcher, relRoot string, replacements []nestedMatcher) []nestedMatcher {
	kept := current[:0]
	for _, matcher := range current {
		if !inIgnoreSubtree(matcher.baseDir, relRoot) {
			kept = append(kept, matcher)
		}
	}
	return append(kept, replacements...)
}

func replaceGrepaiMatchers(current []grepaiMatcher, relRoot string, replacements []grepaiMatcher) []grepaiMatcher {
	kept := current[:0]
	for _, matcher := range current {
		if !inIgnoreSubtree(matcher.baseDir, relRoot) {
			kept = append(kept, matcher)
		}
	}
	return append(kept, replacements...)
}

func inIgnoreSubtree(baseDir, relRoot string) bool {
	return baseDir == relRoot || strings.HasPrefix(baseDir, relRoot+string(filepath.Separator))
}
