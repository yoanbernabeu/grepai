//go:build !windows
// +build !windows

package daemon

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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

// livenessCheck uses a pipe to detect child process exit.
// The write end is inherited by the child; when it exits the kernel closes
// all its FDs, giving EOF on the parent's read end. This reliably detects
// exit regardless of zombie state or process group settings.
type livenessCheck struct {
	pr, pw *os.File
}

func newLivenessCheck() (*livenessCheck, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create liveness pipe: %w", err)
	}
	return &livenessCheck{pr: pr, pw: pw}, nil
}

func (l *livenessCheck) configureCmd(cmd *exec.Cmd) {
	cmd.ExtraFiles = []*os.File{l.pw}
}

// start closes the write end in the parent and begins monitoring.
// Returns a channel that is closed when the child exits.
func (l *livenessCheck) start(_ int) <-chan struct{} {
	l.pw.Close()
	ch := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		if _, err := l.pr.Read(buf); err != nil && err != io.EOF {
			// Ignore read errors: liveness only needs unblocking on exit/close.
			_ = err
		}
		l.pr.Close()
		close(ch)
	}()
	return ch
}

func (l *livenessCheck) cleanup() {
	l.pr.Close()
	l.pw.Close()
}

// StopProcess sends SIGINT to the process with the given PID.
func StopProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send interrupt signal: %w", err)
	}

	return nil
}

// StopChannel returns a channel that never fires on Unix.
// Signal-based shutdown is handled via os/signal, so no additional
// mechanism is needed.
func StopChannel() <-chan struct{} {
	return make(chan struct{})
}
