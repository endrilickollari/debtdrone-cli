//go:build darwin || linux
// +build darwin linux

package git

import (
	"fmt"

	"golang.org/x/sys/unix"
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
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return fmt.Errorf("statfs failed: %w", err)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	total := stat.Blocks * uint64(stat.Bsize)

	if available < MinDiskSpaceBytes {
		return &ErrInsufficientStorage{
			Available: available,
			Total:     total,
			Reason:    fmt.Sprintf("less than 2GB available (%d MB)", available/(1024*1024)),
		}
	}

	if total > 0 && float64(available)/float64(total) < MinDiskSpacePercent {
		return &ErrInsufficientStorage{
			Available: available,
			Total:     total,
			Reason:    fmt.Sprintf("less than 10%% available (%.1f%%)", float64(available)/float64(total)*100),
		}
	}

	return nil
}

func IsInsufficientStorageError(err error) bool {
	_, ok := err.(*ErrInsufficientStorage)
	return ok
}
