package app

// Playtest driver for docs/test_scenarios/02-consensus-scenarios.md.
//
// Cards S01-S34 and S41-S51 are pure vote-arithmetic scenarios: they call
// domain.ResolveVotes / domain.ClusterFindings / domain.RankArbitration
// directly with scripted votes, mirroring exactly what Service.Review's
// panel pipeline would produce for those inputs, without the cost of
// spinning up 30+ full FuncAdapter review runs. Cards marked
// [DISAGREE->CONSENSUS] that need the full Review->Arbitrate loop (S35-S40,
// S52) run through the real Service in consensus_playtest_e2e_test.go.

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func find(id string, category domain.Category, severity domain.Severity, evidence domain.EvidenceStatus, item, quote string, start int) domain.Finding {
	return domain.Finding{
		SchemaVersion: domain.FindingSchemaVersion, ID: id, Reviewer: "R-001", Origin: "panel",
		Severity: severity, Category: category,
		Anchor: domain.Anchor{Kind: "quote", PacketItem: item, Quote: quote, ItemSHA256: "sha", CharOffset: start, EndOffset: start + len(quote)},
		Issue:  "issue", Recommendation: "fix it",
		EvidenceStatus: evidence, Confidence: "high",
	}
}

func vote(reviewer, findingID string, choice domain.VoteChoice, severity domain.Severity) domain.Vote {
	return domain.Vote{SchemaVersion: domain.SchemaVersion, ReviewerID: reviewer, FindingID: findingID, Choice: choice, Severity: severity, Reason: "reason"}
}

type consensusCase struct {
	id           string
	f            domain.Finding
	votes        []domain.Vote
	opts         domain.ConsensusOptions
	wantOutcome  string
	wantReason   string
	wantAccepts  int
	wantRejects  int
	wantDissent  int
	wantSeverity domain.Severity
}

