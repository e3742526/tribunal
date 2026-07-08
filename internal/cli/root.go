package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"tagteam/internal/tagteam"
)

type flagState struct {
	tagteam.FlagInputs
}

func NewRootCommand() *cobra.Command {
	flags := &flagState{}
	root := &cobra.Command{
		Use:           "tagteam [flags] <prompt>",
		Short:         "Run a coder and adversary loop over a repository",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				_ = cmd.Help()
				return &tagteam.ExitError{Code: tagteam.ExitInvalidArguments}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDefault(cmd, flags, strings.Join(args, " "))
		},
	}

	bindSharedFlags(root, flags)
	root.AddCommand(newReviewCommand(flags))
	root.AddCommand(newFixCommand(flags))
	root.AddCommand(newStatusCommand(flags))
	root.AddCommand(newTranscriptCommand(flags))
	root.AddCommand(newInitCommand(flags))
	root.AddCommand(newDoctorCommand(flags))
	return root
}

func bindSharedFlags(cmd *cobra.Command, flags *flagState) {
	flagSet := cmd.PersistentFlags()
	flagSet.StringVar(&flags.Coder, "mc", "", "Coder adapter[:model]")
	flagSet.StringVar(&flags.Adversary, "ma", "", "Adversary adapter[:model]")
	flagSet.StringVarP(&flags.Profile, "profile", "P", "", "Named profile")
	flagSet.StringVarP(&flags.Workdir, "workdir", "C", ".", "Working directory")
	flagSet.IntVarP(&flags.Rounds, "rounds", "r", 0, "Max rounds")
	flagSet.StringVarP(&flags.Test, "test", "t", "", "Test command")
	flagSet.BoolVar(&flags.NoTest, "no-test", false, "Skip tests")
	flagSet.BoolVar(&flags.JSON, "json", false, "Print machine-readable final result")
	flagSet.BoolVar(&flags.DryRun, "dry-run", false, "Print vendor invocations without executing")
	flagSet.BoolVar(&flags.ShowReview, "show-review", false, "Include review findings in output")
	flagSet.BoolVar(&flags.FailOnReview, "fail-on-review", false, "Exit non-zero on blocking review findings")
	flagSet.BoolVar(&flags.AllowDirty, "allow-dirty", false, "Skip clean-worktree preflight")
	flagSet.BoolVar(&flags.Autostash, "autostash", false, "Stash local changes before run")
	flagSet.DurationVar(&flags.Timeout, "timeout", 15*time.Minute, "Per-invocation timeout")
	flagSet.BoolVarP(&flags.Quiet, "quiet", "q", false, "Reduce progress output")
	flagSet.BoolVarP(&flags.Verbose, "verbose", "v", false, "Increase progress output")
	flagSet.StringVar(&flags.CodexArgsRaw, "codex-args", "", "Raw args appended to codex invocations")
	flagSet.StringVar(&flags.ClaudeArgsRaw, "claude-args", "", "Raw args appended to claude invocations")
	flagSet.StringVar(&flags.AgyArgsRaw, "agy-args", "", "Raw args appended to agy invocations")
}

func runDefault(cmd *cobra.Command, flags *flagState, prompt string) error {
	opts, cfg, err := resolve(cmd, flags, prompt)
	if err != nil {
		return err
	}
	app := tagteam.NewApp(cfg)
	final, err := app.Run(context.Background(), opts)
	final = withErrorExitCode(final, err)
	renderFinal(cmd, final, opts)
	return err
}

func withErrorExitCode(final tagteam.FinalRun, err error) tagteam.FinalRun {
	if err != nil && final.RunID != "" && final.ExitCode == tagteam.ExitSuccess {
		final.ExitCode = tagteam.ExitCode(err)
	}
	return final
}

func newReviewCommand(shared *flagState) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "review",
		Short:        "Adversary-only review over the current diff",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, cfg, err := resolve(cmd, shared, "")
			if err != nil {
				return err
			}
			app := tagteam.NewApp(cfg)
			final, err := app.Review(context.Background(), opts, "")
			final = withErrorExitCode(final, err)
			renderFinal(cmd, final, opts)
			return err
		},
	}
	return cmd
}

