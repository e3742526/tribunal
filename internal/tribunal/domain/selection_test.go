package domain

import (
	"strings"
	"testing"
)

func testCatalog() []ModelCandidate {
	return []ModelCandidate{
		{ID: "opus", Adapter: "claude", Model: "claude-opus-5", Family: "anthropic", Capabilities: []string{"document-analysis", "reasoning"}, Quality: 0.9, Reliability: 0.9, Cost: 1.0, MaxContextTokens: 200000, ReservedOutputTokens: 16384},
		{ID: "sonnet", Adapter: "claude", Model: "claude-sonnet-5", Family: "anthropic", Capabilities: []string{"reasoning"}, Quality: 0.8, Reliability: 0.9, Cost: 0.4, MaxContextTokens: 200000, ReservedOutputTokens: 16384},
		{ID: "sol", Adapter: "codex", Model: "gpt-5.6-sol", Family: "openai", Capabilities: []string{"contradiction-detection", "reasoning"}, Quality: 0.88, Reliability: 0.85, Cost: 0.9, MaxContextTokens: 200000, ReservedOutputTokens: 16384},
		{ID: "gemini", Adapter: "agy", Model: "Gemini 3.5 Flash (Medium)", Family: "google", Capabilities: []string{"contradiction-detection"}, Quality: 0.7, Reliability: 0.8, Cost: 0.2, MaxContextTokens: 200000, ReservedOutputTokens: 16384},
		{ID: "mistral", Adapter: "openai-compatible", Model: "mistral-large", Family: "mistral", Capabilities: []string{"literal-reading"}, Quality: 0.6, Reliability: 0.75, Cost: 0.1, MaxContextTokens: 131072, ReservedOutputTokens: 16384},
	}
}

func testPolicy() PanelPolicy {
	return PanelPolicy{
		SchemaVersion: SchemaVersion,
		Name:          "balanced",
		Roles: []PanelRole{
			{Name: "expert-reviewer", Persona: "methodologist", Prefer: []string{"reasoning", "document-analysis"}},
			{Name: "foundation-reviewer", Persona: "foundation", Prefer: []string{"literal-reading"}},
			{Name: "adversarial-reviewer", Persona: "skeptic", Prefer: []string{"contradiction-detection"}},
		},
		MinimumPanel:        3,
		IndependentFamilies: 3,
		DiversityWeight:     0.6,
		ReliabilityWeight:   0.3,
		CostWeight:          0.2,
	}
}

func TestSelectPanelSatisfiesFamilyIndependence(t *testing.T) {
	selection, err := SelectPanel(testPolicy(), testCatalog())
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	if len(selection.Panel.Reviewers) != 3 {
		t.Fatalf("expected three seated reviewers, got %d", len(selection.Panel.Reviewers))
	}
	if len(selection.Families) != 3 {
		t.Fatalf("expected three independent families, got %v", selection.Families)
	}
	if len(selection.Notes) != 0 {
		t.Fatalf("a fully satisfied policy must record no shortfall notes, got %v", selection.Notes)
	}
	personas := map[string]string{}
	for _, seat := range selection.Seats {
		personas[seat.Role] = seat.Persona
	}
	if personas["foundation-reviewer"] != "foundation" {
		t.Fatalf("policy persona must reach the seat, got %v", personas)
	}
}

// The whole point of the diversity term is that it can outrank raw quality.
// With two anthropic models available, a family-blind resolver would seat
// opus and sonnet together; this asserts it does not.
func TestSelectPanelPrefersIndependenceOverDuplicateFamilyQuality(t *testing.T) {
	selection, err := SelectPanel(testPolicy(), testCatalog())
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	counts := map[string]int{}
	for _, reviewer := range selection.Panel.Reviewers {
		counts[reviewer.Family]++
	}
	for family, count := range counts {
		if count > 1 {
			t.Fatalf("family %s seated %d times despite independent_families=3", family, count)
		}
	}
}