// TestConsensusPlaytest_VoteArithmetic runs docs/test_scenarios/02-consensus-scenarios.md
// cards S01-S34 and S41-S51 against the real domain.ResolveVotes function.
func TestConsensusPlaytest_VoteArithmetic(t *testing.T) {
	major, minor, nit, blocker := domain.SeverityMajor, domain.SeverityMinor, domain.SeverityNit, domain.SeverityBlocker

	cases := []consensusCase{
		{id: "S01", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "artifact:a.md", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S02", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteReject, major), vote("R2", "F1", domain.VoteReject, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "rejected", wantReason: "majority_reject"},

		{id: "S03", f: find("F1", domain.CategoryStyle, minor, domain.EvidenceAnchored, "a", "q", 0),
			votes: []domain.Vote{vote("R1", "F1", domain.VoteAccept, minor), vote("R2", "F1", domain.VoteAccept, minor), vote("R3", "F1", domain.VoteReject, minor)},
			opts:  domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			// The lone reject vote is recorded as dissent on the accepted
			// outcome (rejectsAccepted), even at matching severity.
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 1},

		{id: "S04", f: find("F1", domain.CategoryStyle, minor, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, minor), vote("R2", "F1", domain.VoteReject, minor), vote("R3", "F1", domain.VoteReject, minor)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "rejected", wantReason: "majority_reject"},

		{id: "S05", f: find("F1", domain.CategoryStyle, minor, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, minor), vote("R2", "F1", domain.VoteAccept, minor), vote("R3", "F1", domain.VoteReject, minor), vote("R4", "F1", domain.VoteReject, minor)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 4, ValidReviewers: 4},
			wantOutcome: "arbitration", wantReason: "vote_tie"},

		{id: "S06", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("Senior", "F1", domain.VoteReject, major), vote("J1", "F1", domain.VoteAccept, major), vote("J2", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 1.0, "J2": 1.0}},
			wantOutcome: "arbitration", wantReason: "vote_tie"},

		{id: "S07", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("Senior", "F1", domain.VoteAccept, major), vote("J1", "F1", domain.VoteReject, major), vote("J2", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 1.0, "J2": 1.0}},
			wantOutcome: "arbitration", wantReason: "vote_tie"},

		{id: "S08", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes: []domain.Vote{vote("Senior", "F1", domain.VoteAccept, major), vote("J1", "F1", domain.VoteReject, major), vote("J2", "F1", domain.VoteReject, major)},
			opts:  domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 0.5, "J2": 0.5}},
			// Both outvoted reject votes are dissent on the accepted outcome.
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 2},

		{id: "S09", f: find("F1", domain.CategorySecurity, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S10", f: find("F1", domain.CategorySecurity, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "arbitration", wantReason: "category_requires_full_panel_unanimity"},

		{id: "S11", f: find("F1", domain.CategoryDataLoss, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 2},
			wantOutcome: "arbitration", wantReason: "category_requires_full_panel_unanimity"},

		{id: "S12", f: find("F1", domain.CategoryCitationIntegrity, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAbstain, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "arbitration", wantReason: "category_requires_full_panel_unanimity"},

		// S13/S14/S15 are panel-quorum cards (finalizeDegraded short-circuit
		// before per-finding voting even happens); exercised end-to-end in
		// consensus_playtest_e2e_test.go, not via ResolveVotes.

		{id: "S16", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAbstain, major), vote("R3", "F1", domain.VoteAbstain, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "arbitration", wantReason: "insufficient_non_abstain_votes"},

		{id: "S17", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAbstain, major), vote("R2", "F1", domain.VoteAbstain, major), vote("R3", "F1", domain.VoteAbstain, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "arbitration", wantReason: "insufficient_non_abstain_votes"},

		{id: "S18", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Unanimous: true},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S19", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAbstain, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Unanimous: true},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S20", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Unanimous: true},
			wantOutcome: "arbitration", wantReason: "unanimity_not_reached"},

		{id: "S21", f: find("F1", domain.CategoryFactualClaim, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S22", f: find("F1", domain.CategoryFactualClaim, major, domain.EvidenceUnevidenced, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "unverified-claim", wantReason: "factual_claim_lacks_evidence"},

		{id: "S23", f: find("F1", domain.CategoryFactualClaim, major, domain.EvidenceUnevidenced, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteReject, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "rejected", wantReason: "majority_reject"},

		{id: "S24", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceUnevidenced, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S25", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, nit)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 1},

		{id: "S26", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 1},

		{id: "S27", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes: []domain.Vote{vote("R1", "F1", domain.VoteAccept, nit), vote("R2", "F1", domain.VoteAccept, minor), vote("R3", "F1", domain.VoteAccept, major), vote("R4", "F1", domain.VoteReject, major), vote("R5", "F1", domain.VoteReject, blocker)},
			opts:  domain.ConsensusOptions{ConfiguredReviewers: 5, ValidReviewers: 5},
			// Median severity is major (3rd of 5 sorted ranks). Dissent:
			// R1 (nit, rank gap 2 vs major), R4 and R5 (reject votes on an
			// accepted outcome) = 3.
			wantOutcome: "accepted", wantReason: "majority_accept", wantSeverity: major, wantDissent: 3},

		{id: "S45", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteModify, major), vote("R2", "F1", domain.VoteModify, major), vote("R3", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept", wantAccepts: 2, wantRejects: 1, wantDissent: 1},

		{id: "S46", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteModify, major), vote("R2", "F1", domain.VoteModify, major), vote("R3", "F1", domain.VoteModify, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3},
			wantOutcome: "accepted", wantReason: "majority_accept", wantAccepts: 3},

		{id: "S41", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 1, ValidReviewers: 1},
			wantOutcome: "degraded", wantReason: "quorum_unmet"},

		{id: "S42", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteReject, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2},
			wantOutcome: "arbitration", wantReason: "vote_tie"},

		{id: "S43", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2},
			wantOutcome: "accepted", wantReason: "majority_accept"},

		{id: "S44", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes: []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, major), vote("R3", "F1", domain.VoteAccept, major), vote("R4", "F1", domain.VoteAccept, major), vote("R5", "F1", domain.VoteReject, major), vote("R6", "F1", domain.VoteReject, major), vote("R7", "F1", domain.VoteReject, blocker)},
			opts:  domain.ConsensusOptions{ConfiguredReviewers: 7, ValidReviewers: 7},
			// All 3 reject votes dissent on the accepted outcome (median
			// severity is major; the blocker reject is doubly dissenting
			// but counted once in Dissent).
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 3},

		{id: "S50a-gap2", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, nit)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2},
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 1},

		{id: "S50b-gap1", f: find("F1", domain.CategoryCorrectness, major, domain.EvidenceAnchored, "a", "q", 0),
			votes:       []domain.Vote{vote("R1", "F1", domain.VoteAccept, major), vote("R2", "F1", domain.VoteAccept, minor)},
			opts:        domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2},
			wantOutcome: "accepted", wantReason: "majority_accept", wantDissent: 0},
	}

	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			decision := domain.ResolveVotes(c.f, c.votes, c.opts)
			if decision.Outcome != c.wantOutcome || decision.Reason != c.wantReason {
				t.Fatalf("%s: outcome/reason = %s/%s, want %s/%s (full=%#v)", c.id, decision.Outcome, decision.Reason, c.wantOutcome, c.wantReason, decision)
			}
			if c.wantAccepts != 0 && decision.Accepts != c.wantAccepts {
				t.Errorf("%s: Accepts = %d, want %d", c.id, decision.Accepts, c.wantAccepts)
			}
			if c.wantRejects != 0 && decision.Rejects != c.wantRejects {
				t.Errorf("%s: Rejects = %d, want %d", c.id, decision.Rejects, c.wantRejects)
			}
			if c.wantSeverity != "" && decision.Severity != c.wantSeverity {
				t.Errorf("%s: Severity = %s, want %s", c.id, decision.Severity, c.wantSeverity)
			}
			if len(decision.Dissent) != c.wantDissent {
				t.Errorf("%s: len(Dissent) = %d, want %d (dissent=%#v)", c.id, len(decision.Dissent), c.wantDissent, decision.Dissent)
			}
		})
	}
}