func newFixCommand(shared *flagState) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "fix",
		Short:        "Coder applies fixes from the latest saved review",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, cfg, err := resolve(cmd, shared, "")
			if err != nil {
				return err
			}
			app := tagteam.NewApp(cfg)
			final, err := app.Fix(context.Background(), opts)
			final = withErrorExitCode(final, err)
			renderFinal(cmd, final, opts)
			return err
		},
	}
	return cmd
}

func newStatusCommand(shared *flagState) *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Short:        "Show the latest run summary",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir, _ := filepath.Abs(shared.Workdir)
			latest, err := tagteam.ReadLatestForCLI(workdir)
			if err != nil {
				return err
			}
			final, err := tagteam.ReadFinalForCLI(latest.FinalPath)
			if err != nil {
				return err
			}
			renderFinal(cmd, final, tagteam.RunOptions{JSON: shared.JSON, ShowReview: shared.ShowReview})
			return nil
		},
	}
}

func newTranscriptCommand(shared *flagState) *cobra.Command {
	return &cobra.Command{
		Use:          "transcript [RUN_ID]",
		Short:        "Print the path to a saved transcript",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir, _ := filepath.Abs(shared.Workdir)
			runDir := ""
			if len(args) == 0 {
				latest, err := tagteam.ReadLatestForCLI(workdir)
				if err != nil {
					return err
				}
				runDir = latest.RunDir
			} else {
				runDir = filepath.Join(workdir, ".tagteam", "runs", args[0])
			}
			fmt.Fprintln(cmd.OutOrStdout(), runDir)
			return nil
		},
	}
}

func newDoctorCommand(shared *flagState) *cobra.Command {
	return &cobra.Command{
		Use:          "doctor",
		Short:        "Check adapter binaries and auth hints",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, cfg, err := resolve(cmd, shared, "")
			if err != nil {
				return err
			}
			app := tagteam.NewApp(cfg)
			status, err := app.Doctor(context.Background(), opts)
			for _, key := range []string{"codex", "codex-oss", "claude", "agy"} {
				item := status[key]
				fmt.Fprintf(cmd.OutOrStdout(), "%s\tfound=%t\tversion=%s\tauth=%s\thint=%s\n", key, item.Found, item.Version, item.Auth, item.Hint)
			}
			return err
		},
	}
}

func newInitCommand(shared *flagState) *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Write a starter user config",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir, _ := filepath.Abs(shared.Workdir)
			cfg, _, err := tagteam.LoadConfig(workdir)
			if err != nil {
				return err
			}
			path, err := tagteam.UserConfigPathForCLI()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			data, err := tagteam.EncodeConfig(cfg)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return err
			}
			if err := tagteam.EnsureGitignoreEntryForCLI(workdir, ".tagteam/"); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func resolve(cmd *cobra.Command, flags *flagState, prompt string) (tagteam.RunOptions, tagteam.Config, error) {
	workdir, err := filepath.Abs(flags.Workdir)
	if err != nil {
		return tagteam.RunOptions{}, tagteam.Config{}, err
	}
	cfg, sources, err := tagteam.LoadConfig(workdir)
	if err != nil {
		return tagteam.RunOptions{}, tagteam.Config{}, err
	}
	changed := map[string]bool{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			changed[flag.Name] = true
		}
	})
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			changed[flag.Name] = true
		}
	})
	opts, err := tagteam.ResolveOptions(cfg, sources, flags.FlagInputs, changed, prompt)
	if err != nil {
		return tagteam.RunOptions{}, tagteam.Config{}, err
	}
	return opts, cfg, nil
}

func renderFinal(cmd *cobra.Command, final tagteam.FinalRun, opts tagteam.RunOptions) {
	if final.RunID == "" {
		return
	}
	if opts.JSON {
		payload, _ := json.MarshalIndent(final, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "run=%s verdict=%s exit=%d rounds=%d/%d\n", final.RunID, final.Verdict, final.ExitCode, final.RoundsCompleted, final.RoundsRequested)
	if final.Summary != "" {
		fmt.Fprintln(cmd.OutOrStdout(), final.Summary)
	}
	if final.RunDir != "" {
		fmt.Fprintln(cmd.OutOrStdout(), final.RunDir)
	}
	if opts.ShowReview && final.Review != nil {
		for _, finding := range final.Review.Findings {
			fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s:%d %s\n", finding.Severity, finding.File, finding.Line, finding.Issue)
		}
	}
}
