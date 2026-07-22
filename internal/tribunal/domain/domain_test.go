package domain

import "testing"

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
