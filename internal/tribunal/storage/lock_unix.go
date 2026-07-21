//go:build darwin || linux

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Lock struct{ file *os.File }

func AcquireLock(ctx context.Context, path string, onWait func()) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open lock file")
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

func LockStatus(path string) (bool, int, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		return false, 0, nil
	} else if err != unix.EWOULDBLOCK {
		return false, 0, err
	}
	data := make([]byte, 32)
	n, _ := file.ReadAt(data, 0)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data[:n])))
	return true, pid, nil
}
