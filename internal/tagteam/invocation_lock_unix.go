//go:build !windows

package tagteam

import (
	"errors"
	"os"
	"syscall"
)

// tryLockInvocationFile attempts one non-blocking flock acquisition. It
// returns the held lock, or the recorded holder PID when the lock is busy.
// The kernel releases the flock when the holder exits for any reason, so no
// stale-lock reclamation is needed and mutual exclusion cannot be broken by
// interleaved cleanup.
func tryLockInvocationFile(path string) (*invocationLock, int, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, 0, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, readInvocationHolderPID(path), nil
		}
		return nil, 0, err
	}
	return &invocationLock{file: file, path: path, pid: os.Getpid()}, 0, nil
}

func (l *invocationLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
