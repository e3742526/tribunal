package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
	"github.com/cephalopod-ai/tagteam/internal/tui"
)

func newTUICommand(shared *flagState) *cobra.Command {
	return &cobra.Command{
		Use:   "tui [RUN_ID]",
		Short: "Read-only live view of a run's status, plan, findings, and artifacts",
		Long: `tui polls the on-disk run artifacts (active.json/state.json/final.json/plan.json) and renders a read-only terminal view. It does not invoke agents and does not modify the run directory.

With no RUN_ID it prefers the currently active (running) run and falls back to the most recent completed run.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir, err := filepath.Abs(shared.Workdir)
			if err != nil {
				return err
			}
			runDir, err := resolveTUIRunDir(workdir, args)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			return tui.Run(ctx, workdir, runDir, os.Stdout, os.Stdin)
		},
	}
}

// resolveTUIRunDir picks the run directory tui should display: an explicit
// RUN_ID, otherwise the active (running) run if one exists, otherwise the
// most recent completed run.
func resolveTUIRunDir(workdir string, args []string) (string, error) {
	if len(args) > 0 {
		runID := args[0]
		runDir := filepath.Join(workdir, ".tagteam", "runs", runID)
		info, err := os.Stat(runDir)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("run %q not found under %s", runID, filepath.Join(workdir, ".tagteam", "runs"))
		}
		return runDir, nil
	}
	if active, err := tagteam.ReadActiveRunForCLI(workdir); err == nil && active.Status == "running" && active.RunDir != "" {
		return active.RunDir, nil
	}
	latest, err := tagteam.ReadLatestForCLI(workdir)
	if err != nil {
		return "", fmt.Errorf("no active or previous tagteam run found in %s", workdir)
	}
	return latest.RunDir, nil
}
