//go:build !darwin && !linux

package storage

import "os"

func openNoFollowReadWrite(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
