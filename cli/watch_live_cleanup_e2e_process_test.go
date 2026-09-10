//go:build e2e

package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type liveWatchProcess struct {
	t       *testing.T
	cmd     *exec.Cmd
	output  *lockedBuffer
	done    chan struct{}
	stopOne sync.Once
	errMu   sync.Mutex
	waitErr error
}

func (h *liveCleanupHarness) startWatch() *liveWatchProcess {
	p := &liveWatchProcess{t: h.t, output: &lockedBuffer{}, done: make(chan struct{})}
	p.cmd = exec.Command(h.bin, "watch", "--no-ui", "--log-dir", filepath.Join(filepath.Dir(h.root), "logs"))
	p.cmd.Dir, p.cmd.Env = h.root, h.env
	p.cmd.Stdout, p.cmd.Stderr = p.output, p.output
	if err := p.cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	go func() {
		err := p.cmd.Wait()
		p.errMu.Lock()
		p.waitErr = err
		p.errMu.Unlock()
		close(p.done)
	}()
	h.t.Cleanup(p.stop)
	return p
}

func (p *liveWatchProcess) waitFor(marker string) {
	deadline := time.NewTimer(liveCleanupTimeout)
	defer deadline.Stop()
	for {
		if strings.Contains(p.output.String(), marker) {
			return
		}
		select {
		case <-p.done:
			p.t.Fatalf("watch exited before %q: %v\n%s", marker, p.err(), p.output.String())
		case <-time.After(20 * time.Millisecond):
		case <-deadline.C:
			p.t.Fatalf("timeout waiting for %q\n%s", marker, p.output.String())
		}
	}
}

func (p *liveWatchProcess) assertRunning(pid int) {
	if p.cmd.Process == nil || p.cmd.Process.Pid != pid {
		p.t.Fatalf("watch PID changed, want %d", pid)
	}
	select {
	case <-p.done:
		p.t.Fatalf("watch PID %d exited: %v\n%s", pid, p.err(), p.output.String())
	default:
	}
	if err := p.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		p.t.Fatalf("watch PID %d is not running: %v", pid, err)
	}
}

func (p *liveWatchProcess) err() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.waitErr
}

func (p *liveWatchProcess) stop() {
	p.stopOne.Do(func() {
		select {
		case <-p.done:
			return
		default:
		}
		_ = p.cmd.Process.Signal(os.Interrupt)
		select {
		case <-p.done:
		case <-time.After(20 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
			p.t.Errorf("watch required kill\n%s", p.output.String())
		}
	})
}

type lockedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
