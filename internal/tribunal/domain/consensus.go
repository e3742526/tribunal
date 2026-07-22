package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

// FindingFingerprint is identity-bearing: it keys the workspace ledger,
// explicit deferrals, and decision memory. 16 hex chars (64 bits) keeps the
// silent-collision probability negligible at realistic ledger sizes, where
// the previous 32-bit form reached ~1% at ten thousand findings.
func FindingFingerprint(f Finding) string {
	payload := strings.Join([]string{string(f.Category), f.Anchor.PacketItem, f.Anchor.Quote, f.Issue}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])[:16]
}

// ValidFingerprint reports whether value has the exact shape FindingFingerprint
// emits. Persisted stores must reject other shapes (notably the pre-v0.1.0
// 8-hex form) instead of silently never matching recomputed identities.
func ValidFingerprint(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func MatchesDecisionMemory(f Finding, packetItem, fingerprint string) bool {
	return f.Anchor.PacketItem == packetItem && FindingFingerprint(f) == fingerprint
}

func AnchorsOverlap(a, b Anchor) bool {
	if a.PacketItem != b.PacketItem || a.ItemSHA256 != b.ItemSHA256 {
		return false
	}
	if a.EndOffset > a.CharOffset && b.EndOffset > b.CharOffset {
		return a.CharOffset < b.EndOffset && b.CharOffset < a.EndOffset
	}
	return a.Quote != "" && b.Quote != "" && (strings.Contains(a.Quote, b.Quote) || strings.Contains(b.Quote, a.Quote))
}

func ClusterFindings(findings []Finding) []Cluster {
	ordered := append([]Finding(nil), findings...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Anchor.PacketItem != ordered[j].Anchor.PacketItem {
			return ordered[i].Anchor.PacketItem < ordered[j].Anchor.PacketItem
		}
		if ordered[i].Anchor.CharOffset != ordered[j].Anchor.CharOffset {
			return ordered[i].Anchor.CharOffset < ordered[j].Anchor.CharOffset
		}
		return ordered[i].ID < ordered[j].ID
	})
	var clusters []Cluster
	for _, finding := range ordered {
		if finding.Quarantined {
			continue
		}
		merged := false
		for i := range clusters {
			if clusters[i].Category == finding.Category && AnchorsOverlap(clusters[i].Anchor, finding.Anchor) {
				clusters[i].MemberIDs = append(clusters[i].MemberIDs, finding.ID)
				if finding.Severity.Rank() > clusters[i].Finding.Severity.Rank() {
					clusters[i].Finding.Severity = finding.Severity
				}
				merged = true
				break
			}
		}
		if !merged {
			id := FindingFingerprint(finding)
			clusters = append(clusters, Cluster{
				SchemaVersion: SchemaVersion,
				ID:            "C-" + id,
				Category:      finding.Category,
				MemberIDs:     []string{finding.ID},
				Anchor:        finding.Anchor,
				Finding:       finding,
			})
		}
	}
	return clusters
}

type ConsensusOptions struct {
	ConfiguredReviewers int
	ValidReviewers      int
	Unanimous           bool
	Weighted            bool
	Weights             map[string]float64
}

func ResolveVotes(f Finding, votes []Vote, opts ConsensusOptions) Decision {
	decision := Decision{
		SchemaVersion: SchemaVersion,
		FindingID:     f.ID,
		Configured:    opts.ConfiguredReviewers,
		Valid:         opts.ValidReviewers,
		Strict:        f.Category.Strict(),
	}
	var severityRanks []int
	// Weights accumulate as integer hundredths so tie detection is exact;
	// float sums would make "vote_tie" depend on rounding and vote order.
	acceptWeight, rejectWeight := 0, 0
	for _, vote := range votes {
		switch vote.Choice {
		case VoteAccept, VoteModify:
			decision.Accepts++
			acceptWeight += voteWeight(vote.ReviewerID, opts)
			severityRanks = append(severityRanks, vote.Severity.Rank())
		case VoteReject:
			decision.Rejects++
			rejectWeight += voteWeight(vote.ReviewerID, opts)
			severityRanks = append(severityRanks, vote.Severity.Rank())
		case VoteAbstain:
			decision.Abstains++
		}
	}
	if len(severityRanks) > 0 {
		sort.Ints(severityRanks)
		decision.Severity = SeverityFromRank(severityRanks[(len(severityRanks)-1)/2])
	} else {
		decision.Severity = f.Severity
	}
	if f.EvidenceStatus == EvidenceUnevidenced && decision.Severity.Rank() > SeverityMinor.Rank() {
		decision.Severity = SeverityMinor
	}
	nonAbstain := decision.Accepts + decision.Rejects
	switch {
	case opts.ValidReviewers < 2 || opts.ValidReviewers*2 <= opts.ConfiguredReviewers:
		decision.Outcome, decision.Reason = "degraded", "quorum_unmet"
	case nonAbstain < 2:
		decision.Outcome, decision.Reason = "arbitration", "insufficient_non_abstain_votes"
	case f.Category.Strict() && (opts.ValidReviewers != opts.ConfiguredReviewers || decision.Accepts != opts.ConfiguredReviewers):
		decision.Outcome, decision.Reason = "arbitration", "category_requires_full_panel_unanimity"
	case opts.Unanimous && decision.Accepts != nonAbstain:
		decision.Outcome, decision.Reason = "arbitration", "unanimity_not_reached"
	case acceptWeight == rejectWeight:
		decision.Outcome, decision.Reason = "arbitration", "vote_tie"
	case acceptWeight > rejectWeight:
		if f.Category == CategoryFactualClaim && f.EvidenceStatus == EvidenceUnevidenced {
			decision.Outcome, decision.Reason = "unverified-claim", "factual_claim_lacks_evidence"
		} else {
			decision.Outcome, decision.Reason = "accepted", "majority_accept"
		}
	default:
		decision.Outcome, decision.Reason = "rejected", "majority_reject"
	}
	for _, vote := range votes {
		severityDivergence := abs(vote.Severity.Rank()-decision.Severity.Rank()) >= 2
		rejectsAccepted := decision.Outcome == "accepted" && vote.Choice == VoteReject
		if severityDivergence || rejectsAccepted {
			decision.Dissent = append(decision.Dissent, Dissent{ReviewerID: vote.ReviewerID, Choice: vote.Choice, Severity: vote.Severity, Reason: vote.Reason})
		}
	}
	return decision
}

