package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func FindingFingerprint(f Finding) string {
	payload := strings.Join([]string{string(f.Category), f.Anchor.PacketItem, f.Anchor.Quote, f.Issue}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])[:8]
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
	acceptWeight, rejectWeight := 0.0, 0.0
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

func voteWeight(reviewer string, opts ConsensusOptions) float64 {
	if !opts.Weighted {
		return 1
	}
	weight := opts.Weights[reviewer]
	if weight < 0.5 {
		return 0.5
	}
	if weight > 2 {
		return 2
	}
	return weight
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
		disputes = append(disputes, ArbitrationDispute{SchemaVersion: SchemaVersion, ID: "A-" + cluster.ID[2:], Finding: cluster.Finding, Decision: *cluster.Decision, ForArgument: forArg, Against: against, Default: defaultRecommendation(*cluster.Decision)})
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
	if f.ID == "" || f.Reviewer == "" || f.Issue == "" || f.Recommendation == "" {
		return fmt.Errorf("finding requires id, reviewer, issue, and recommendation")
	}
	if f.Severity.Rank() == 0 {
		return fmt.Errorf("finding %q has invalid severity %q", f.ID, f.Severity)
	}
	if f.Anchor.PacketItem == "" || f.Anchor.ItemSHA256 == "" || f.Anchor.Quote == "" {
		return fmt.Errorf("finding %q requires a quote anchor", f.ID)
	}
	if f.Confidence != "low" && f.Confidence != "med" && f.Confidence != "high" {
		return fmt.Errorf("finding %q has invalid confidence", f.ID)
	}
	return nil
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
