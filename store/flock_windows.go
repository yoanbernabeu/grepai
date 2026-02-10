//go:build windows
// +build windows

package store

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
	winLockfileExclusiveLock = 0x00000002
)

// flockExclusive acquires an exclusive (write) lock on the file.
func flockExclusive(f *os.File) error {
	var overlapped syscall.Overlapped
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(winLockfileExclusiveLock),
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

// flockShared acquires a shared (read) lock on the file.
func flockShared(f *os.File) error {
	var overlapped syscall.Overlapped
	// No exclusive flag = shared lock
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		0,
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

// funlock releases the lock on the file.
func funlock(f *os.File) error {
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
