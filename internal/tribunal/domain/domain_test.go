package domain

import (
	"math"
	"testing"
)

func TestParsePanelPreservesModelGrammar(t *testing.T) {
	panel, err := ParsePanel("ollama/gemma4:27b,codex/hf/meta-llama/Llama-3-70B@methodologist")
	if err != nil {
		t.Fatal(err)
	}
	if panel.Reviewers[0].Model != "gemma4:27b" || panel.Reviewers[1].Model != "hf/meta-llama/Llama-3-70B" {
		t.Fatalf("models not preserved: %#v", panel.Reviewers)
	}
	if panel.Reviewers[1].Persona != "methodologist" {
		t.Fatalf("persona = %q", panel.Reviewers[1].Persona)
	}
}

func TestParsePanelPreservesNonPersonaAtSuffix(t *testing.T) {
	panel, err := ParsePanel("openai-compatible/vendor/model@2026.07,codex/gpt-5.6-sol@methodologist")
	if err != nil {
		t.Fatal(err)
	}
	if panel.Reviewers[0].Model != "vendor/model@2026.07" || panel.Reviewers[0].Persona != "plain" {
		t.Fatalf("non-persona suffix was not preserved: %#v", panel.Reviewers[0])
	}
	if panel.Reviewers[1].Model != "gpt-5.6-sol" || panel.Reviewers[1].Persona != "methodologist" {
		t.Fatalf("valid persona suffix was not parsed: %#v", panel.Reviewers[1])
	}
}

func TestDiversityNoteIsDeterministicForMultipleDuplicateFamilies(t *testing.T) {
	panel := Panel{SchemaVersion: SchemaVersion, Reviewers: []Panelist{
		{ID: "R-001", Adapter: "a", Model: "one", Family: "zeta", Weight: 1, MaxContextTokens: 2, ReservedOutputTokens: 1},
		{ID: "R-002", Adapter: "b", Model: "two", Family: "alpha", Weight: 1, MaxContextTokens: 2, ReservedOutputTokens: 1},
		{ID: "R-003", Adapter: "c", Model: "three", Family: "zeta", Weight: 1, MaxContextTokens: 2, ReservedOutputTokens: 1},
		{ID: "R-004", Adapter: "d", Model: "four", Family: "alpha", Weight: 1, MaxContextTokens: 2, ReservedOutputTokens: 1},
	}}
	const want = "2 of 4 reviewers share the alpha family; treat agreement as correlated"
	for i := 0; i < 64; i++ {
		if got := DiversityNote(panel); got != want {
			t.Fatalf("diversity note = %q, want %q", got, want)
		}
	}
}

func TestNormalizePanelClampsWeights(t *testing.T) {
	panel, _ := ParsePanel("a/one,b/two")
	panel.Reviewers[0].Weight = 0.1
	panel.Reviewers[1].Weight = 9
	if err := NormalizePanel(&panel); err != nil {
		t.Fatal(err)
	}
	if panel.Reviewers[0].Weight != 0.5 || panel.Reviewers[1].Weight != 2 {
		t.Fatalf("weights were not clamped: %#v", panel.Reviewers)
	}
}

func TestNormalizePanelRejectsNonFiniteWeight(t *testing.T) {
	for _, weight := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		panel, err := ParsePanel("a/one")
		if err != nil {
			t.Fatal(err)
		}
		panel.Reviewers[0].Weight = weight
		if err := NormalizePanel(&panel); err == nil {
			t.Fatalf("NormalizePanel accepted non-finite weight %v", weight)
		}
	}
}

func TestResolveVotesEdges(t *testing.T) {
	finding := Finding{ID: "F-1", Severity: SeverityMajor, Category: CategoryCorrectness, EvidenceStatus: EvidenceAnchored}
	tests := []struct {
		name    string
		votes   []Vote
		opts    ConsensusOptions
		outcome string
	}{
		{"majority", []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityMajor}, {ReviewerID: "b", Choice: VoteAccept, Severity: SeverityMinor}, {ReviewerID: "c", Choice: VoteReject, Severity: SeverityMajor}}, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3}, "accepted"},
		{"tie", []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityMajor}, {ReviewerID: "b", Choice: VoteReject, Severity: SeverityMajor}}, ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2}, "arbitration"},
		{"quorum", []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityMajor}}, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 1}, "degraded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveVotes(finding, test.votes, test.opts).Outcome; got != test.outcome {
				t.Fatalf("outcome = %q, want %q", got, test.outcome)
			}
		})
	}
}

