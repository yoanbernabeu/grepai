//go:build !windows
// +build !windows

package daemon

import (
	"testing"
	"time"
)

func TestLivenessCheckStart_ClosesOnReadError(t *testing.T) {
	l, err := newLivenessCheck()
	if err != nil {
		t.Fatalf("newLivenessCheck failed: %v", err)
	}
	defer l.cleanup()

	// Force a deterministic read-error path by closing the read end
	// before the reader goroutine starts.
	if err := l.pr.Close(); err != nil {
		t.Fatalf("failed to close read pipe: %v", err)
	}
	ch := l.start(0)

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for liveness channel to close")
	}
}
