package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type reviewStatusArtifact struct {
	SchemaVersion int                `json:"schema_version"`
	Status        domain.PanelStatus `json:"status"`
}

type voteStatusArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
}

type findingsArtifact struct {
	SchemaVersion int              `json:"schema_version"`
	Findings      []domain.Finding `json:"findings"`
}

type clustersArtifact struct {
	SchemaVersion int              `json:"schema_version"`
	Clusters      []domain.Cluster `json:"clusters"`
}

type parsedVotesArtifact struct {
	SchemaVersion int           `json:"schema_version"`
	Votes         []domain.Vote `json:"votes"`
}

type arbitrationArtifact struct {
	SchemaVersion int                         `json:"schema_version"`
	Disputes      []domain.ArbitrationDispute `json:"disputes"`
	Overflow      []string                    `json:"overflow"`
}

func readPacket(path string) (documents.Packet, error) {
	var packet documents.Packet
	if err := storage.ReadJSONStrict(path, &packet); err != nil {
		return documents.Packet{}, err
	}
	if err := documents.ValidatePacket(packet); err != nil {
		return documents.Packet{}, fmt.Errorf("validate persisted packet: %w", err)
	}
	return packet, nil
}

func readMeta(path string) (Meta, error) {
	var meta Meta
	if err := storage.ReadJSONStrict(path, &meta); err != nil {
		return Meta{}, err
	}
	if meta.SchemaVersion != 1 {
		return Meta{}, fmt.Errorf("unsupported meta schema_version %d", meta.SchemaVersion)
	}
	if meta.RunID == "" || meta.WorkspaceID == "" || meta.InputRoot == "" || meta.PacketHash == "" || meta.StartedAt.IsZero() {
		return Meta{}, fmt.Errorf("incomplete run metadata")
	}
	if err := domain.ValidatePanel(meta.Panel); err != nil {
		return Meta{}, fmt.Errorf("invalid persisted panel: %w", err)
	}
	return meta, nil
}

// resumeCheckpoint is a reducer over immutable call artifacts. It invokes only
// the first absent provider work and never replaces a completed call result.
// The caller (Resume) already holds run.lock and has revalidated the run
// directory; acquiring the lock again here would self-deadlock.
func (s *Service) resumeCheckpoint(ctx context.Context, workspace storage.Workspace, runDir, runID string, packet documents.Packet, meta Meta) (domain.Final, error) {
	runCtx, cancel := withRunTimeout(ctx, s.Config.Limits.RunTimeout)
	defer cancel()
	var state domain.RunState
	if err := storage.ReadJSONStrict(filepath.Join(runDir, "state.json"), &state); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "resume state: %v", err)
	}
	if err := domain.ValidateRunState(state); err != nil || state.RunID != runID || state.WorkspaceID != packet.WorkspaceID || state.PacketHash != packet.PacketHash {
		return domain.Final{}, exitError(ExitPreflight, "resume state identity mismatch")
	}
	budget, err := loadUsageBudget(runDir, s.Config.Limits.TokenBudget)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	runCtx = withUsageBudget(runCtx, budget)
	results, err := s.resumeReviews(runCtx, runDir, packet, meta.Panel)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "resume reviews: %v", err)
	}
	valid, allFindings, statuses, reasons := validatePass(packet, results)
	reasons = append(reasons, state.ReasonCodes...)
	workerFindings, err := s.resumeWorkerFindings(runDir, packet, meta.NoWorkers)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	allFindings = append(allFindings, workerFindings...)
	canonicalizeFindingIDs(allFindings)
	if len(valid)*2 <= len(meta.Panel.Reviewers) || len(valid) < 2 {
		final, persistErr := s.finalizeDegraded(runDir, workspace, runID, packet, meta.Panel, meta.StartedAt, statuses, allFindings, reasons)
		if persistErr != nil {
			return domain.Final{}, exitError(ExitPreflight, "%v", persistErr)
		}
		return finalResult(final)
	}
	verification, verificationReasons, err := s.resumeVerification(runCtx, runDir, allFindings, meta.NoWorkers)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	reasons = append(reasons, verificationReasons...)
	clusters, err := resumeClusters(runDir, allFindings)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	voteResults, err := s.resumeVotes(runCtx, runDir, packet, verification, valid, clusterFindings(clusters))
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	votesByFinding := map[string][]domain.Vote{}
	for _, result := range voteResults {
		if result.err != nil {
			reasons = append(reasons, "voter_unavailable")
			continue
		}
		for _, vote := range result.votes {
			votesByFinding[vote.FindingID] = append(votesByFinding[vote.FindingID], vote)
		}
	}
	weights := map[string]float64{}
	for _, reviewer := range meta.Panel.Reviewers {
		weights[reviewer.ID] = reviewer.Weight
	}
	var decisions []domain.Decision
	for i := range clusters {
		clusters[i].Votes = votesForCluster(clusters[i], votesByFinding)
		decision := domain.ResolveVotes(clusters[i].Finding, clusters[i].Votes, domain.ConsensusOptions{ConfiguredReviewers: len(meta.Panel.Reviewers), ValidReviewers: len(valid), Weighted: true, Weights: weights})
		clusters[i].Decision = &decision
		decisions = append(decisions, decision)
	}
	disputes, overflow := domain.RankArbitration(clusters, s.Config.Limits.MaxArbitration)
	memory, err := storage.ReadDecisions(workspace)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "read decision memory: %v", err)
	}
	for i := range disputes {
		for _, record := range memory {
			if domain.MatchesDecisionMemory(disputes[i].Finding, record.PacketItem, record.Fingerprint) {
				disputes[i].MemoryHint = "previous ruling: " + record.Ruling
				break
			}
		}
	}
	if len(overflow) > 0 {
		reasons = append(reasons, "arbitration_overflow")
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "votes.json"), clustersArtifact{SchemaVersion: 1, Clusters: clusters}); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "persist votes: %v", err)
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "arbitration.json"), arbitrationArtifact{SchemaVersion: 1, Disputes: disputes, Overflow: overflow}); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "persist arbitration: %v", err)
	}
	final := buildFinal(runID, packet, meta.StartedAt, s.now(), statuses, allFindings, verification.Evidence, decisions, disputes, reasons)
	if err := s.publish(runDir, workspace, final, meta.Panel); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	return finalResult(final)
}

