//go:build windows
// +build windows

package fileutil

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
	procUnlockFile = modkernel32.NewProc("UnlockFileEx")
)

const (
	winLockfileExclusiveLock   = 0x00000002
	winLockfileFailImmediately = 0x00000001
)

// FlockExclusive acquires an exclusive (write) lock on the file.
// If nonBlocking is true, returns immediately with an error if the lock cannot be acquired.
func FlockExclusive(f *os.File, nonBlocking bool) error {
	flags := uintptr(winLockfileExclusiveLock)
	if nonBlocking {
		flags |= winLockfileFailImmediately
	}
	var overlapped syscall.Overlapped
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		flags,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

// FlockShared acquires a shared (read) lock on the file.
// If nonBlocking is true, returns immediately with an error if the lock cannot be acquired.
func FlockShared(f *os.File, nonBlocking bool) error {
	flags := uintptr(0) // No exclusive flag = shared lock
	if nonBlocking {
		flags |= winLockfileFailImmediately
	}
	var overlapped syscall.Overlapped
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		flags,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	return nil
}

// Funlock releases the lock on the file.
func Funlock(f *os.File) error {
	var overlapped syscall.Overlapped
	ret, _, err := procUnlockFile.Call(
		f.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("failed to unlock file: %w", err)
	}
	return nil
}
