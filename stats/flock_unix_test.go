//go:build !windows

package stats

import (
	"os"
	"testing"
)

func TestFlockExclusive_Error(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "flock-test-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	// Fermer le fichier pour invalider le fd — syscall.Flock retournera EBADF.
	f.Close()

	if err := flockExclusive(f); err == nil {
		t.Fatal("flockExclusive() expected error on closed fd, got nil")
	}
}
