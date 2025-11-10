//go:build windows

package cacheify

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procMoveFileExW = kernel32.NewProc("MoveFileExW")
)

const (
	MOVEFILE_REPLACE_EXISTING = 0x1
)

// atomicRename performs an atomic rename operation on Windows
// Windows requires special handling because:
// 1. Files must be closed before renaming
// 2. We need MOVEFILE_REPLACE_EXISTING to overwrite destination
// 3. We use MoveFileEx for atomic replacement
func atomicRename(oldpath, newpath string) error {
	// On Windows, we need to use MoveFileEx with MOVEFILE_REPLACE_EXISTING
	// to atomically replace the destination file if it exists
	oldpathPtr, err := syscall.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	newpathPtr, err := syscall.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}

	// MOVEFILE_REPLACE_EXISTING = 0x1
	// This makes the operation atomic and allows overwriting
	r1, _, err := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(oldpathPtr)),
		uintptr(unsafe.Pointer(newpathPtr)),
		uintptr(MOVEFILE_REPLACE_EXISTING),
	)

	if r1 == 0 {
		return &os.PathError{Op: "rename", Path: oldpath, Err: err}
	}

	return nil
}