// S28 — even-reviewer-count median severity, isolated because it documents
// the (len-1)/2 lower-median index explicitly.
func TestConsensusPlaytest_S28_EvenCountMedian(t *testing.T) {
	f := find("F1", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, "a", "q", 0)
	votes := []domain.Vote{
		vote("R1", "F1", domain.VoteAccept, domain.SeverityNit),
		vote("R2", "F1", domain.VoteAccept, domain.SeverityMinor),
		vote("R3", "F1", domain.VoteAccept, domain.SeverityMajor),
		vote("R4", "F1", domain.VoteAccept, domain.SeverityBlocker),
	}
	opts := domain.ConsensusOptions{ConfiguredReviewers: 4, ValidReviewers: 4}
	decision := domain.ResolveVotes(f, votes, opts)
	if decision.Outcome != "accepted" {
		t.Fatalf("outcome = %s, want accepted", decision.Outcome)
	}
	// sorted ranks: nit < minor < major < blocker; (len-1)/2 = 1 -> minor.
	if decision.Severity != domain.SeverityMinor {
		t.Fatalf("median severity of 4 = %s, want %s (lower-median index (n-1)/2)", decision.Severity, domain.SeverityMinor)
	}
}

// S29/S30 — clustering: overlapping same-category findings merge; different
// categories never merge even with overlapping anchors.
func TestConsensusPlaytest_S29_S30_Clustering(t *testing.T) {
	item := "artifact:a.md"
	overlapA := find("F1", domain.CategoryCorrectness, domain.SeverityMinor, domain.EvidenceAnchored, item, "the launch date is unsupported", 10)
	overlapB := find("F2", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, item, "launch date is unsupported", 14)
	overlapB.Anchor.ItemSHA256 = overlapA.Anchor.ItemSHA256

	clusters := domain.ClusterFindings([]domain.Finding{overlapA, overlapB})
	if len(clusters) != 1 {
		t.Fatalf("S29: same-category overlapping findings produced %d clusters, want 1", len(clusters))
	}
	if len(clusters[0].MemberIDs) != 2 {
		t.Fatalf("S29: cluster member IDs = %v, want 2 members", clusters[0].MemberIDs)
	}
	if clusters[0].Finding.Severity != domain.SeverityMajor {
		t.Fatalf("S29: cluster retained severity %s, want the higher severity %s", clusters[0].Finding.Severity, domain.SeverityMajor)
	}

	diffCategory := find("F3", domain.CategoryStyle, domain.SeverityMinor, domain.EvidenceAnchored, item, "launch date is unsupported", 14)
	diffCategory.Anchor.ItemSHA256 = overlapA.Anchor.ItemSHA256
	clusters2 := domain.ClusterFindings([]domain.Finding{overlapA, diffCategory})
	if len(clusters2) != 2 {
		t.Fatalf("S30: different-category overlapping findings produced %d clusters, want 2 (no cross-category merge)", len(clusters2))
	}
}

