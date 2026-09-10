//go:build e2e

package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
)

const liveCleanupTimeout = 90 * time.Second

type liveCleanupHarness struct {
	t      *testing.T
	root   string
	bin    string
	env    []string
	server *httptest.Server
}

func buildLiveCleanupCandidate(t *testing.T) string {
	t.Helper()
	top := t.TempDir()
	moduleCache := os.Getenv("GOMODCACHE")
	if moduleCache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		moduleCache = filepath.Join(home, "go", "pkg", "mod")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repo := filepath.Dir(filepath.Dir(sourceFile))
	bin := filepath.Join(top, "grepai")
	ctx, cancel := context.WithTimeout(context.Background(), liveCleanupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-p", "1", "-o", bin, "./cmd/grepai")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", "GOMODCACHE="+moduleCache, "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build candidate: %v\n%s", err, output)
	}
	return bin
}

func newLiveCleanupHarness(t *testing.T, bin string) *liveCleanupHarness {
	t.Helper()
	top := t.TempDir()
	h := &liveCleanupHarness{t: t, root: filepath.Join(top, "project"), bin: bin}
	for _, dir := range []string{h.root, filepath.Join(top, "home"), filepath.Join(top, "config"), filepath.Join(top, "cache"), filepath.Join(top, "state"), filepath.Join(top, "logs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h.env = append(os.Environ(), "HOME="+filepath.Join(top, "home"), "XDG_CONFIG_HOME="+filepath.Join(top, "config"), "XDG_CACHE_HOME="+filepath.Join(top, "cache"), "XDG_STATE_HOME="+filepath.Join(top, "state"))
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"e2e-model"}]}`))
		case "/api/embeddings":
			var request struct {
				Prompt string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			base := float32(len(request.Prompt)%7 + 1)
			_ = json.NewEncoder(w).Encode(map[string][]float32{"embedding": {base, 1, 2, 3, 4, 5, 6, 7}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(h.server.Close)
	return h
}

func (h *liveCleanupHarness) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), liveCleanupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.bin, args...)
	cmd.Dir, cmd.Env = h.root, h.env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (h *liveCleanupHarness) init() {
	output, err := h.run("init", "--provider", "ollama", "--model", "e2e-model", "--backend", "gob", "--yes")
	if err != nil {
		h.t.Fatalf("init: %v\n%s", err, output)
	}
	cfg, err := config.Load(h.root)
	if err != nil {
		h.t.Fatal(err)
	}
	dimensions := 8
	cfg.Embedder.Endpoint = h.server.URL
	cfg.Embedder.Dimensions = &dimensions
	cfg.Watch.DebounceMs = 25
	if err := cfg.Save(h.root); err != nil {
		h.t.Fatal(err)
	}
}

func (h *liveCleanupHarness) write(name, contents string) {
	path := filepath.Join(h.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		h.t.Fatal(err)
	}
}
