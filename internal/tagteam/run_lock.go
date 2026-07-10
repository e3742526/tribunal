package tagteam

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type runLockRecord struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

type runLock struct {
	path string
	pid  int
}

func acquireRunLock(runDir string, allowStale bool) (*runLock, error) {
	path := filepath.Join(runDir, "run.lock")
	if data, err := os.ReadFile(path); err == nil {
		var existing runLockRecord
		if json.Unmarshal(data, &existing) == nil && existing.PID > 0 && processAlive(existing.PID) {
			return nil, fmt.Errorf("run is locked by pid %d", existing.PID)
		}
		if !allowStale {
			return nil, fmt.Errorf("stale run lock exists at %s", path)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	record := runLockRecord{PID: os.Getpid(), CreatedAt: time.Now().UTC()}
	data, err := marshalJSON(record, true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &runLock{path: path, pid: os.Getpid()}, nil
}

func (l *runLock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var existing runLockRecord
	if json.Unmarshal(data, &existing) != nil || existing.PID != l.pid {
		return nil
	}
	return os.Remove(l.path)
}
