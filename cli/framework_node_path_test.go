package cli

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

// A project config must not be able to make grepai execute a file shipped
// inside the project through framework_processing.node_path.
func TestBuildFrameworkRegistry_IgnoresNodePathInsideProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake node binary")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "MARKER")
	script := filepath.Join(root, "tools", "node")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\ntouch '" + marker + "'\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, nodePath := range []string{"./tools/node", "tools/node", script} {
		t.Run(nodePath, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Framework.NodePath = nodePath

			var logs bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(prev) })

			t.Chdir(root)
			reg := buildFrameworkRegistry(cfg, root)
			_, _ = reg.TransformForEmbedding(context.Background(), "App.vue", "<template><div/></template>\n")

			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("node_path %q inside the project was executed", nodePath)
			}
			if out := logs.String(); !strings.Contains(out, "framework_processing.node_path") || !strings.Contains(out, `using "node" instead`) {
				t.Fatalf("expected a warning about the ignored node_path, got %q", out)
			}
		})
	}
}
