//go:build unix

package indexer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSnapshotFIFOHelper(t *testing.T) {
	path := os.Getenv("GREPAI_TEST_FIFO")
	if path == "" {
		return
	}

	// Repeatedly replace a regular path with a FIFO and back. This exercises
	// the TOCTOU window where the preflight stat sees a regular file but open
	// resolves the replacement FIFO; O_NONBLOCK must keep every attempt bounded.
	done := make(chan error, 1)
	go func() {
		for i := range 2_000 {
			tmp := fmt.Sprintf("%s.swap-%d", path, i)
			var err error
			if i%2 == 0 {
				err = unix.Mkfifo(tmp, 0o600)
			} else {
				err = os.WriteFile(tmp, []byte("package regular\n"), 0o600)
			}
			if err == nil {
				err = os.Rename(tmp, path)
			}
			if err != nil {
				_ = os.Remove(tmp)
				done <- err
				return
			}
			runtime.Gosched()
		}
		done <- nil
	}()
	for range 2_000 {
		_, _ = readFileSnapshot(path, "pipe.go")
		runtime.Gosched()
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReadFileSnapshotDoesNotBlockOnFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.go")
	if err := os.WriteFile(path, []byte("package regular\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSnapshotFIFOHelper$")
	cmd.Env = append(os.Environ(), "GREPAI_TEST_FIFO="+path)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("FIFO snapshot blocked until timeout: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("FIFO helper failed: %v\n%s", err, output)
	}
}
