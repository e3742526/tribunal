package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/e3742526/tribunal/internal/tribunal/app"
	tribunaltui "github.com/e3742526/tribunal/internal/tui"
)

func newStatusCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "status [file-or-folder]",
		Short: "Show a read-only snapshot of a Tribunal run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			snapshot, err := service.Status(app.RunRef{Input: input, RunID: runID})
			if err != nil {
				return err
			}
			return printValue(cmd, f.JSON, snapshot, tribunaltui.RenderSnapshot(snapshot))
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	return cmd
}

func newTranscriptCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "transcript [file-or-folder]",
		Short: "Show lifecycle events and exact delivery records",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			transcript, err := service.Transcript(app.RunRef{Input: input, RunID: runID})
			if err != nil {
				return err
			}
			var lines []string
			for _, event := range transcript.Events {
				lines = append(lines, fmt.Sprintf("%s  %s -> %s  %s", event.At.Format("2006-01-02T15:04:05Z"), event.From, event.To, event.Status))
			}
			return printValue(cmd, f.JSON, transcript, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	return cmd
}

func newTUICommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "tui [file-or-folder]",
		Short: "Render the read-only status view from a RunSnapshot",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			snapshot, err := service.Status(app.RunRef{Input: input, RunID: runID})
			if err != nil {
				return err
			}
			return printValue(cmd, f.JSON, snapshot, tribunaltui.RenderSnapshot(snapshot))
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	return cmd
}

func newResumeCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "resume [file-or-folder]",
		Short: "Resume an incomplete run from its durably frozen packet",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			ctx, stop := commandContext(cmd)
			defer stop()
			final, resumeErr := service.Resume(ctx, app.RunRef{Input: input, RunID: runID})
			if err := renderFinal(cmd, f.JSON, final); err != nil {
				return err
			}
			return resumeErr
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	return cmd
}

func newReplayCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "replay [file-or-folder]",
		Short: "Run the recorded panel over the exact frozen packet as a new run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			ctx, stop := commandContext(cmd)
			defer stop()
			final, replayErr := service.Replay(ctx, app.RunRef{Input: input, RunID: runID})
			if err := renderFinal(cmd, f.JSON, final); err != nil {
				return err
			}
			return replayErr
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "source run ID (default latest)")
	return cmd
}

func newExplainCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "explain <file-or-folder> <finding-id>",
		Short: "Explain one finding and its recorded consensus decision",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := serviceFor(args[0], f)
			if err != nil {
				return err
			}
			explanation, err := service.Explain(app.RunRef{Input: args[0], RunID: runID}, args[1])
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s [%s/%s]\n%s\nRecommendation: %s", explanation.Finding.ID, explanation.Finding.Severity, explanation.Finding.Category, explanation.Finding.Issue, explanation.Finding.Recommendation)
			return printValue(cmd, f.JSON, explanation, human)
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	return cmd
}

func newFindingsCommand(f *flags) *cobra.Command {
	root := &cobra.Command{Use: "findings", Short: "Inspect and explicitly defer workspace findings"}
	list := &cobra.Command{
		Use:   "list [file-or-folder]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List the external findings ledger",
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			ledger, err := service.Findings(app.RunRef{Input: input})
			if err != nil {
				return err
			}
			var lines []string
			for _, record := range ledger.Findings {
				lines = append(lines, fmt.Sprintf("%s  %-16s  %-7s  %s", record.ID, record.Status, record.Finding.Severity, record.Finding.Issue))
			}
			return printValue(cmd, f.JSON, ledger, strings.Join(lines, "\n"))
		},
	}
	var reason, operator string
	deferCmd := &cobra.Command{
		Use:   "defer <file-or-folder> <finding-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Defer a finding with an auditable reason and operator",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := serviceFor(args[0], f)
			if err != nil {
				return err
			}
			if err := service.DeferFinding(app.RunRef{Input: args[0]}, args[1], reason, operator); err != nil {
				return err
			}
			return printValue(cmd, f.JSON, map[string]any{"schema_version": 1, "finding_id": args[1], "status": "deferred"}, "deferred "+args[1])
		},
	}
	deferCmd.Flags().StringVar(&reason, "reason", "", "required defer reason")
	deferCmd.Flags().StringVar(&operator, "operator", "", "required operator identity")
	root.AddCommand(list, deferCmd)
	return root
}

func newDecisionsCommand(f *flags) *cobra.Command {
	root := &cobra.Command{Use: "decisions", Short: "Export recorded arbitration memory"}
	export := &cobra.Command{
		Use:   "export [file-or-folder]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Export decision-memory records",
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			records, err := service.Decisions(app.RunRef{Input: input})
			if err != nil {
				return err
			}
			return printValue(cmd, f.JSON, map[string]any{"schema_version": 1, "decisions": records}, fmt.Sprintf("%d recorded decisions", len(records)))
		},
	}
	root.AddCommand(export)
	return root
}

func firstArg(args []string, fallback string) string {
	if len(args) > 0 {
		return args[0]
	}
	return fallback
}