// S31 — a finding whose anchor never resolves (Quarantined=true, set by the
// app layer before clustering) must never enter ClusterFindings.
func TestConsensusPlaytest_S31_QuarantinedExcludedFromClustering(t *testing.T) {
	item := "artifact:a.md"
	good := find("F1", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, item, "q", 0)
	quarantined := find("F2", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceUnevidenced, item, "q2", 100)
	quarantined.Quarantined = true
	quarantined.QuarantineWhy = "anchor quote occurs more than once and lacks disambiguating context"

	clusters := domain.ClusterFindings([]domain.Finding{good, quarantined})
	if len(clusters) != 1 {
		t.Fatalf("expected the quarantined finding excluded, got %d clusters", len(clusters))
	}
	if clusters[0].Finding.ID != "F1" {
		t.Fatalf("cluster contains the wrong finding: %s", clusters[0].Finding.ID)
	}
}

// S32/S33 — arbitration ranking: severity first, then smaller vote-gap.
func TestConsensusPlaytest_S32_S33_ArbitrationRanking(t *testing.T) {
	minorCluster := domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-1111111111111111", Category: domain.CategoryStyle, MemberIDs: []string{"F1"},
		Finding: find("F1", domain.CategoryStyle, domain.SeverityMinor, domain.EvidenceAnchored, "a", "q1", 0)}
	minorDecision := domain.ResolveVotes(minorCluster.Finding, []domain.Vote{vote("R1", "F1", domain.VoteAccept, domain.SeverityMinor), vote("R2", "F1", domain.VoteReject, domain.SeverityMinor)}, domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
	minorCluster.Decision = &minorDecision

	blockerCluster := domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-2222222222222222", Category: domain.CategorySecurity, MemberIDs: []string{"F2"},
		Finding: find("F2", domain.CategorySecurity, domain.SeverityBlocker, domain.EvidenceAnchored, "a", "q2", 50)}
	blockerDecision := domain.ResolveVotes(blockerCluster.Finding, []domain.Vote{vote("R1", "F2", domain.VoteAccept, domain.SeverityBlocker), vote("R2", "F2", domain.VoteReject, domain.SeverityBlocker)}, domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
	blockerCluster.Decision = &blockerDecision

	disputes, overflow := domain.RankArbitration([]domain.Cluster{minorCluster, blockerCluster}, 0)
	if len(overflow) != 0 {
		t.Fatalf("unexpected overflow: %v", overflow)
	}
	if len(disputes) != 2 {
		t.Fatalf("expected 2 disputes, got %d", len(disputes))
	}
	if disputes[0].Decision.Severity != domain.SeverityBlocker {
		t.Fatalf("S32: first-ranked dispute severity = %s, want blocker (severity-first ordering)", disputes[0].Decision.Severity)
	}
}

