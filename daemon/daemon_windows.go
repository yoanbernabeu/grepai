//go:build windows
// +build windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
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
// On Windows, returns nil as no special process attributes are needed for background spawning.
func sysProcAttr() *syscall.SysProcAttr {
	return nil
}
