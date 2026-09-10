//go:build e2e

package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestWatchLiveCleanupPersistsWhileProcessRemainsRunning(t *testing.T) {
	bin := buildLiveCleanupCandidate(t)
	for _, scenario := range []struct {
		name            string
		deleteDirectory bool
	}{
		{name: "plain-file"},
		{name: "nested-directory", deleteDirectory: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h := newLiveCleanupHarness(t, bin)
			for _, dir := range []string{filepath.Join(h.root, "src", "nested"), filepath.Join(h.root, "src-old")} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			h.init()
			watch := h.startWatch()
			watch.waitFor("Watching for changes")
			pid := watch.cmd.Process.Pid
			watch.assertRunning(pid)

			h.write("src/nested/delete.go", "package nested\nfunc LiveDeleteTarget() {}\nfunc LiveDeleteCaller() { LiveDeleteTarget() }\n")
			h.write("src-old/keep.go", "package srcold\nfunc LiveKeepTarget() {}\nfunc LiveKeepCaller() { LiveKeepTarget() }\n")
			h.awaitState(watch, pid, liveCleanupExpectation{
				presentFiles:  []string{"src/nested/delete.go", "src-old/keep.go"},
				presentTraces: map[string]string{"LiveDeleteTarget": "src/nested/delete.go", "LiveKeepTarget": "src-old/keep.go"},
			})

			if scenario.deleteDirectory {
				if err := os.RemoveAll(filepath.Join(h.root, "src")); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(filepath.Join(h.root, "src", "nested", "delete.go")); err != nil {
				t.Fatal(err)
			}
			h.awaitState(watch, pid, liveCleanupExpectation{
				presentFiles: []string{"src-old/keep.go"}, absentFiles: []string{"src/nested/delete.go"},
				presentTraces: map[string]string{"LiveKeepTarget": "src-old/keep.go"}, absentTraces: []string{"LiveDeleteTarget"},
			})
			watch.assertRunning(pid)
			if _, err := os.Stat(filepath.Join(h.root, "src-old", "keep.go")); err != nil {
				t.Fatalf("boundary sibling removed: %v", err)
			}
			// Persistent document, chunk, symbol, reference, and real trace CLI
			// assertions deliberately complete before the watcher is stopped.
			watch.stop()
		})
	}
}

