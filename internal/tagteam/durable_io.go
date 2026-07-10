package tagteam

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func writeFileDurable(path string, data []byte, mode os.FileMode, preservePrevious bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if preservePrevious {
		if err := preserveCurrentArtifact(path, mode); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".tagteam-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func preserveCurrentArtifact(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular artifact %s", path)
	}
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	previous := path + ".previous"
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tagteam-previous-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, source); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, previous)
}

func marshalJSON(value any, newline bool) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	if newline {
		data = append(data, '\n')
	}
	return data, nil
}

func writeJSONDurable(path string, value any, newline, preservePrevious bool) error {
	data, err := marshalJSON(value, newline)
	if err != nil {
		return err
	}
	return writeFileDurable(path, data, 0o644, preservePrevious)
}
