//go:build !windows
// +build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// IsProcessRunning checks if a process with the given PID is running on Unix systems.
// Uses signal(0) which returns an error if the process doesn't exist or we don't have permission.
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Try to send signal 0 (null signal) to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, Signal(0) checks if the process exists without actually sending a signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// lockFile acquires an exclusive lock on the given file.
// On Unix, uses flock(2) with LOCK_EX|LOCK_NB for non-blocking exclusive lock.
// The lock is held for the lifetime of the process and released by the OS on exit.
// Returns an error if the lock cannot be acquired (e.g., another process holds it).
func lockFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	return nil
}

// sysProcAttr returns platform-specific process attributes for spawning background processes.
// On Unix, sets Setpgid to detach the child from the parent's process group.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