func TestSelectPanelIsDeterministic(t *testing.T) {
	policy, catalog := testPolicy(), testCatalog()
	first, err := SelectPanel(policy, catalog)
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	// Reordering the catalog must not reorder the panel: selection keys on
	// declared scores and candidate ids, never on input order.
	shuffled := []ModelCandidate{catalog[3], catalog[0], catalog[4], catalog[2], catalog[1]}
	for i := 0; i < 5; i++ {
		next, err := SelectPanel(policy, shuffled)
		if err != nil {
			t.Fatalf("select panel: %v", err)
		}
		if len(next.Seats) != len(first.Seats) {
			t.Fatalf("seat count changed between runs")
		}
		for j := range first.Seats {
			if next.Seats[j].CandidateID != first.Seats[j].CandidateID || next.Seats[j].ReviewerID != first.Seats[j].ReviewerID {
				t.Fatalf("selection is not deterministic: %v vs %v", next.Seats[j], first.Seats[j])
			}
		}
	}
}

func TestSelectPanelReportsInfeasiblePolicyInsteadOfShrinking(t *testing.T) {
	policy := testPolicy()
	policy.IndependentFamilies = 3
	catalog := []ModelCandidate{
		{ID: "opus", Adapter: "claude", Model: "claude-opus-5", Family: "anthropic", Quality: 0.9, Reliability: 0.9, Cost: 1},
		{ID: "sonnet", Adapter: "claude", Model: "claude-sonnet-5", Family: "anthropic", Quality: 0.8, Reliability: 0.9, Cost: 0.4},
		{ID: "haiku", Adapter: "claude", Model: "claude-haiku-4-5", Family: "anthropic", Quality: 0.6, Reliability: 0.9, Cost: 0.1},
	}
	if _, err := SelectPanel(policy, catalog); err == nil || !strings.Contains(err.Error(), "cannot be satisfied") {
		t.Fatalf("single-family catalog must fail an independence policy, got %v", err)
	}
}

func TestSelectPanelRejectsCatalogSmallerThanQuorum(t *testing.T) {
	catalog := testCatalog()[:2]
	if _, err := SelectPanel(testPolicy(), catalog); err == nil || !strings.Contains(err.Error(), "catalog offers") {
		t.Fatalf("expected a catalog-size error, got %v", err)
	}
}

func TestSelectPanelRecordsUnfilledOptionalSeat(t *testing.T) {
	policy := testPolicy()
	policy.Roles = append(policy.Roles, PanelRole{Name: "domain-specialist", Persona: "governor", Require: []string{"legal-analysis"}, Optional: true})
	selection, err := SelectPanel(policy, testCatalog())
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	if len(selection.Panel.Reviewers) != 3 {
		t.Fatalf("unfillable optional seat must not be filled, got %d reviewers", len(selection.Panel.Reviewers))
	}
	found := false
	for _, note := range selection.Notes {
		if strings.Contains(note, "optional role domain-specialist left unfilled") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a dropped optional seat must be reported, notes=%v", selection.Notes)
	}
}

func TestSelectPanelRejectsUnfillableRequiredRole(t *testing.T) {
	policy := testPolicy()
	policy.Roles[1].Require = []string{"legal-analysis"}
	if _, err := SelectPanel(policy, testCatalog()); err == nil || !strings.Contains(err.Error(), "no catalog model declares") {
		t.Fatalf("expected an unfillable required role error, got %v", err)
	}
}

