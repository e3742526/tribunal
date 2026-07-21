//go:build !darwin && !linux

package storage

import (
	"context"
	"fmt"
)

type Lock struct{}

func AcquireLock(context.Context, string, func()) (*Lock, error) {
	return nil, fmt.Errorf("Tribunal runtime locks are supported only on macOS and Linux")
}

func (*Lock) Close() error { return nil }
