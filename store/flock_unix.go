//go:build !windows
// +build !windows

package store

import (
	"fmt"
	"os"
	"syscall"
)

// flockExclusive acquires an exclusive (write) lock on the file.
// Blocks until the lock is available.
func flockExclusive(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

// flockShared acquires a shared (read) lock on the file.
// Multiple readers can hold shared locks simultaneously.
func flockShared(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	return nil
}

// funlock releases the lock on the file.
func funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
