package domain

import (
	"strings"
	"testing"
)

// Weighted sums that are decimal-equal must tie exactly: 0.7+0.5 vs 1.2
// diverges under float64 accumulation but not under integer hundredths.
func TestWeightedVoteTieIsExact(t *testing.T) {
	finding := Finding{ID: "F-1", Severity: SeverityMajor, Category: CategoryCorrectness}
	votes := []Vote{
		{ReviewerID: "R-001", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityMajor},
		{ReviewerID: "R-002", FindingID: "F-1", Choice: VoteAccept, Severity: SeverityMajor},
		{ReviewerID: "R-003", FindingID: "F-1", Choice: VoteReject, Severity: SeverityMajor},
	}
	opts := ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"R-001": 0.7, "R-002": 0.5, "R-003": 1.2}}
	decision := ResolveVotes(finding, votes, opts)
	if decision.Outcome != "arbitration" || decision.Reason != "vote_tie" {
		t.Fatalf("decision = %s/%s, want arbitration/vote_tie", decision.Outcome, decision.Reason)
	}
}

func TestFindingFingerprintIsSixtyFourBits(t *testing.T) {
	fp := FindingFingerprint(Finding{Category: CategoryCorrectness, Anchor: Anchor{PacketItem: "artifact:a.md", Quote: "q"}, Issue: "i"})
	if len(fp) != 16 {
		t.Fatalf("fingerprint length = %d, want 16 hex chars", len(fp))
	}
}

// A persisted cluster with a malformed ID must fail validation instead of
// panicking downstream consumers that slice the "C-" prefix.
func TestValidateClusterRejectsMalformedID(t *testing.T) {
	finding := Finding{SchemaVersion: FindingSchemaVersion, ID: "F-1", Reviewer: "R-001", Origin: "panel", Severity: SeverityMajor, Category: CategoryCorrectness, Anchor: Anchor{Kind: "quote", PacketItem: "artifact:a.md", Quote: "q", ItemSHA256: "s"}, Issue: "i", Recommendation: "r", EvidenceStatus: EvidenceAnchored, Confidence: "high"}
	base := Cluster{SchemaVersion: SchemaVersion, Category: CategoryCorrectness, MemberIDs: []string{"F-1"}, Finding: finding}
	for _, id := range []string{"C", "C-", "C-abc", "X-" + strings.Repeat("a", 16), "C-" + strings.Repeat("A", 16), "C-" + strings.Repeat("a", 15) + "!"} {
		cluster := base
		cluster.ID = id
		if err := ValidateCluster(cluster); err == nil {
			t.Fatalf("cluster id %q was accepted", id)
		}
	}
	good := base
	good.ID = "C-" + strings.Repeat("0af9", 4)
	if err := ValidateCluster(good); err != nil {
		t.Fatalf("valid cluster rejected: %v", err)
	}
}
