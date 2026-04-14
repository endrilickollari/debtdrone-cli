//go:build windows
// +build windows

package git

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	MinDiskSpaceBytes   = 2 * 1024 * 1024 * 1024 // 2GB
	MinDiskSpacePercent = 0.10                   // 10%
)

type ErrInsufficientStorage struct {
	Available uint64
	Total     uint64
	Reason    string
}

func (e *ErrInsufficientStorage) Error() string {
	return fmt.Sprintf("INSUFFICIENT_STORAGE: %s (available: %d MB, total: %d MB)",
		e.Reason, e.Available/(1024*1024), e.Total/(1024*1024))
}

func CheckDiskSpace(path string) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	pathPtr, _ := syscall.UTF16PtrFromString(path)
	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		return fmt.Errorf("GetDiskFreeSpaceExW failed: %w", err)
	}

	if freeBytesAvailable < MinDiskSpaceBytes {
		return &ErrInsufficientStorage{
			Available: freeBytesAvailable,
			Total:     totalBytes,
			Reason:    fmt.Sprintf("less than 2GB available (%d MB)", freeBytesAvailable/(1024*1024)),
		}
	}

	if totalBytes > 0 && float64(freeBytesAvailable)/float64(totalBytes) < MinDiskSpacePercent {
		return &ErrInsufficientStorage{
			Available: freeBytesAvailable,
			Total:     totalBytes,
			Reason:    fmt.Sprintf("less than 10%% available (%.1f%%)", float64(freeBytesAvailable)/float64(totalBytes)*100),
		}
	}

	return nil
}

func IsInsufficientStorageError(err error) bool {
	_, ok := err.(*ErrInsufficientStorage)
	return ok
}
