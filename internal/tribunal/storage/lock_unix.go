//go:build darwin || linux

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type Lock struct{ file *os.File }

func AcquireLock(ctx context.Context, path string, onWait func()) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			if err := file.Truncate(0); err != nil {
				file.Close()
				return nil, err
			}
			if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
				file.Close()
				return nil, err
			}
			if err := file.Sync(); err != nil {
				file.Close()
				return nil, err
			}
			return &Lock{file: file}, nil
		} else if err != unix.EWOULDBLOCK {
			file.Close()
			return nil, err
		}
		if onWait != nil {
			onWait()
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, fmt.Errorf("acquire lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return firstError(err, l.file.Close())
}

func firstError(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
