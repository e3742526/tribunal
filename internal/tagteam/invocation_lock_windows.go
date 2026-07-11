//go:build windows

package tagteam

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// invocationLockFreshGrace protects a just-created lock file whose holder
// record is not yet readable from being judged stale by another contender.
const invocationLockFreshGrace = 30 * time.Second

// Windows has no flock in the standard library, so the lock is an
// exclusively created PID file. Stale records from dead processes are
// reclaimed through an atomic rename to a unique quarantine name: only one
// contender's rename can succeed, and the quarantined content is re-verified
// before deletion so a freshly created live lock is restored, not destroyed.
func tryLockInvocationFile(path string) (*invocationLock, int, error) {
	if busy, holder, err := invocationLockBusy(path); err != nil {
		return nil, 0, err
	} else if busy {
		return nil, holder, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, readInvocationHolderPID(path), nil
		}
		return nil, 0, err
	}
	lock := &invocationLock{file: file, path: path, pid: os.Getpid()}
	lock.recordHolder()
	return lock, 0, nil
}

// invocationLockBusy reports whether a live holder owns the lock file and
// reclaims records left behind by dead processes.
func invocationLockBusy(path string) (bool, int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	var existing runLockRecord
	parsed := json.Unmarshal(data, &existing) == nil && existing.PID > 0
	if parsed && existing.PID != os.Getpid() && processAlive(existing.PID) {
		return true, existing.PID, nil
	}
	if !parsed {
		// An unreadable record may be a lock created microseconds ago whose
		// holder has not finished writing; only reclaim it once it is old.
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) < invocationLockFreshGrace {
			return true, 0, nil
		}
	}
	quarantine := fmt.Sprintf("%s.stale-%d-%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.Rename(path, quarantine); err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if data, err := os.ReadFile(quarantine); err == nil {
		var moved runLockRecord
		if json.Unmarshal(data, &moved) == nil && moved.PID > 0 && moved.PID != os.Getpid() && processAlive(moved.PID) {
			_ = os.Rename(quarantine, path)
			return true, moved.PID, nil
		}
	}
	_ = os.Remove(quarantine)
	return false, 0, nil
}

func (l *invocationLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	closeErr := l.file.Close()
	l.file = nil
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return closeErr
	}
	if err != nil {
		return err
	}
	var existing runLockRecord
	if json.Unmarshal(data, &existing) != nil || existing.PID != l.pid {
		return closeErr
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return closeErr
}