func TestSelectPanelHonorsCostWeight(t *testing.T) {
	catalog := []ModelCandidate{
		{ID: "cheap", Adapter: "a", Model: "cheap", Family: "fa", Capabilities: []string{"reasoning"}, Quality: 0.7, Reliability: 0.9, Cost: 0},
		{ID: "pricey", Adapter: "b", Model: "pricey", Family: "fb", Capabilities: []string{"reasoning"}, Quality: 0.72, Reliability: 0.9, Cost: 10},
		{ID: "third", Adapter: "c", Model: "third", Family: "fc", Capabilities: []string{"reasoning"}, Quality: 0.5, Reliability: 0.9, Cost: 0},
	}
	policy := PanelPolicy{
		SchemaVersion: SchemaVersion,
		Name:          "frugal",
		Roles: []PanelRole{
			{Name: "primary", Prefer: []string{"reasoning"}},
			{Name: "second", Prefer: []string{"reasoning"}},
		},
		MinimumPanel: 2, IndependentFamilies: 2, DiversityWeight: 0.1, ReliabilityWeight: 0.1, CostWeight: 1.0,
	}
	selection, err := SelectPanel(policy, catalog)
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	for _, seat := range selection.Seats {
		if seat.CandidateID == "pricey" {
			t.Fatalf("a dominant cost weight must exclude the marginally better but 10x costlier model: %v", selection.Seats)
		}
	}
}

func TestValidatePolicyRejectsIncoherentQuorum(t *testing.T) {
	cases := map[string]func(*PanelPolicy){
		"minimum_panel must be at least 2":  func(p *PanelPolicy) { p.MinimumPanel = 1 },
		"exceeds its":                       func(p *PanelPolicy) { p.MinimumPanel = 9 },
		"exceeds minimum_panel":             func(p *PanelPolicy) { p.IndependentFamilies = 4; p.MinimumPanel = 3 },
		"must be a finite non-negative num": func(p *PanelPolicy) { p.DiversityWeight = -1 },
	}
	for want, mutate := range cases {
		policy := testPolicy()
		mutate(&policy)
		err := ValidatePolicy(policy)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q, got %v", want, err)
		}
	}
}

func TestValidatePolicyRejectsMoreRequiredRolesThanQuorum(t *testing.T) {
	policy := testPolicy()
	policy.MinimumPanel = 2
	policy.IndependentFamilies = 2
	if err := ValidatePolicy(policy); err == nil || !strings.Contains(err.Error(), "non-optional roles") {
		t.Fatalf("expected a required-role/quorum conflict, got %v", err)
	}
}

func TestValidateCandidateRejectsOutOfRangePriors(t *testing.T) {
	candidate := ModelCandidate{ID: "x", Adapter: "claude", Model: "m", Quality: 1.5, Reliability: 0.5}
	if err := ValidateCandidate(candidate); err == nil || !strings.Contains(err.Error(), "within 0..1") {
		t.Fatalf("expected a range error, got %v", err)
	}
	candidate = ModelCandidate{ID: "Bad ID", Adapter: "claude", Model: "m"}
	if err := ValidateCandidate(candidate); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected a slug error, got %v", err)
	}
}

func TestCapabilityFitRanksEarlierPreferencesHigher(t *testing.T) {
	prefer := []string{"reasoning", "document-analysis"}
	first := capabilityFit(prefer, []string{"reasoning"})
	second := capabilityFit(prefer, []string{"document-analysis"})
	if !(first > second) {
		t.Fatalf("earlier preference must weigh more: %f vs %f", first, second)
	}
	if got := capabilityFit(nil, []string{"reasoning"}); got != 0 {
		t.Fatalf("an empty preference list must be neutral, got %f", got)
	}
	if got := capabilityFit(prefer, []string{"reasoning", "document-analysis"}); got != 1 {
		t.Fatalf("a full match must score 1, got %f", got)
	}
}

func TestSelectedPanelPassesPanelNormalization(t *testing.T) {
	selection, err := SelectPanel(testPolicy(), testCatalog())
	if err != nil {
		t.Fatalf("select panel: %v", err)
	}
	panel := selection.Panel
	if err := NormalizePanel(&panel); err != nil {
		t.Fatalf("selected panel must satisfy the panel contract: %v", err)
	}
}