// S34 — arbitration overflow beyond MaxArbitration.
func TestConsensusPlaytest_S34_ArbitrationOverflow(t *testing.T) {
	var clusters []domain.Cluster
	for i := 0; i < 6; i++ {
		id := string(rune('A' + i))
		f := find("F"+id, domain.CategoryStyle, domain.SeverityMinor, domain.EvidenceAnchored, "a", "q"+id, i*10)
		decision := domain.ResolveVotes(f, []domain.Vote{vote("R1", f.ID, domain.VoteAccept, domain.SeverityMinor), vote("R2", f.ID, domain.VoteReject, domain.SeverityMinor)}, domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
		clusters = append(clusters, domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-" + id + "111111111111111", Category: domain.CategoryStyle, MemberIDs: []string{f.ID}, Finding: f, Decision: &decision})
	}
	disputes, overflow := domain.RankArbitration(clusters, 3)
	if len(disputes) != 3 {
		t.Fatalf("expected 3 disputes with MaxArbitration=3, got %d", len(disputes))
	}
	if len(overflow) != 3 {
		t.Fatalf("expected 3 overflowed IDs, got %d: %v", len(overflow), overflow)
	}
}

// S47 — three findings in one run resolve to three independent outcomes.
func TestConsensusPlaytest_S47_IndependentOutcomesPerFinding(t *testing.T) {
	styleF := find("F1", domain.CategoryStyle, domain.SeverityMinor, domain.EvidenceAnchored, "a", "q1", 0)
	styleDecision := domain.ResolveVotes(styleF, []domain.Vote{vote("R1", "F1", domain.VoteAccept, domain.SeverityMinor), vote("R2", "F1", domain.VoteAccept, domain.SeverityMinor), vote("R3", "F1", domain.VoteReject, domain.SeverityMinor)}, domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3})
	if styleDecision.Outcome != "accepted" {
		t.Fatalf("style finding outcome = %s, want accepted", styleDecision.Outcome)
	}

	correctnessF := find("F2", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, "a", "q2", 50)
	correctnessDecision := domain.ResolveVotes(correctnessF, []domain.Vote{vote("R1", "F2", domain.VoteAccept, domain.SeverityMajor), vote("R2", "F2", domain.VoteReject, domain.SeverityMajor), vote("R3", "F2", domain.VoteReject, domain.SeverityMajor)}, domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3})
	if correctnessDecision.Outcome != "rejected" {
		t.Fatalf("correctness finding outcome = %s, want rejected", correctnessDecision.Outcome)
	}

	evidenceF := find("F3", domain.CategoryEvidence, domain.SeverityMinor, domain.EvidenceAnchored, "a", "q3", 100)
	evidenceDecision := domain.ResolveVotes(evidenceF, []domain.Vote{vote("R1", "F3", domain.VoteAccept, domain.SeverityMinor), vote("R2", "F3", domain.VoteReject, domain.SeverityMinor), vote("R3", "F3", domain.VoteAbstain, domain.SeverityMinor)}, domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3})
	if evidenceDecision.Outcome != "arbitration" || evidenceDecision.Reason != "vote_tie" {
		t.Fatalf("evidence finding outcome/reason = %s/%s, want arbitration/vote_tie", evidenceDecision.Outcome, evidenceDecision.Reason)
	}
}

// S51 — strict-category unanimity must be computed only over findings that
// actually reached clustering; a quarantined variant of the "same issue"
// must not be double-counted or silently required for unanimity.
func TestConsensusPlaytest_S51_StrictCategoryIgnoresQuarantined(t *testing.T) {
	item := "artifact:a.md"
	live1 := find("F1", domain.CategorySecurity, domain.SeverityBlocker, domain.EvidenceAnchored, item, "sql injection risk", 0)
	live2 := find("F2", domain.CategorySecurity, domain.SeverityBlocker, domain.EvidenceAnchored, item, "sql injection risk here", 40)
	quarantined := find("F3", domain.CategorySecurity, domain.SeverityBlocker, domain.EvidenceUnevidenced, item, "sql injection", 80)
	quarantined.Quarantined = true

	clusters := domain.ClusterFindings([]domain.Finding{live1, live2, quarantined})
	if len(clusters) != 2 {
		t.Fatalf("expected 2 live clusters (quarantined excluded), got %d", len(clusters))
	}
	// Both live findings vote-accept unanimously among the 2 configured (and
	// valid) reviewers who actually voted -- strict unanimity should pass.
	for _, cluster := range clusters {
		decision := domain.ResolveVotes(cluster.Finding, []domain.Vote{vote("R1", cluster.Finding.ID, domain.VoteAccept, domain.SeverityBlocker), vote("R2", cluster.Finding.ID, domain.VoteAccept, domain.SeverityBlocker)}, domain.ConsensusOptions{ConfiguredReviewers: 2, ValidReviewers: 2})
		if decision.Outcome != "accepted" {
			t.Fatalf("strict-category cluster %s outcome = %s, want accepted (quarantined finding must not affect the headcount)", cluster.ID, decision.Outcome)
		}
	}
}

