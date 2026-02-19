//go:build windows
// +build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess         = kernel32.NewProc("OpenProcess")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
	procLockFileEx          = kernel32.NewProc("LockFileEx")
	processQueryLimitedInfo = uint32(0x1000)
)

const (
	// LockFileEx flags
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// IsProcessRunning checks if a process with the given PID is running on Windows.
// Uses OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION to check process existence.
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Try to open process handle with minimal access rights
	handle, _, _ := procOpenProcess.Call(
		uintptr(processQueryLimitedInfo),
		uintptr(0),
		uintptr(pid),
	)

	if handle == 0 {
		// Failed to open process (doesn't exist or no permission)
		return false
	}

	// Close the handle
	procCloseHandle.Call(handle)
	return true
}

// lockFile acquires an exclusive lock on the given file.
// On Windows, uses LockFileEx with LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY.
// The lock is held for the lifetime of the process and released by the OS on exit.
// Returns an error if the lock cannot be acquired (e.g., another process holds it).
func lockFile(f *os.File) error {
	// OVERLAPPED structure for LockFileEx (can be zeroed for synchronous operation)
	var overlapped syscall.Overlapped

	// LockFileEx(hFile, dwFlags, dwReserved, nNumberOfBytesToLockLow, nNumberOfBytesToLockHigh, lpOverlapped)
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, // Lock 1 byte (we just need any lock)
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if ret == 0 {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	return nil
}

// sysProcAttr returns platform-specific process attributes for spawning background processes.
// On Windows, uses CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW to fully detach the child
// from the parent's console. Without CREATE_NO_WINDOW, the child still receives
// CTRL_CLOSE_EVENT (mapped to SIGTERM by Go) when the parent's console closes.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
	}
}

// livenessCheck uses polling on Windows since ExtraFiles is not supported.
// Windows doesn't have zombie processes, so IsProcessRunning is reliable.
type livenessCheck struct{}

func newLivenessCheck() (*livenessCheck, error) {
	return &livenessCheck{}, nil
}

func (l *livenessCheck) configureCmd(cmd *exec.Cmd) {
	// no-op: ExtraFiles not supported on Windows
}

// start polls IsProcessRunning to detect child exit.
// Returns a channel that is closed when the child exits.
func (l *livenessCheck) start(pid int) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for {
			time.Sleep(250 * time.Millisecond)
			if !IsProcessRunning(pid) {
				close(ch)
				return
			}
		}
	}()
	return ch
}

func (l *livenessCheck) cleanup() {
	// no-op
}

const (
	stopFilePrefix    = "grepai-stop-"
	stopPollInterval  = 500 * time.Millisecond
	stopStaleAfter    = 60 * time.Second
)

// stopFilePath returns the path to the sentinel stop file for the given PID.
func stopFilePath(pid int) (string, error) {
	logDir, err := GetDefaultLogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(logDir, fmt.Sprintf("%s%d", stopFilePrefix, pid)), nil
}

// StopProcess writes a sentinel stop file that the daemon polls for.
// This avoids os.Interrupt which is not supported cross-console on Windows.
func StopProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	if !IsProcessRunning(pid) {
		return fmt.Errorf("process %d is not running", pid)
	}

	path, err := stopFilePath(pid)
	if err != nil {
		return fmt.Errorf("failed to determine stop file path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0600); err != nil {
		return fmt.Errorf("failed to write stop file: %w", err)
	}

	return nil
}

// StopChannel returns a channel that is closed when a stop file is detected
// for the current process. It also cleans up any stale stop files from
// previous runs on startup.
func StopChannel() <-chan struct{} {
	ch := make(chan struct{})
	pid := os.Getpid()

	path, err := stopFilePath(pid)
	if err != nil {
		// Can't determine path; return inert channel.
		return ch
	}

	// Clean up stale stop file from a previous run that reused this PID.
	_ = os.Remove(path)

	go func() {
		for {
			time.Sleep(stopPollInterval)
			if _, err := os.Stat(path); err == nil {
				// Stop file detected — remove it and signal shutdown.
				_ = os.Remove(path)
				close(ch)
				return
			}
		}
	}()

	return ch
}
