package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// Built-in policies declare panel *shape* — how many seats, which lenses, how
// much independence — and never name a model. Model identity, quality, and
// cost are operator-declared catalog facts, because Tribunal has no honest
// basis for shipping vendor capability claims of its own.
var starterPolicies = map[string]domain.PanelPolicy{
	"balanced": {
		SchemaVersion: domain.SchemaVersion,
		Name:          "balanced",
		Summary:       "Expert, foundation, and adversarial lenses across three independent families.",
		Roles: []domain.PanelRole{
			{Name: "expert-reviewer", Persona: "methodologist", Prefer: []string{"reasoning", "document-analysis"}},
			{Name: "foundation-reviewer", Persona: "foundation", Prefer: []string{"literal-reading", "instruction-following"}},
			{Name: "adversarial-reviewer", Persona: "skeptic", Prefer: []string{"contradiction-detection", "reasoning"}},
		},
		MinimumPanel:        3,
		IndependentFamilies: 3,
		DiversityWeight:     0.6,
		ReliabilityWeight:   0.3,
		CostWeight:          0.2,
	},
	"high-stakes": {
		SchemaVersion: domain.SchemaVersion,
		Name:          "high-stakes",
		Summary:       "Balanced lenses plus an optional governance seat; independence outweighs cost.",
		Roles: []domain.PanelRole{
			{Name: "expert-reviewer", Persona: "methodologist", Prefer: []string{"reasoning", "document-analysis"}},
			{Name: "foundation-reviewer", Persona: "foundation", Prefer: []string{"literal-reading", "instruction-following"}},
			{Name: "adversarial-reviewer", Persona: "skeptic", Prefer: []string{"contradiction-detection", "reasoning"}},
			{Name: "governance-reviewer", Persona: "governor", Prefer: []string{"domain-knowledge", "reasoning"}, Optional: true},
		},
		MinimumPanel:        3,
		IndependentFamilies: 3,
		DiversityWeight:     0.9,
		ReliabilityWeight:   0.4,
		CostWeight:          0.05,
	},
	"frugal": {
		SchemaVersion: domain.SchemaVersion,
		Name:          "frugal",
		Summary:       "Minimum quorum of two independent families with cost dominating the score.",
		Roles: []domain.PanelRole{
			{Name: "expert-reviewer", Persona: "methodologist", Prefer: []string{"reasoning", "document-analysis"}},
			{Name: "foundation-reviewer", Persona: "foundation", Prefer: []string{"literal-reading", "instruction-following"}},
		},
		MinimumPanel:        2,
		IndependentFamilies: 2,
		DiversityWeight:     0.5,
		ReliabilityWeight:   0.2,
		CostWeight:          1.0,
	},
}

// StarterPolicies returns the built-in policies in deterministic name order.
func StarterPolicies() []domain.PanelPolicy {
	names := make([]string, 0, len(starterPolicies))
	for name := range starterPolicies {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]domain.PanelPolicy, 0, len(names))
	for _, name := range names {
		values = append(values, starterPolicies[name])
	}
	return values
}

// ResolvePanelPolicy prefers a trusted-config policy over a built-in of the
// same name, matching how personas resolve.
func ResolvePanelPolicy(cfg Config, name string) (domain.PanelPolicy, error) {
	for _, policy := range cfg.Policies {
		if policy.Name == name {
			if err := domain.ValidatePolicy(policy); err != nil {
				return domain.PanelPolicy{}, err
			}
			return policy, nil
		}
	}
	if policy, ok := starterPolicies[name]; ok {
		return policy, nil
	}
	return domain.PanelPolicy{}, fmt.Errorf("unknown panel policy %q; run `tribunal panel list`", name)
}

// PanelCatalog returns the selectable model catalog plus any notes describing
// how it was obtained. When no `[[models]]` are configured the catalog is
// derived from the configured panel string with neutral priors: that keeps
// policy selection usable out of the box without Tribunal inventing quality,
// reliability, or cost figures for a vendor it has never measured. The
// derived catalog still carries real family identity, so the independence
// constraint remains meaningful.
func PanelCatalog(cfg Config) ([]domain.ModelCandidate, []string, error) {
	if len(cfg.Models) > 0 {
		catalog := append([]domain.ModelCandidate(nil), cfg.Models...)
		for _, candidate := range catalog {
			if err := domain.ValidateCandidate(candidate); err != nil {
				return nil, nil, err
			}
		}
		return catalog, nil, nil
	}
	panel, err := domain.ParsePanel(cfg.Panel)
	if err != nil {
		return nil, nil, err
	}
	catalog := make([]domain.ModelCandidate, 0, len(panel.Reviewers))
	used := map[string]struct{}{}
	for _, reviewer := range panel.Reviewers {
		id := uniqueSlug(reviewer.Adapter+"-"+reviewer.Model, used)
		catalog = append(catalog, domain.ModelCandidate{
			ID:      id,
			Adapter: reviewer.Adapter,
			Model:   reviewer.Model,
			Family:  reviewer.Family,
			Quality: 0.5, Reliability: 1, Cost: 1, Weight: 1,
		})
	}
	note := fmt.Sprintf("no [[models]] catalog is configured; %d candidates were derived from the configured panel with neutral quality, reliability, and cost priors", len(catalog))
	return catalog, []string{note}, nil
}

// slugify reduces a verbatim model identifier — which may contain slashes,
// colons, spaces, or parentheses — to the catalog id shape.
func slugify(value string) string {
	var out strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(value) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
			lastDash = false
		case !lastDash:
			out.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(out.String(), "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	if slug == "" {
		slug = "model"
	}
	return slug
}

func uniqueSlug(value string, used map[string]struct{}) string {
	base := slugify(value)
	candidate := base
	for index := 2; ; index++ {
		if _, taken := used[candidate]; !taken {
			used[candidate] = struct{}{}
			return candidate
		}
		suffix := fmt.Sprintf("-%d", index)
		trimmed := base
		if len(trimmed)+len(suffix) > 64 {
			trimmed = strings.Trim(base[:64-len(suffix)], "-")
		}
		candidate = trimmed + suffix
	}
}