func TestWatchLiveCleanupCodexCorrections(t *testing.T) {
	bin := buildLiveCleanupCandidate(t)

	t.Run("move-in-populated-tree-with-live-ignore-refresh", func(t *testing.T) {
		h := newLiveCleanupHarness(t, bin)
		h.init()
		cfg, err := config.Load(h.root)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Chunking.CustomExtensions = append(cfg.Chunking.CustomExtensions, ".codex")
		if err := cfg.Save(h.root); err != nil {
			t.Fatal(err)
		}

		watch := h.startWatch()
		watch.waitFor("Watching for changes")
		pid := watch.cmd.Process.Pid
		watch.assertRunning(pid)

		staged := filepath.Join(filepath.Dir(h.root), "staged-import")
		if err := os.MkdirAll(filepath.Join(staged, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staged, "nested", ".gitignore"), []byte("git-drop.go\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staged, "nested", ".grepaiignore"), []byte("grepai-drop.go\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(staged, "vendor"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staged, ".grepaiignore"), []byte("vendor/**\n!vendor/keep.go\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		want := liveCleanupExpectation{presentSymbols: make(map[string][]string)}
		for i := 0; i < 305; i++ {
			name := fmt.Sprintf("file-%03d.go", i)
			target := fmt.Sprintf("ImportedTarget%03d", i)
			path := filepath.Join(staged, name)
			contents := fmt.Sprintf("package imported\nfunc %s() {}\nfunc ImportedCaller%03d() { %s() }\n", target, i, target)
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			rel := "imported/" + name
			want.presentFiles = append(want.presentFiles, rel)
			want.presentSymbols[rel] = []string{target}
		}
		if err := os.WriteFile(filepath.Join(staged, "notes.codex"), []byte("custom extension live import\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for name, contents := range map[string]string{
			"nested/git-drop.go":    "package nested\nfunc GitIgnoredTarget() {}\n",
			"nested/grepai-drop.go": "package nested\nfunc GrepaiIgnoredTarget() {}\n",
			"vendor/drop.go":        "package vendor\nfunc VendorIgnoredTarget() {}\n",
			"vendor/keep.go":        "package vendor\nfunc VendorKeepTarget() {}\nfunc VendorKeepCaller() { VendorKeepTarget() }\n",
		} {
			path := filepath.Join(staged, filepath.FromSlash(name))
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want.presentFiles = append(want.presentFiles, "imported/notes.codex", "imported/vendor/keep.go")
		want.presentSymbols["imported/vendor/keep.go"] = []string{"VendorKeepTarget"}
		want.absentFiles = []string{"imported/nested/git-drop.go", "imported/nested/grepai-drop.go", "imported/vendor/drop.go"}
		want.absentSymbolFiles = append([]string(nil), want.absentFiles...)
		want.presentTraces = map[string]string{"ImportedTarget304": "imported/file-304.go"}

		if err := os.Rename(staged, filepath.Join(h.root, "imported")); err != nil {
			t.Fatal(err)
		}
		h.awaitState(watch, pid, want)
		watch.assertRunning(pid)
		if output := watch.output.String(); containsDroppedEventWarning(output) {
			t.Fatalf("watch dropped an event from the populated move:\n%s", output)
		}
		watch.stop()
	})

	t.Run("directory-replaced-by-regular-file-in-one-burst", func(t *testing.T) {
		h := newLiveCleanupHarness(t, bin)
		if err := os.MkdirAll(filepath.Join(h.root, "replace.go", "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		h.init()
		watch := h.startWatch()
		watch.waitFor("Watching for changes")
		pid := watch.cmd.Process.Pid

		h.write("replace.go/nested/old.go", "package old\nfunc ReplaceOldTarget() {}\nfunc ReplaceOldCaller() { ReplaceOldTarget() }\n")
		h.write("sibling.go", "package sibling\nfunc ReplaceSiblingTarget() {}\nfunc ReplaceSiblingCaller() { ReplaceSiblingTarget() }\n")
		h.awaitState(watch, pid, liveCleanupExpectation{
			presentFiles: []string{"replace.go/nested/old.go", "sibling.go"},
			presentSymbols: map[string][]string{
				"replace.go/nested/old.go": {"ReplaceOldTarget"},
				"sibling.go":               {"ReplaceSiblingTarget"},
			},
			presentTraces: map[string]string{"ReplaceOldTarget": "replace.go/nested/old.go"},
		})

		movedOut := filepath.Join(filepath.Dir(h.root), "replace-old")
		if err := os.Rename(filepath.Join(h.root, "replace.go"), movedOut); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.root, "replace.go"), []byte("package replacement\nfunc ReplacementTarget() {}\nfunc ReplacementCaller() { ReplacementTarget() }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		h.awaitState(watch, pid, liveCleanupExpectation{
			presentFiles: []string{"replace.go", "sibling.go"},
			absentFiles:  []string{"replace.go/nested/old.go"},
			presentSymbols: map[string][]string{
				"replace.go": {"ReplacementTarget"},
				"sibling.go": {"ReplaceSiblingTarget"},
			},
			absentSymbolFiles: []string{"replace.go/nested/old.go"},
			presentTraces:     map[string]string{"ReplacementTarget": "replace.go"},
			absentTraces:      []string{"ReplaceOldTarget"},
		})
		watch.assertRunning(pid)
		watch.stop()
	})
}

func containsDroppedEventWarning(output string) bool {
	return strings.Contains(output, "Event channel full") || strings.Contains(output, "dropping event")
}
