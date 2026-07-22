//go:build darwin || linux

package storage

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openNoFollowReadWrite opens (creating if absent) a journal file without
// following symlinks, so a pre-planted link cannot redirect appends outside
// the state root.
func openNoFollowReadWrite(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open %s", path)
	}
	return file, nil
}
