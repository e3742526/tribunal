package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/e3742526/tribunal/internal/tribunal/app"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func newPanelCommand(f *flags) *cobra.Command {
	root := &cobra.Command{Use: "panel", Short: "Inspect panel policies and the panel a review would use"}
	root.AddCommand(newPanelListCommand(f), newPanelShowCommand(f))
	return root
}

func newPanelListCommand(f *flags) *cobra.Command {
	return &cobra.Command{Use: "list [workspace]", Args: cobra.MaximumNArgs(1), Short: "List built-in and trusted-config panel policies", RunE: func(cmd *cobra.Command, args []string) error {
		service, err := serviceFor(firstArg(args, "."), f)
		if err != nil {
			return err
		}
		// A trusted-config policy shadows a built-in of the same name, so the
		// listing reports the resolved set rather than both entries.
		policies := map[string]domain.PanelPolicy{}
		origin := map[string]string{}
		for _, policy := range config.StarterPolicies() {
			policies[policy.Name], origin[policy.Name] = policy, "builtin"
		}
		for _, policy := range service.Config.Policies {
			policies[policy.Name], origin[policy.Name] = policy, "config"
		}
		entries := make([]panelPolicyEntry, 0, len(policies))
		for _, policy := range config.StarterPolicies() {
			if origin[policy.Name] == "builtin" {
				entries = append(entries, panelPolicyEntry{Origin: "builtin", Policy: policies[policy.Name]})
			}
		}
		for _, policy := range service.Config.Policies {
			entries = append(entries, panelPolicyEntry{Origin: "config", Policy: policies[policy.Name]})
		}
		var lines []string
		for _, entry := range entries {
			roles := make([]string, 0, len(entry.Policy.Roles))
			for _, role := range entry.Policy.Roles {
				label := role.Name
				if role.Optional {
					label += " (optional)"
				}
				roles = append(roles, label)
			}
			lines = append(lines, fmt.Sprintf("%-14s %-8s min=%d families=%d  %s", entry.Policy.Name, entry.Origin, entry.Policy.MinimumPanel, entry.Policy.IndependentFamilies, strings.Join(roles, ", ")))
		}
		return printValue(cmd, f, map[string]any{"schema_version": 1, "policies": entries}, strings.Join(lines, "\n"))
	}}
}

type panelPolicyEntry struct {
	Origin string             `json:"origin"`
	Policy domain.PanelPolicy `json:"policy"`
}

func newPanelShowCommand(f *flags) *cobra.Command {
	return &cobra.Command{Use: "show [file-or-folder]", Args: cobra.MaximumNArgs(1), Short: "Resolve the panel a review would use, without calling any model", RunE: func(cmd *cobra.Command, args []string) error {
		service, err := serviceFor(firstArg(args, "."), f)
		if err != nil {
			return err
		}
		preview, err := service.PanelPreview(app.ReviewOptions{Panel: f.Panel, PanelPolicy: f.PanelPolicy})
		if err != nil {
			return err
		}
		var lines []string
		header := "source=" + preview.Source
		if preview.Policy != "" {
			header += " policy=" + preview.Policy
		}
		lines = append(lines, header)
		for _, reviewer := range preview.Panel.Reviewers {
			lines = append(lines, fmt.Sprintf("%-6s %-18s %-32s family=%-12s persona=%s", reviewer.ID, reviewer.Adapter, reviewer.Model, reviewer.Family, reviewer.Persona))
		}
		lines = append(lines, preview.DiversityNote)
		if preview.Selection != nil {
			for _, seat := range preview.Selection.Seats {
				lines = append(lines, "  "+seat.Rationale)
			}
			for _, note := range preview.Selection.Notes {
				lines = append(lines, "  note: "+note)
			}
		}
		return printValue(cmd, f, preview, strings.Join(lines, "\n"))
	}}
}