// S49 — a reviewer's finding is raised, but its voter fails at vote time
// (distinct from the panel-level quorum check, which runs earlier). This is
// pure domain-layer: it simply confirms ResolveVotes computes correctly off
// whatever ballots actually arrive, regardless of why a voter is missing.
func TestConsensusPlaytest_S49_MissingVoterBallotsStillResolve(t *testing.T) {
	f := find("F1", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, "a", "q", 0)
	// Only 2 of 3 configured reviewers' ballots arrive (the third voter
	// failed); ValidReviewers still reflects 3 successful REVIEW calls.
	votes := []domain.Vote{vote("R1", "F1", domain.VoteAccept, domain.SeverityMajor), vote("R2", "F1", domain.VoteAccept, domain.SeverityMajor)}
	decision := domain.ResolveVotes(f, votes, domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3})
	if decision.Outcome != "accepted" {
		t.Fatalf("outcome = %s, want accepted (2 of 3 available ballots agree)", decision.Outcome)
	}
}

// Playtest-discovered bug (S06/S07 weighted-tie regression): RankArbitration
// derived a dispute's Default recommendation from raw, unweighted
// Accepts/Rejects counts even when the decision's actual outcome was a
// weighted vote_tie. A raw 2-accept/1-reject split that ties under weighting
// (e.g. the dissenting reviewer weighted 2x) produced Default="accept
// majority" on a dispute the panel had NOT actually leaned toward accepting
// -- and because arbitrationRulings' --accept-majority reads that exact
// "accept" prefix, it would have silently auto-accepted a genuine tie
// instead of routing it to a human operator.
//
// The first fix attempt special-cased Reason=="vote_tie", which an
// adversarial review caught as incomplete: category_requires_full_panel_
// unanimity and unanimity_not_reached can ALSO co-occur with a raw/weighted
// count divergence (they are checked before the weight comparison in
// ResolveVotes' switch), so that fix still mislabeled those cases. The real
// fix adds Decision.WeightedLean, computed once directly from the weighted
// sums independent of which Reason ultimately fires, and makes
// defaultRecommendation read WeightedLean exclusively. This test and
// TestConsensusPlaytest_StrictCategoryWeightedMismatchNeverReadsAsMajority
// cover both the original vote_tie case and the sibling bypass.
func TestConsensusPlaytest_WeightedTieNeverReadsAsMajority(t *testing.T) {
	f := find("F1", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, "a", "q", 0)
	votes := []domain.Vote{
		vote("Senior", "F1", domain.VoteReject, domain.SeverityMajor),
		vote("J1", "F1", domain.VoteAccept, domain.SeverityMajor),
		vote("J2", "F1", domain.VoteAccept, domain.SeverityMajor),
	}
	opts := domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 1.0, "J2": 1.0}}
	decision := domain.ResolveVotes(f, votes, opts)
	if decision.Outcome != "arbitration" || decision.Reason != "vote_tie" {
		t.Fatalf("setup: decision = %s/%s, want arbitration/vote_tie", decision.Outcome, decision.Reason)
	}
	if decision.Accepts <= decision.Rejects {
		t.Fatalf("setup: raw counts must favor accept (2 vs 1) for this regression to be meaningful, got Accepts=%d Rejects=%d", decision.Accepts, decision.Rejects)
	}
	cluster := domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-1111111111111111", Category: domain.CategoryCorrectness, MemberIDs: []string{"F1"}, Finding: f, Decision: &decision}
	disputes, _ := domain.RankArbitration([]domain.Cluster{cluster}, 0)
	if len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d", len(disputes))
	}
	if strings.HasPrefix(disputes[0].Default, "accept") {
		t.Fatalf("regression: weighted tie's Default = %q, must never read as an accept majority", disputes[0].Default)
	}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, domain.Final{Arbitration: disputes})
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 1 || rulings[0].Outcome == "accepted" {
		t.Fatalf("regression: --accept-majority auto-accepted a genuine weighted tie: %#v", rulings)
	}
}

