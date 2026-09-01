//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package localhistory

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type historyLock struct {
	file *os.File
}

func acquireHistoryLock(ctx context.Context, path string) (*historyLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}

	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &historyLock{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (l *historyLock) release() {
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	_ = l.file.Close()
}

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
