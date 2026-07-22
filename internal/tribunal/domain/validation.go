package domain

import (
	"fmt"
)

func validSeverity(value Severity) bool { return value.Rank() != 0 }

func validClusterID(id string) bool {
	if len(id) != 2+16 || id[:2] != "C-" {
		return false
	}
	for _, r := range id[2:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func validCategory(value Category) bool {
	switch value {
	case CategoryCorrectness, CategoryEvidence, CategoryCitationIntegrity,
		CategoryFactualClaim, CategorySecurity, CategoryDataLoss, CategoryIntegrity,
		CategoryStyle, CategoryScope, CategoryStructure:
		return true
	default:
		return false
	}
}

func ValidateEvidenceItem(item EvidenceItem) error {
	if item.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported evidence schema_version %d", item.SchemaVersion)
	}
	if item.ID == "" || item.Task == "" || item.Phase == "" || item.Source == "" || item.Status == "" {
		return fmt.Errorf("evidence identity, provenance, and status are required")
	}
	return nil
}

func ValidateReview(review Review) error {
	if review.SchemaVersion != SchemaVersion || review.ReviewerID == "" {
		return fmt.Errorf("invalid review schema or identity")
	}
	for _, finding := range review.Findings {
		if err := ValidateFinding(finding); err != nil {
			return err
		}
	}
	return nil
}

func ValidateVote(vote Vote) error {
	if vote.SchemaVersion != SchemaVersion || vote.ReviewerID == "" || vote.FindingID == "" || !validSeverity(vote.Severity) {
		return fmt.Errorf("invalid vote schema or identity")
	}
	switch vote.Choice {
	case VoteAccept, VoteReject, VoteModify, VoteAbstain:
	default:
		return fmt.Errorf("invalid vote choice %q", vote.Choice)
	}
	return nil
}

func ValidateDecision(decision Decision) error {
	if decision.SchemaVersion != SchemaVersion || decision.FindingID == "" || !validSeverity(decision.Severity) {
		return fmt.Errorf("invalid decision schema or identity")
	}
	switch decision.Outcome {
	case "accepted", "rejected", "arbitration", "deferred", "unverified-claim":
	default:
		return fmt.Errorf("invalid decision outcome %q", decision.Outcome)
	}
	switch decision.WeightedLean {
	case "accept", "reject", "tie":
	default:
		return fmt.Errorf("invalid decision weighted_lean %q", decision.WeightedLean)
	}
	return nil
}

func ValidateCluster(cluster Cluster) error {
	if cluster.SchemaVersion != SchemaVersion || !validCategory(cluster.Category) || len(cluster.MemberIDs) == 0 {
		return fmt.Errorf("invalid cluster schema or identity")
	}
	// Cluster IDs are derived ("C-" + finding fingerprint) and downstream
	// consumers slice the prefix off, so the shape is validated here instead
	// of trusting persisted state.
	if !validClusterID(cluster.ID) {
		return fmt.Errorf("invalid cluster id %q", cluster.ID)
	}
	if err := ValidateFinding(cluster.Finding); err != nil {
		return err
	}
	for _, vote := range cluster.Votes {
		if err := ValidateVote(vote); err != nil {
			return err
		}
	}
	if cluster.Decision != nil {
		return ValidateDecision(*cluster.Decision)
	}
	return nil
}

func ValidateRunState(state RunState) error {
	if state.SchemaVersion != SchemaVersion || state.RunID == "" || state.WorkspaceID == "" || state.Phase == "" || state.Status == "" {
		return fmt.Errorf("invalid run state schema or identity")
	}
	return nil
}

func ValidateDeliveryRecord(record DeliveryRecord) error {
	if record.SchemaVersion != SchemaVersion || record.InvocationID == "" || record.ReviewerID == "" ||
		record.Adapter == "" || record.Model == "" || record.Phase == "" || record.PacketHash == "" || record.RubricHash == "" {
		return fmt.Errorf("invalid delivery record schema or identity")
	}
	return nil
}

func ValidateFinal(final Final) error {
	if final.SchemaVersion != SchemaVersion || final.RunID == "" || final.WorkspaceID == "" || final.PacketHash == "" || final.Status == "" {
		return fmt.Errorf("invalid final schema or identity")
	}
	if final.ExitCode < 0 || final.ExitCode > 6 || final.StartedAt.IsZero() || final.FinishedAt.IsZero() || final.FinishedAt.Before(final.StartedAt) {
		return fmt.Errorf("invalid final outcome or timestamps")
	}
	for _, status := range final.PanelStatus {
		if status.ReviewerID == "" || status.Adapter == "" || status.Model == "" || status.Status == "" {
			return fmt.Errorf("invalid panel status")
		}
	}
	for _, finding := range final.Findings {
		if err := ValidateFinding(finding); err != nil {
			return err
		}
	}
	for _, evidence := range final.Evidence {
		if err := ValidateEvidenceItem(evidence); err != nil {
			return err
		}
	}
	for _, decision := range final.Decisions {
		if err := ValidateDecision(decision); err != nil {
			return err
		}
	}
	for _, dispute := range final.Arbitration {
		if dispute.SchemaVersion != SchemaVersion || dispute.ID == "" {
			return fmt.Errorf("invalid arbitration dispute")
		}
		if err := ValidateFinding(dispute.Finding); err != nil {
			return err
		}
		if err := ValidateDecision(dispute.Decision); err != nil {
			return err
		}
	}
	return nil
}
