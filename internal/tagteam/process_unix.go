//go:build !windows

package tagteam

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func prepareProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		pid := cmd.Process.Pid
		go func() {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			<-timer.C
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}()
		return nil
	}
	cmd.WaitDelay = 6 * time.Second
}
