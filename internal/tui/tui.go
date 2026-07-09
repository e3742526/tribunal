package tui

import (
	"context"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
)

const pollInterval = time.Second

// Run drives the read-only polling view for the run at runDir until the run
// stops being "running" or the user quits. It never invokes an adapter or
// mutates the run directory -- each tick only re-reads the on-disk snapshot.
func Run(ctx context.Context, workdir, runDir string, out *os.File, in *os.File) error {
	toggles := DefaultToggles()
	isTTY := in != nil && term.IsTerminal(int(in.Fd()))

	var restore func()
	if isTTY {
		oldState, err := term.MakeRaw(int(in.Fd()))
		if err == nil {
			restore = func() { _ = term.Restore(int(in.Fd()), oldState) }
			defer restore()
		}
	}

	for {
		snapshot, err := tagteam.BuildRunSnapshot(workdir, runDir)
		if err != nil {
			return err
		}
		plan, hasPlan := readPlan(runDir)
		var planPtr *tagteam.ExecutionPlan
		if hasPlan {
			planPtr = &plan
		}

		clearScreen(out)
		Render(out, snapshot, planPtr, toggles)

		if snapshot.Status != "running" {
			return nil
		}

		key, timedOut, err := waitForKey(ctx, in, isTTY)
		if err != nil {
			return nil
		}
		if timedOut {
			continue
		}
		switch key {
		case 'q', 'Q', 3: // 3 = Ctrl-C, since raw mode disables signal generation
			return nil
		case 'r', 'R':
			// fall through and re-render immediately
		case 'p', 'P':
			toggles.ShowPlan = !toggles.ShowPlan
		case 'f', 'F':
			toggles.ShowFindings = !toggles.ShowFindings
		case 'd', 'D':
			toggles.ShowArtifacts = !toggles.ShowArtifacts
		}
	}
}

func readPlan(runDir string) (tagteam.ExecutionPlan, bool) {
	plan, err := tagteam.ReadPlanForCLI(runDir)
	if err != nil {
		return tagteam.ExecutionPlan{}, false
	}
	return plan, true
}

func clearScreen(out *os.File) {
	_, _ = out.WriteString("\x1b[H\x1b[2J")
}

// waitForKey waits up to one poll interval for a single keypress. On a
// non-TTY stdin (CI, piped input) there is no interactive key to read, so it
// just sleeps out the tick. Each tick that times out on a TTY leaves its read
// goroutine blocked on the next keypress rather than canceled -- os.File
// reads cannot be interrupted from another goroutine, so the leaked reads are
// left to resolve (and be discarded) whenever the user does press a key.
func waitForKey(ctx context.Context, in *os.File, isTTY bool) (key byte, timedOut bool, err error) {
	if !isTTY {
		select {
		case <-ctx.Done():
			return 0, false, ctx.Err()
		case <-time.After(pollInterval):
			return 0, true, nil
		}
	}

	keyCh := make(chan byte, 1)
	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		n, readErr := in.Read(buf)
		if readErr != nil {
			errCh <- readErr
			return
		}
		if n > 0 {
			keyCh <- buf[0]
		}
	}()

	select {
	case <-ctx.Done():
		return 0, false, ctx.Err()
	case readErr := <-errCh:
		return 0, false, readErr
	case k := <-keyCh:
		return k, false, nil
	case <-time.After(pollInterval):
		return 0, true, nil
	}
}
