package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/e3742526/tribunal/internal/tribunal/app"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func newReviewCommand(f *flags, recommend bool) *cobra.Command {
	name, short := "review", "Run an independent panel review over a document or folder"
	if recommend {
		name, short = "recommend", "Produce recommendation-oriented findings using a selected rubric"
	}
	var split, failOnSecret, noWorkers bool
	var rubric string
	cmd := &cobra.Command{
		Use:   name + " <file-or-folder>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := serviceFor(args[0], f)
			if err != nil {
				return err
			}
			kind := f.Kind
			if recommend && rubric != "" {
				kind = rubric
			}
			ctx, stop := commandContext(cmd)
			defer stop()
			final, reviewErr := service.Review(ctx, app.ReviewOptions{Input: args[0], Kind: kind, Panel: f.Panel, Split: split, FailOnSecret: failOnSecret, NoWorkers: noWorkers})
			if err := renderFinal(cmd, f.JSON, final); err != nil {
				return err
			}
			return reviewErr
		},
	}
	cmd.Flags().BoolVar(&split, "split", false, "split the frozen packet to the smallest panel context budget")
	cmd.Flags().BoolVar(&failOnSecret, "fail-on-secret", false, "fail instead of length-preserving secret/PII redaction")
	cmd.Flags().BoolVar(&noWorkers, "no-workers", false, "disable deterministic spelling and reference workers")
	if recommend {
		cmd.Flags().StringVar(&rubric, "rubric", "generic", "built-in rubric: generic, manuscript, strategy, or governance")
	}
	return cmd
}

func newArbitrateCommand(f *flags) *cobra.Command {
	var runID, decisions, operator string
	var acceptMajority bool
	var except []string
	cmd := &cobra.Command{
		Use:   "arbitrate [file-or-folder]",
		Short: "Resolve pending disagreements with recorded operator decisions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			opts := app.ArbitrationOptions{RunRef: app.RunRef{Input: input, RunID: runID}, DecisionsPath: decisions, AcceptMajority: acceptMajority, Except: except, Operator: operator}
			if decisions == "" && !acceptMajority {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return &app.ExitError{Code: app.ExitInvalidArguments, Err: fmt.Errorf("non-interactive arbitration requires --decisions or --accept-majority")}
				}
				snapshot, statusErr := service.Status(opts.RunRef)
				if statusErr != nil || snapshot.Final == nil {
					return statusErr
				}
				rulings, promptErr := promptRulings(cmd, snapshot.Final.Arbitration, operator)
				if promptErr != nil {
					return promptErr
				}
				opts.Rulings = rulings
			}
			final, arbitrationErr := service.Arbitrate(opts)
			if err := renderFinal(cmd, f.JSON, final); err != nil {
				return err
			}
			return arbitrationErr
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	cmd.Flags().StringVar(&decisions, "decisions", "", "schema-versioned arbitration decisions JSON file")
	cmd.Flags().BoolVar(&acceptMajority, "accept-majority", false, "apply each dispute's recorded majority recommendation")
	cmd.Flags().StringSliceVar(&except, "except", nil, "dispute IDs to leave pending with --accept-majority")
	cmd.Flags().StringVar(&operator, "operator", "", "operator identity recorded with decisions")
	return cmd
}

func promptRulings(cmd *cobra.Command, disputes []domain.ArbitrationDispute, operator string) ([]app.ArbitrationRuling, error) {
	if operator == "" {
		return nil, &app.ExitError{Code: app.ExitInvalidArguments, Err: fmt.Errorf("interactive arbitration requires --operator")}
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	var rulings []app.ArbitrationRuling
	for _, dispute := range disputes {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\nDefault: %s\nChoose accept/reject/defer/skip: ", dispute.ID, dispute.Finding.Issue, dispute.Default)
		choice, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		choice = strings.TrimSpace(strings.ToLower(choice))
		if choice == "skip" || choice == "s" {
			continue
		}
		outcomes := map[string]string{"a": "accepted", "accept": "accepted", "r": "rejected", "reject": "rejected", "d": "deferred", "defer": "deferred"}
		outcome, ok := outcomes[choice]
		if !ok {
			return nil, &app.ExitError{Code: app.ExitInvalidArguments, Err: fmt.Errorf("invalid arbitration choice %q", choice)}
		}
		fmt.Fprint(cmd.OutOrStdout(), "Reason: ")
		reason, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		rulings = append(rulings, app.ArbitrationRuling{ID: dispute.ID, Outcome: outcome, Reason: strings.TrimSpace(reason), Operator: operator})
	}
	return rulings, nil
}

func renderFinal(cmd *cobra.Command, jsonMode bool, final domain.Final) error {
	if jsonMode {
		return printJSON(cmd.OutOrStdout(), final)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "run=%s status=%s exit=%d\n%s\n", final.RunID, final.Status, final.ExitCode, final.Summary)
	return err
}