// The sibling bypass an adversarial review found in the first fix attempt:
// a strict-category (Security) decision with a raw 2-accept/1-reject split
// that is an exact weighted tie fails on category_requires_full_panel_
// unanimity, not vote_tie -- but the raw-count divergence is identical, and
// --accept-majority must not auto-accept it either.
func TestConsensusPlaytest_StrictCategoryWeightedMismatchNeverReadsAsMajority(t *testing.T) {
	f := find("F1", domain.CategorySecurity, domain.SeverityBlocker, domain.EvidenceAnchored, "a", "q", 0)
	votes := []domain.Vote{
		vote("Senior", "F1", domain.VoteReject, domain.SeverityBlocker),
		vote("J1", "F1", domain.VoteAccept, domain.SeverityBlocker),
		vote("J2", "F1", domain.VoteAccept, domain.SeverityBlocker),
	}
	opts := domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 1.0, "J2": 1.0}}
	decision := domain.ResolveVotes(f, votes, opts)
	if decision.Outcome != "arbitration" || decision.Reason != "category_requires_full_panel_unanimity" {
		t.Fatalf("setup: decision = %s/%s, want arbitration/category_requires_full_panel_unanimity", decision.Outcome, decision.Reason)
	}
	if decision.WeightedLean != "tie" {
		t.Fatalf("setup: WeightedLean = %q, want tie", decision.WeightedLean)
	}
	if decision.Accepts <= decision.Rejects {
		t.Fatalf("setup: raw counts must favor accept for this regression to be meaningful, got Accepts=%d Rejects=%d", decision.Accepts, decision.Rejects)
	}
	cluster := domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-2222222222222222", Category: domain.CategorySecurity, MemberIDs: []string{"F1"}, Finding: f, Decision: &decision}
	disputes, _ := domain.RankArbitration([]domain.Cluster{cluster}, 0)
	if len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d", len(disputes))
	}
	if strings.HasPrefix(disputes[0].Default, "accept") {
		t.Fatalf("regression: strict-category weighted-tie Default = %q, must never read as an accept majority", disputes[0].Default)
	}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, domain.Final{Arbitration: disputes})
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 1 || rulings[0].Outcome == "accepted" {
		t.Fatalf("regression: --accept-majority auto-accepted a strict-category weighted tie: %#v", rulings)
	}
}

// Third sibling of the same bypass class: a configured-unanimity decision
// (ConsensusOptions.Unanimous) that fails on unanimity_not_reached rather
// than vote_tie, with a raw-count/weighted-lean mismatch. WeightedLean is
// computed once before the outcome switch, so this is structurally covered
// by the fix already, but it is pinned here explicitly rather than left as
// an inference from the other two cases.
func TestConsensusPlaytest_UnanimityNotReachedWeightedMismatchNeverReadsAsMajority(t *testing.T) {
	f := find("F1", domain.CategoryCorrectness, domain.SeverityMajor, domain.EvidenceAnchored, "a", "q", 0)
	votes := []domain.Vote{
		vote("Senior", "F1", domain.VoteReject, domain.SeverityMajor),
		vote("J1", "F1", domain.VoteAccept, domain.SeverityMajor),
		vote("J2", "F1", domain.VoteAccept, domain.SeverityMajor),
	}
	opts := domain.ConsensusOptions{ConfiguredReviewers: 3, ValidReviewers: 3, Unanimous: true, Weighted: true, Weights: map[string]float64{"Senior": 2.0, "J1": 1.0, "J2": 1.0}}
	decision := domain.ResolveVotes(f, votes, opts)
	if decision.Outcome != "arbitration" || decision.Reason != "unanimity_not_reached" {
		t.Fatalf("setup: decision = %s/%s, want arbitration/unanimity_not_reached", decision.Outcome, decision.Reason)
	}
	if decision.WeightedLean != "tie" {
		t.Fatalf("setup: WeightedLean = %q, want tie", decision.WeightedLean)
	}
	if decision.Accepts <= decision.Rejects {
		t.Fatalf("setup: raw counts must favor accept for this regression to be meaningful, got Accepts=%d Rejects=%d", decision.Accepts, decision.Rejects)
	}
	cluster := domain.Cluster{SchemaVersion: domain.SchemaVersion, ID: "C-3333333333333333", Category: domain.CategoryCorrectness, MemberIDs: []string{"F1"}, Finding: f, Decision: &decision}
	disputes, _ := domain.RankArbitration([]domain.Cluster{cluster}, 0)
	if len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d", len(disputes))
	}
	if strings.HasPrefix(disputes[0].Default, "accept") {
		t.Fatalf("regression: unanimity-not-reached weighted-tie Default = %q, must never read as an accept majority", disputes[0].Default)
	}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, domain.Final{Arbitration: disputes})
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 1 || rulings[0].Outcome == "accepted" {
		t.Fatalf("regression: --accept-majority auto-accepted a configured-unanimity weighted tie: %#v", rulings)
	}
}
