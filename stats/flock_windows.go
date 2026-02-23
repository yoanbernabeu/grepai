//go:build windows

package stats

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

const winLockfileExclusiveLock = 0x00000002

func flockExclusive(f *os.File) error {
	var overlapped syscall.Overlapped
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(winLockfileExclusiveLock),
		0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

func funlock(f *os.File) error {
	var overlapped syscall.Overlapped
	ret, _, err := procUnlockFile.Call(
		f.Fd(),
		0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("failed to unlock file: %w", err)
	}
	return nil
}
