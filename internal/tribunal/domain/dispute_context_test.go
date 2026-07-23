package domain

import (
	"strings"
	"testing"
)

func contextFixtureCluster(category Category, decision Decision) Cluster {
	finding := Finding{SchemaVersion: FindingSchemaVersion, ID: "F-1", Reviewer: "R-001", Origin: "panel", Severity: SeverityBlocker, Category: category, Anchor: Anchor{Kind: "quote", PacketItem: "artifact:a.md", Quote: "q", ItemSHA256: "s"}, Issue: "i", Recommendation: "r", EvidenceStatus: EvidenceAnchored, Confidence: "high"}
	return Cluster{SchemaVersion: SchemaVersion, ID: "C-1111111111111111", Category: category, MemberIDs: []string{"F-1"}, Finding: finding, Decision: &decision}
}

// L-03 regression: the live C01/C08 geometry — strict category, incomplete
// panel (2 of 3 valid), both valid reviewers accepting — must carry a
// context explaining this is panel geometry, not a substantive split.
func TestDisputeContextExplainsIncompleteStrictPanel(t *testing.T) {
	votes := []Vote{
		{SchemaVersion: SchemaVersion, ReviewerID: "R-002", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityBlocker},
		{SchemaVersion: SchemaVersion, ReviewerID: "R-003", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityBlocker},
	}
	cluster := contextFixtureCluster(CategorySecurity, Decision{})
	decision := ResolveVotes(cluster.Finding, votes, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 2})
	if decision.Reason != "category_requires_full_panel_unanimity" {
		t.Fatalf("setup: reason = %s", decision.Reason)
	}
	cluster.Decision = &decision
	disputes, _ := RankArbitration([]Cluster{cluster}, 0)
	if len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d", len(disputes))
	}
	context := disputes[0].Context
	for _, required := range []string{"2 of 3 configured reviewers were valid", "2 accepted, 0 rejected", "incomplete panel"} {
		if !strings.Contains(context, required) {
			t.Fatalf("context %q missing %q", context, required)
		}
	}
}

func TestDisputeContextForNonDecidableAndUnanimityReasons(t *testing.T) {
	abstainVotes := []Vote{
		{SchemaVersion: SchemaVersion, ReviewerID: "R-001", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityMajor},
		{SchemaVersion: SchemaVersion, ReviewerID: "R-002", FindingID: "F-1", Choice: VoteAbstain, Severity: SeverityMajor},
		{SchemaVersion: SchemaVersion, ReviewerID: "R-003", FindingID: "F-1", Choice: VoteAbstain, Severity: SeverityMajor},
	}
	cluster := contextFixtureCluster(CategoryCorrectness, Decision{})
	decision := ResolveVotes(cluster.Finding, abstainVotes, ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3})
	if decision.Reason != "insufficient_non_abstain_votes" {
		t.Fatalf("setup: reason = %s", decision.Reason)
	}
	cluster.Decision = &decision
	disputes, _ := RankArbitration([]Cluster{cluster}, 0)
	if !strings.Contains(disputes[0].Context, "not decidable") {
		t.Fatalf("abstain-heavy context = %q", disputes[0].Context)
	}
}

// A plain vote tie already reads as what it is; no gloss is added, so the
// pre-fix rendering of ordinary disputes is unchanged.
func TestDisputeContextAbsentForPlainTie(t *testing.T) {
	votes := []Vote{
		{SchemaVersion: SchemaVersion, ReviewerID: "R-001", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityMajor},
		{SchemaVersion: SchemaVersion, ReviewerID: "R-002", FindingID: "F-1", Choice: VoteReject, Severity: SeverityMajor},
	}
	cluster := contextFixtureCluster(CategoryCorrectness, Decision{})
	decision := ResolveVotes(cluster.Finding, votes, ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
	if decision.Reason != "vote_tie" {
		t.Fatalf("setup: reason = %s", decision.Reason)
	}
	cluster.Decision = &decision
	disputes, _ := RankArbitration([]Cluster{cluster}, 0)
	if disputes[0].Context != "" {
		t.Fatalf("plain tie gained a context gloss: %q", disputes[0].Context)
	}
}
