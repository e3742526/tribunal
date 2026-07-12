package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
)

func newIntegrateCommand() *cobra.Command {
	var target, path string
	root := &cobra.Command{Use: "integrate", Short: "Plan and manage non-destructive Tagteam editor integration blocks", SilenceUsage: true}
	run := func(action string) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if !validCLITarget(target) {
				return fmt.Errorf("--target must be codex, claude, cursor, vscode, or mcp-json")
			}
			if path == "" {
				return fmt.Errorf("--path is required; Tagteam never guesses or rewrites agent configuration")
			}
			existing, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			var result tagteam.IntegrationResult
			switch action {
			case "plan", "install":
				result, err = tagteam.PlanIntegration(target, existing)
			case "doctor":
				result = tagteam.DoctorIntegration(target, existing)
			case "uninstall":
				result, err = tagteam.UninstallIntegration(target, existing)
			}
			if err != nil {
				return err
			}
			if action == "install" || action == "uninstall" {
				if result.Changed {
					if err := tagteam.WriteFileDurableForCLI(path, result.Content, 0o644); err != nil {
						return err
					}
				}
			}
			if action == "plan" {
				fmt.Fprintln(cmd.OutOrStdout(), unifiedDiff(existing, result.Content))
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
	}
	for _, action := range []string{"plan", "install", "doctor", "uninstall"} {
		root.AddCommand(&cobra.Command{Use: action, RunE: run(action)})
	}
	root.PersistentFlags().StringVar(&target, "target", "", "Integration target: codex, claude, cursor, vscode, or mcp-json")
	root.PersistentFlags().StringVar(&path, "path", "", "Explicit configuration file path")
	return root
}

func validCLITarget(target string) bool {
	for _, candidate := range []string{"codex", "claude", "cursor", "vscode", "mcp-json"} {
		if target == candidate {
			return true
		}
	}
	return false
}
func unifiedDiff(before, after []byte) string {
	if bytes.Equal(before, after) {
		return "(no changes)"
	}
	return "--- existing\n+++ planned\n-" + string(before) + "+" + string(after)
}