func (s *Service) resumeReviews(ctx context.Context, runDir string, packet documents.Packet, panel domain.Panel) ([]panelResult, error) {
	results := make([]panelResult, len(panel.Reviewers))
	for i, reviewer := range panel.Reviewers {
		dir := filepath.Join(runDir, "calls", reviewer.ID, "review")
		var status reviewStatusArtifact
		statusErr := storage.ReadJSONStrict(filepath.Join(dir, "status.json"), &status)
		var review domain.Review
		parsedErr := storage.ReadJSONStrict(filepath.Join(dir, "parsed.json"), &review)
		if statusErr == nil {
			if status.SchemaVersion != 1 || status.Status.ReviewerID != reviewer.ID {
				return nil, fmt.Errorf("invalid persisted review status for %s", reviewer.ID)
			}
			results[i] = panelResult{panelist: reviewer, status: status.Status}
			if status.Status.Status != "ok" {
				results[i].err = fmt.Errorf("persisted review failure: %s", status.Status.Reason)
				continue
			}
			if parsedErr != nil {
				return nil, fmt.Errorf("completed review %s is missing parsed output", reviewer.ID)
			}
			if err := domain.ValidateReview(review); err != nil || review.ReviewerID != reviewer.ID {
				return nil, fmt.Errorf("invalid persisted review for %s", reviewer.ID)
			}
			results[i].review = review
			continue
		}
		if !os.IsNotExist(statusErr) {
			return nil, statusErr
		}
		if parsedErr == nil {
			if err := domain.ValidateReview(review); err != nil || review.ReviewerID != reviewer.ID {
				return nil, fmt.Errorf("invalid persisted review for %s", reviewer.ID)
			}
			ok := domain.PanelStatus{ReviewerID: reviewer.ID, Adapter: reviewer.Adapter, Model: reviewer.Model, Status: "ok"}
			if err := storage.WriteJSON(filepath.Join(dir, "status.json"), reviewStatusArtifact{SchemaVersion: 1, Status: ok}); err != nil {
				return nil, err
			}
			results[i] = panelResult{panelist: reviewer, review: review, status: ok}
			continue
		}
		if !os.IsNotExist(parsedErr) {
			return nil, parsedErr
		}
		result := s.invokeReview(ctx, runDir, packet, reviewer)
		if err := storage.WriteJSON(filepath.Join(dir, "status.json"), reviewStatusArtifact{SchemaVersion: 1, Status: result.status}); err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

func (s *Service) resumeWorkerFindings(runDir string, packet documents.Packet, disabled bool) ([]domain.Finding, error) {
	if disabled {
		return nil, nil
	}
	path := filepath.Join(runDir, "worker-findings.json")
	var artifact findingsArtifact
	if err := storage.ReadJSONStrict(path, &artifact); err == nil {
		if artifact.SchemaVersion != 1 {
			return nil, fmt.Errorf("unsupported worker findings schema")
		}
		for _, finding := range artifact.Findings {
			if err := domain.ValidateFinding(finding); err != nil {
				return nil, err
			}
		}
		return artifact.Findings, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	artifact = findingsArtifact{SchemaVersion: 1, Findings: append(adapters.Spellcheck(packet), adapters.ReferenceCheck(packet)...)}
	if len(artifact.Findings) > s.Config.Limits.MaxFindings {
		artifact.Findings = artifact.Findings[:s.Config.Limits.MaxFindings]
	}
	return artifact.Findings, storage.WriteJSON(path, artifact)
}

func (s *Service) resumeVerification(ctx context.Context, runDir string, findings []domain.Finding, disabled bool) (verificationArtifact, []string, error) {
	empty := verificationArtifact{SchemaVersion: 1, Evidence: []domain.EvidenceItem{}, VerificationHash: hashText("[]")}
	if disabled {
		return empty, nil, nil
	}
	var artifact verificationArtifact
	if err := storage.ReadJSONStrict(filepath.Join(runDir, "verification-evidence.json"), &artifact); err == nil {
		if artifact.SchemaVersion != 1 || artifact.VerificationHash != hashText(string(marshal(artifact.Evidence))) {
			return verificationArtifact{}, nil, fmt.Errorf("invalid verification artifact hash")
		}
		for _, evidence := range artifact.Evidence {
			if err := domain.ValidateEvidenceItem(evidence); err != nil {
				return verificationArtifact{}, nil, err
			}
		}
		return artifact, nil, nil
	} else if !os.IsNotExist(err) {
		return verificationArtifact{}, nil, err
	}
	return s.verifyFindings(ctx, runDir, findings)
}

func resumeClusters(runDir string, findings []domain.Finding) ([]domain.Cluster, error) {
	path := filepath.Join(runDir, "clusters.json")
	var artifact clustersArtifact
	if err := storage.ReadJSONStrict(path, &artifact); err == nil {
		if artifact.SchemaVersion != 1 {
			return nil, fmt.Errorf("unsupported clusters schema")
		}
		for _, cluster := range artifact.Clusters {
			if err := domain.ValidateCluster(cluster); err != nil {
				return nil, err
			}
		}
		return artifact.Clusters, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	artifact = clustersArtifact{SchemaVersion: 1, Clusters: domain.ClusterFindings(findings)}
	return artifact.Clusters, storage.WriteJSON(path, artifact)
}

func (s *Service) resumeVotes(ctx context.Context, runDir string, packet documents.Packet, verification verificationArtifact, voters []domain.Panelist, findings []domain.Finding) ([]panelResult, error) {
	results := make([]panelResult, len(voters))
	for i, voter := range voters {
		dir := filepath.Join(runDir, "calls", voter.ID, "vote")
		var status voteStatusArtifact
		statusErr := storage.ReadJSONStrict(filepath.Join(dir, "status.json"), &status)
		var parsed parsedVotesArtifact
		parsedErr := storage.ReadJSONStrict(filepath.Join(dir, "parsed.json"), &parsed)
		if statusErr == nil {
			if status.SchemaVersion != 1 {
				return nil, fmt.Errorf("invalid vote status for %s", voter.ID)
			}
			results[i].panelist = voter
			if status.Status != "ok" {
				results[i].err = fmt.Errorf("persisted vote failure: %s", status.Reason)
				continue
			}
			if parsedErr != nil {
				return nil, fmt.Errorf("completed vote %s is missing parsed output", voter.ID)
			}
		} else if !os.IsNotExist(statusErr) {
			return nil, statusErr
		} else if parsedErr != nil && !os.IsNotExist(parsedErr) {
			return nil, parsedErr
		} else if os.IsNotExist(parsedErr) {
			result := s.invokeVotes(ctx, runDir, packet, verification, voter, findings)
			statusValue := voteStatusArtifact{SchemaVersion: 1, Status: "ok"}
			if result.err != nil {
				statusValue.Status, statusValue.Reason = "vote_failed", result.err.Error()
			}
			if err := storage.WriteJSON(filepath.Join(dir, "status.json"), statusValue); err != nil {
				return nil, err
			}
			results[i] = result
			continue
		}
		if parsed.SchemaVersion != 1 {
			return nil, fmt.Errorf("invalid parsed votes schema")
		}
		for _, vote := range parsed.Votes {
			if err := domain.ValidateVote(vote); err != nil || vote.ReviewerID != voter.ID {
				return nil, fmt.Errorf("invalid persisted vote for %s", voter.ID)
			}
		}
		if statusErr != nil {
			if err := storage.WriteJSON(filepath.Join(dir, "status.json"), voteStatusArtifact{SchemaVersion: 1, Status: "ok"}); err != nil {
				return nil, err
			}
		}
		results[i] = panelResult{panelist: voter, votes: parsed.Votes}
	}
	return results, nil
}
