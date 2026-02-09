package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/rpg"
)

type stubEmbedder struct {
	dim int
}

func (s *stubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	_ = text
	vec := make([]float32, s.dim)
	for i := range vec {
		vec[i] = float32(i + 1)
	}
	return vec, nil
}

func (s *stubEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	_ = ctx
	out := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, s.dim)
		for j := range vec {
			vec[j] = float32(j + 1)
		}
		out[i] = vec
	}
	return out, nil
}

func (s *stubEmbedder) Dimensions() int {
	return s.dim
}

func (s *stubEmbedder) Close() error {
	return nil
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func normalizedPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

func TestWatchProject_MultiWorktreeRPGIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	mainRoot := t.TempDir()
	runGit(t, mainRoot, "init")
	runGit(t, mainRoot, "config", "user.email", "test@example.com")
	runGit(t, mainRoot, "config", "user.name", "test")

	mainCode := "package main\n\nfunc helper() {}\n\nfunc main() { helper() }\n"
	if err := os.WriteFile(filepath.Join(mainRoot, "main.go"), []byte(mainCode), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}
	runGit(t, mainRoot, "add", "main.go")
	runGit(t, mainRoot, "commit", "-m", "init")

	wtParent := t.TempDir()
	linkedRoot := filepath.Join(wtParent, "linked")
	runGit(t, mainRoot, "worktree", "add", "-b", "feature/rpg-integration", linkedRoot)

	linkedCode := "package main\n\nfunc helper() {}\n\nfunc linkedOnly() { helper() }\n\nfunc main() { linkedOnly() }\n"
	if err := os.WriteFile(filepath.Join(linkedRoot, "main.go"), []byte(linkedCode), 0644); err != nil {
		t.Fatalf("failed to write linked main.go: %v", err)
	}

	cfg := config.DefaultConfig()
	dim := 8
	cfg.Embedder.Dimensions = &dim
	cfg.RPG.Enabled = true
	cfg.RPG.FeatureMode = "local"
	cfg.RPG.FeatureGroupStrategy = "sample"
	cfg.Store.Backend = "gob"
	if err := cfg.Save(mainRoot); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	discovered := discoverWorktreesForWatch(mainRoot)
	if len(discovered) != 1 || normalizedPath(discovered[0]) != normalizedPath(linkedRoot) {
		t.Fatalf("discoverWorktreesForWatch() = %v, want [%s]", discovered, linkedRoot)
	}
	if !config.Exists(linkedRoot) {
		t.Fatalf("expected linked worktree to be auto-initialized with .grepai config")
	}

	gitignoreBytes, err := os.ReadFile(filepath.Join(linkedRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read linked .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignoreBytes), ".grepai/") {
		t.Fatalf("linked .gitignore does not contain .grepai/ entry")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emb := &stubEmbedder{dim: dim}
	projects := []string{mainRoot, linkedRoot}
	readyCh := make(chan string, len(projects))
	errCh := make(chan error, len(projects))

	var wg sync.WaitGroup
	for _, root := range projects {
		root := root
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := watchProject(ctx, root, emb, true, func() {
				readyCh <- root
			})
			errCh <- err
		}()
	}

	ready := map[string]bool{}
	deadline := time.After(30 * time.Second)
	for len(ready) < len(projects) {
		select {
		case root := <-readyCh:
			ready[root] = true
		case <-deadline:
			t.Fatalf("timeout waiting for watchProject readiness; ready=%v", ready)
		}
	}
	cancel()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("watchProject returned error: %v", err)
		}
	}

	statsByRoot := make(map[string]int)
	for _, root := range projects {
		indexPath := config.GetRPGIndexPath(root)
		info, statErr := os.Stat(indexPath)
		if statErr != nil {
			t.Fatalf("expected RPG index at %s: %v", indexPath, statErr)
		}
		if info.Size() == 0 {
			t.Fatalf("RPG index file is empty: %s", indexPath)
		}

		rpgStore := rpg.NewGOBRPGStore(indexPath)
		if err := rpgStore.Load(context.Background()); err != nil {
			t.Fatalf("failed to load RPG index %s: %v", indexPath, err)
		}
		graphStats := rpgStore.GetGraph().Stats()
		_ = rpgStore.Close()

		if graphStats.TotalNodes == 0 {
			t.Fatalf("RPG graph has no nodes for %s", root)
		}
		statsByRoot[root] = graphStats.TotalNodes
	}

	if statsByRoot[linkedRoot] <= statsByRoot[mainRoot] {
		t.Fatalf("expected linked worktree RPG nodes (%d) > main (%d)", statsByRoot[linkedRoot], statsByRoot[mainRoot])
	}

	t.Logf("RPG node counts: main=%d linked=%d", statsByRoot[mainRoot], statsByRoot[linkedRoot])
	t.Logf("main=%s linked=%s", mainRoot, linkedRoot)
}

func TestWatchProject_MultiWorktreeRPGDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "gob"
	cfg.RPG.Enabled = false
	dim := 8
	cfg.Embedder.Dimensions = &dim
	if err := cfg.Save(root); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchProject(ctx, root, &stubEmbedder{dim: dim}, true, func() {
			close(ready)
		})
	}()

	select {
	case <-ready:
		cancel()
	case <-time.After(20 * time.Second):
		cancel()
		t.Fatal("timeout waiting for watchProject readiness")
	}

	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("watchProject returned error: %v", err)
	}

	if _, err := os.Stat(config.GetRPGIndexPath(root)); err == nil {
		t.Fatalf("unexpected RPG index file when RPG disabled")
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed to stat RPG index path: %v", err)
	}
}

func TestDiscoverWorktreesForWatch_DiscoversLinkedOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")

	wtRoot := filepath.Join(t.TempDir(), "wt")
	runGit(t, root, "worktree", "add", "-b", "feature/parity", wtRoot)

	cfg := config.DefaultConfig()
	if err := cfg.Save(root); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	discovered := discoverWorktreesForWatch(root)
	if len(discovered) != 1 || normalizedPath(discovered[0]) != normalizedPath(wtRoot) {
		t.Fatalf("discoverWorktreesForWatch() = %v, want [%s]", discovered, wtRoot)
	}
}
