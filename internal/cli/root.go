package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/e3742526/tribunal/internal/tribunal/app"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

var Version = "0.1.0"

type flags struct {
	JSON           bool
	StateRoot      string
	Panel          string
	Kind           string
	TrustWorkspace bool
	MaxOutputBytes int64
	RunTimeout     time.Duration
	TokenBudget    int
}

func NewRootCommand() *cobra.Command {
	f := &flags{}
	root := &cobra.Command{
		Use:           "tribunal",
		Short:         "Independent, evidence-aware deliberation over documents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			return &app.ExitError{Code: app.ExitInvalidArguments, Err: errors.New("a command is required")}
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&f.JSON, "json", false, "emit a stable machine-readable JSON result")
	root.PersistentFlags().StringVar(&f.StateRoot, "state-root", "", "external state root (default ~/.local/state/tribunal)")
	root.PersistentFlags().StringVar(&f.Panel, "panel", "", "reviewers as adapter/model[@persona], comma-separated")
	root.PersistentFlags().StringVar(&f.Kind, "kind", "", "document kind: generic, manuscript, strategy, or governance")
	root.PersistentFlags().BoolVar(&f.TrustWorkspace, "trust-workspace-config", false, "trust .tribunal.toml in the document workspace")
	root.PersistentFlags().Int64Var(&f.MaxOutputBytes, "max-output-bytes", 0, "maximum bytes accepted from one model call")
	root.PersistentFlags().DurationVar(&f.RunTimeout, "max-wall-time", 0, "maximum total review time")
	root.PersistentFlags().IntVar(&f.TokenBudget, "token-budget", 0, "maximum estimated tokens for the run")
	root.AddCommand(newReviewCommand(f, false), newReviewCommand(f, true))
	root.AddCommand(newArbitrateCommand(f), newEditCommand(f), newRevertCommand(f))
	root.AddCommand(newResumeCommand(f), newReplayCommand(f), newExplainCommand(f))
	root.AddCommand(newFindingsCommand(f), newDecisionsCommand(f))
	root.AddCommand(newStatusCommand(f), newTranscriptCommand(f), newTUICommand(f))
	root.AddCommand(newPersonaCommand(f), newBenchCommand(f), newDoctorCommand(f), newAdoptCommand(f))
	root.AddCommand(newVersionCommand(f), newVerifyInstallCommand(f))
	return root
}

func serviceFor(input string, f *flags) (*app.Service, error) {
	workspace := input
	if workspace == "" {
		workspace = "."
	}
	if info, err := os.Stat(workspace); err == nil && !info.IsDir() {
		workspace = filepath.Dir(workspace)
	}
	cfg, err := config.Load(config.LoadOptions{Workspace: workspace, TrustWorkspaceConfig: f.TrustWorkspace, ExplicitStateRoot: f.StateRoot, ExplicitPanel: f.Panel, ExplicitKind: f.Kind, ExplicitOutputBytes: f.MaxOutputBytes, ExplicitRunTimeout: f.RunTimeout, ExplicitTokenBudget: f.TokenBudget})
	if err != nil {
		return nil, &app.ExitError{Code: app.ExitInvalidArguments, Err: err}
	}
	store, err := storage.New(cfg.StateRoot)
	if err != nil {
		return nil, &app.ExitError{Code: app.ExitPreflight, Err: err}
	}
	return app.New(cfg, store, app.DefaultRegistry(cfg))
}

func commandContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
}

func printJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func printValue(cmd *cobra.Command, jsonMode bool, value any, human string) error {
	if jsonMode {
		return printJSON(cmd.OutOrStdout(), value)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), human)
	return err
}