func TestStrictCategoryNeedsConfiguredPanel(t *testing.T) {
	finding := Finding{ID: "F-1", Severity: SeverityMajor, Category: CategorySecurity, EvidenceStatus: EvidenceAnchored}
	votes := []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityMajor}, {ReviewerID: "b", Choice: VoteAccept, Severity: SeverityMajor}}
	decision := ResolveVotes(finding, votes, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 2})
	if decision.Outcome != "arbitration" {
		t.Fatalf("outcome = %q", decision.Outcome)
	}
}

func TestConsensusWeightAbstainSeverityEvidenceAndDissent(t *testing.T) {
	finding := Finding{ID: "F-1", Severity: SeverityBlocker, Category: CategoryCorrectness, EvidenceStatus: EvidenceAnchored}
	votes := []Vote{
		{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityBlocker, Reason: "accept"},
		{ReviewerID: "b", Choice: VoteReject, Severity: SeverityMinor, Reason: "reject"},
		{ReviewerID: "c", Choice: VoteAbstain, Severity: SeverityNit, Reason: "insufficient context"},
	}
	decision := ResolveVotes(finding, votes, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"a": 2, "b": 0.5, "c": 1}})
	if decision.Outcome != "accepted" || decision.Abstains != 1 || decision.Severity != SeverityMinor || len(decision.Dissent) != 2 {
		t.Fatalf("unexpected weighted decision: %#v", decision)
	}
	finding.Category, finding.EvidenceStatus = CategoryFactualClaim, EvidenceUnevidenced
	decision = ResolveVotes(finding, []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityBlocker}, {ReviewerID: "b", Choice: VoteAccept, Severity: SeverityMajor}}, ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
	if decision.Outcome != "unverified-claim" || decision.Severity != SeverityMinor {
		t.Fatalf("unevidenced factual claim escaped gate: %#v", decision)
	}
}

func TestUnevidencedSeverityCapAppliesToEveryCategory(t *testing.T) {
	categories := []Category{CategoryCorrectness, CategoryEvidence, CategoryCitationIntegrity, CategoryFactualClaim, CategorySecurity, CategoryDataLoss, CategoryIntegrity, CategoryStyle, CategoryScope, CategoryStructure}
	for _, category := range categories {
		t.Run(string(category), func(t *testing.T) {
			finding := Finding{ID: "F-1", Severity: SeverityBlocker, Category: category, EvidenceStatus: EvidenceUnevidenced}
			votes := []Vote{{ReviewerID: "a", Choice: VoteAccept, Severity: SeverityBlocker}, {ReviewerID: "b", Choice: VoteAccept, Severity: SeverityBlocker}}
			decision := ResolveVotes(finding, votes, ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
			if decision.Severity != SeverityMinor {
				t.Fatalf("unevidenced %s retained severity %s", category, decision.Severity)
			}
		})
	}
}

func TestRuleClusteringPreservesMembers(t *testing.T) {
	anchor := Anchor{PacketItem: "artifact:x.md", ItemSHA256: "abc", Quote: "same quote", CharOffset: 2, EndOffset: 12}
	findings := []Finding{{ID: "F-1", Category: CategoryStyle, Severity: SeverityNit, Anchor: anchor}, {ID: "F-2", Category: CategoryStyle, Severity: SeverityMajor, Anchor: anchor}}
	clusters := ClusterFindings(findings)
	if len(clusters) != 1 || len(clusters[0].MemberIDs) != 2 || clusters[0].Finding.Severity != SeverityMajor {
		t.Fatalf("unexpected clusters: %#v", clusters)
	}
}

func TestDecisionMemoryRequiresExactItemAndFingerprint(t *testing.T) {
	finding := Finding{ID: "F-1", Category: CategoryCorrectness, Severity: SeverityMajor, Anchor: Anchor{PacketItem: "artifact:x.md", ItemSHA256: "abc", Quote: "same quote"}, Issue: "issue"}
	fingerprint := FindingFingerprint(finding)
	if !MatchesDecisionMemory(finding, finding.Anchor.PacketItem, fingerprint) {
		t.Fatal("expected exact decision-memory match")
	}
	if MatchesDecisionMemory(finding, "artifact:other.md", fingerprint) || MatchesDecisionMemory(finding, finding.Anchor.PacketItem, "deadbeef") {
		t.Fatal("decision memory matched non-identical scope")
	}
}
