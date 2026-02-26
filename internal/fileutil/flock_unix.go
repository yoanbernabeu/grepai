//go:build !windows
// +build !windows

package fileutil

import (
	"fmt"
	"os"
	"syscall"
)

// FlockExclusive acquires an exclusive (write) lock on the file.
// If nonBlocking is true, returns immediately with an error if the lock cannot be acquired.
func FlockExclusive(f *os.File, nonBlocking bool) error {
	flags := syscall.LOCK_EX
	if nonBlocking {
		flags |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), flags); err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

// FlockShared acquires a shared (read) lock on the file.
// If nonBlocking is true, returns immediately with an error if the lock cannot be acquired.
func FlockShared(f *os.File, nonBlocking bool) error {
	flags := syscall.LOCK_SH
	if nonBlocking {
		flags |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), flags); err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	return nil
}

// Funlock releases the lock on the file.
func Funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