// voteWeight returns the reviewer's clamped weight in integer hundredths
// (quantized to 0.01) so consensus arithmetic stays exact.
func voteWeight(reviewer string, opts ConsensusOptions) int {
	if !opts.Weighted {
		return 100
	}
	weight := opts.Weights[reviewer]
	// The inverted comparison also catches NaN, which would otherwise pass
	// both clamps and produce an implementation-defined int conversion.
	if !(weight >= 0.5) {
		weight = 0.5
	}
	if weight > 2 {
		weight = 2
	}
	return int(math.Round(weight * 100))
}

func RankArbitration(clusters []Cluster, max int) ([]ArbitrationDispute, []string) {
	var disputes []ArbitrationDispute
	for _, cluster := range clusters {
		if cluster.Decision == nil || cluster.Decision.Outcome != "arbitration" {
			continue
		}
		forArg, against := "", ""
		for _, vote := range cluster.Votes {
			if forArg == "" && (vote.Choice == VoteAccept || vote.Choice == VoteModify) {
				forArg = vote.Reason
			}
			if against == "" && vote.Choice == VoteReject {
				against = vote.Reason
			}
		}
		disputes = append(disputes, ArbitrationDispute{SchemaVersion: SchemaVersion, ID: "A-" + strings.TrimPrefix(cluster.ID, "C-"), Finding: cluster.Finding, Decision: *cluster.Decision, ForArgument: forArg, Against: against, Default: defaultRecommendation(*cluster.Decision)})
	}
	sort.SliceStable(disputes, func(i, j int) bool {
		if disputes[i].Decision.Severity.Rank() != disputes[j].Decision.Severity.Rank() {
			return disputes[i].Decision.Severity.Rank() > disputes[j].Decision.Severity.Rank()
		}
		iGap := abs(disputes[i].Decision.Accepts - disputes[i].Decision.Rejects)
		jGap := abs(disputes[j].Decision.Accepts - disputes[j].Decision.Rejects)
		if iGap != jGap {
			return iGap < jGap
		}
		return disputes[i].ID < disputes[j].ID
	})
	if max <= 0 || len(disputes) <= max {
		return disputes, nil
	}
	overflow := make([]string, 0, len(disputes)-max)
	for _, dispute := range disputes[max:] {
		overflow = append(overflow, dispute.ID)
	}
	return disputes[:max], overflow
}

func defaultRecommendation(decision Decision) string {
	if decision.Accepts > decision.Rejects {
		return "accept majority"
	}
	if decision.Rejects > decision.Accepts {
		return "reject majority"
	}
	return "review both arguments"
}

func ValidateFinding(f Finding) error {
	if f.SchemaVersion != FindingSchemaVersion {
		return fmt.Errorf("finding schema_version must be %d", FindingSchemaVersion)
	}
	if f.ID == "" || f.Reviewer == "" || f.Origin == "" || f.Issue == "" || f.Recommendation == "" {
		return fmt.Errorf("finding requires id, reviewer, origin, issue, and recommendation")
	}
	if f.Severity.Rank() == 0 || !validCategory(f.Category) {
		return fmt.Errorf("finding %q has invalid severity %q", f.ID, f.Severity)
	}
	if f.Anchor.PacketItem == "" || f.Anchor.ItemSHA256 == "" || f.Anchor.Quote == "" {
		return fmt.Errorf("finding %q requires a quote anchor", f.ID)
	}
	if f.Confidence != "low" && f.Confidence != "med" && f.Confidence != "high" {
		return fmt.Errorf("finding %q has invalid confidence", f.ID)
	}
	switch f.EvidenceStatus {
	case EvidenceAnchored, EvidenceWorkerVerified, EvidenceUnevidenced:
	default:
		return fmt.Errorf("finding %q has invalid evidence status", f.ID)
	}
	if f.Quarantined && f.QuarantineWhy == "" {
		return fmt.Errorf("finding %q is quarantined without a reason", f.ID)
	}
	return nil
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
