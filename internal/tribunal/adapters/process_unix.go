//go:build darwin || linux

package adapters

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Cancel runs inside exec's own state machine, which sequences the kill
	// strictly before Wait reaps the child — a hand-rolled select raced
	// cmd.Wait against a raw Kill(-pid) and could, on pid reuse, SIGKILL an
	// unrelated process group.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// WaitDelay force-closes the stdio pipes if the process group leaves a
	// helper behind (e.g. a setsid escapee holding the write end), so Wait
	// cannot block past cancellation indefinitely.
	cmd.WaitDelay = 5 * time.Second
}

func runProcess(ctx context.Context, cmd *exec.Cmd) error {
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// ErrWaitDelay means the child itself exited successfully but an orphan
	// held the stdio pipes past WaitDelay. The call succeeded; stdout may be
	// truncated, which downstream contract validation handles, and file
	// outputs (-o) are consulted separately.
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}
