//go:build !darwin && !linux

package adapters

import (
	"context"
	"fmt"
	"os/exec"
)

func configureProcess(*exec.Cmd) {}
func runProcess(context.Context, *exec.Cmd) error {
	return fmt.Errorf("Tribunal subprocess adapters are supported only on macOS and Linux")
}
