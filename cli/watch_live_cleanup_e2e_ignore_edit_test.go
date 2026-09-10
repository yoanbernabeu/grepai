//go:build e2e

package cli_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchLiveCleanupReloadsEditedIgnorePolicy(t *testing.T) {
	bin := buildLiveCleanupCandidate(t)
	h := newLiveCleanupHarness(t, bin)

	h.write(".gitignore", "hidden/\n")
	h.write(".grepaiignore", "!kept.go\n")
	h.write("visible.go", "package liveignore\nfunc VisibleTarget() {}\nfunc VisibleCaller() { VisibleTarget() }\n")
	h.write("kept.go", "package liveignore\nfunc KeptTarget() {}\nfunc KeptCaller() { KeptTarget() }\n")
	h.write("hidden/secret.go", "package hidden\nfunc SecretTarget() {}\nfunc SecretCaller() { SecretTarget() }\n")
	h.init()

	watch := h.startWatch()
	watch.waitFor("Watching for changes")
	pid := watch.cmd.Process.Pid
	h.awaitState(watch, pid, liveCleanupExpectation{
		presentFiles: []string{"visible.go", "kept.go"},
		absentFiles:  []string{"hidden/secret.go"},
		presentSymbols: map[string][]string{
			"visible.go": {"VisibleTarget"},
			"kept.go":    {"KeptTarget"},
		},
		absentSymbolFiles: []string{"hidden/secret.go"},
		presentTraces:     map[string]string{"VisibleTarget": "visible.go"},
		absentTraces:      []string{"SecretTarget"},
	})

	if err := os.WriteFile(filepath.Join(h.root, ".gitignore"), []byte("hidden/\nvisible.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".grepaiignore"), []byte("!hidden/**\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.awaitState(watch, pid, liveCleanupExpectation{
		presentFiles: []string{"kept.go", "hidden/secret.go"},
		absentFiles:  []string{"visible.go"},
		presentSymbols: map[string][]string{
			"kept.go":          {"KeptTarget"},
			"hidden/secret.go": {"SecretTarget"},
		},
		absentSymbolFiles: []string{"visible.go"},
		presentTraces:     map[string]string{"SecretTarget": "hidden/secret.go"},
		absentTraces:      []string{"VisibleTarget"},
	})
	watch.assertRunning(pid)

	for _, path := range []string{"visible.go", "kept.go", "hidden/secret.go"} {
		if info, err := os.Stat(filepath.Join(h.root, filepath.FromSlash(path))); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("source file %q was changed or removed: info=%v err=%v", path, info, err)
		}
	}

	h.write("hidden/new.go", "package hidden\nfunc NewlyEligibleTarget() {}\nfunc NewlyEligibleCaller() { NewlyEligibleTarget() }\n")
	h.awaitState(watch, pid, liveCleanupExpectation{
		presentFiles: []string{"kept.go", "hidden/secret.go", "hidden/new.go"},
		absentFiles:  []string{"visible.go"},
		presentSymbols: map[string][]string{
			"kept.go":          {"KeptTarget"},
			"hidden/secret.go": {"SecretTarget"},
			"hidden/new.go":    {"NewlyEligibleTarget"},
		},
		absentSymbolFiles: []string{"visible.go"},
		presentTraces:     map[string]string{"NewlyEligibleTarget": "hidden/new.go"},
		absentTraces:      []string{"VisibleTarget"},
	})
	watch.assertRunning(pid)
	// Stop only after both persisted transitions and the newly registered
	// directory event have been proved through the real CLI.
	watch.stop()
}
