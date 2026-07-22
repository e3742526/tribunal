package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

func newEditCommand(f *flags) *cobra.Command {
	var runID, proposal string
	var apply, confirmDocument, rereview bool
	cmd := &cobra.Command{
		Use:   "edit [file-or-folder]",
		Short: "Propose or atomically apply accepted, scope-bounded replacement hunks",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			ctx, stop := commandContext(cmd)
			defer stop()
			result, editErr := service.Edit(ctx, app.EditOptions{RunRef: app.RunRef{Input: input, RunID: runID}, ProposalPath: proposal, Apply: apply, ConfirmDocument: confirmDocument, Rereview: rereview})
			if editErr != nil && result.SchemaVersion == 0 {
				return renderError(cmd, f, editErr)
			}
			if err := printValue(cmd, f, result, fmt.Sprintf("run=%s hunks=%d applied=%t", result.RunID, len(result.Proposal.Hunks), result.Applied)); err != nil {
				return err
			}
			return editErr
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "run ID (default latest)")
	cmd.Flags().StringVar(&proposal, "proposal", "", "validate a proposal JSON file instead of invoking an editor")
	cmd.Flags().BoolVar(&apply, "apply", false, "apply the validated proposal (default is dry-run proposal only)")
	cmd.Flags().BoolVar(&confirmDocument, "confirm-document-scope", false, "explicitly permit document-scope hunks")
	cmd.Flags().BoolVar(&rereview, "rereview", false, "run a bounded review over the edited document")
	return cmd
}

func newRevertCommand(f *flags) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "revert [file-or-folder]",
		Short: "Restore preserved originals only when edited hashes still match",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := firstArg(args, ".")
			service, err := serviceFor(input, f)
			if err != nil {
				return err
			}
			record, err := service.Revert(app.RunRef{Input: input, RunID: runID})
			if err != nil {
				return err
			}
			return printValue(cmd, f, record, fmt.Sprintf("reverted run %s (%d files)", record.RunID, len(record.Files)))
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "edited run ID (default latest)")
	return cmd
}
