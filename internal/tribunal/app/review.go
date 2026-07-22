package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type panelResult struct {
	panelist domain.Panelist
	review   domain.Review
	votes    []domain.Vote
	status   domain.PanelStatus
	repaired bool
	err      error
}

func (s *Service) Review(ctx context.Context, opts ReviewOptions) (domain.Final, error) {
	if opts.Input == "" && opts.Packet == nil {
		return domain.Final{}, exitError(ExitInvalidArguments, "review requires a file or folder")
	}
	runCtx, cancel := withRunTimeout(ctx, s.Config.Limits.RunTimeout)
	defer cancel()
	panel, err := s.resolvePanel(opts)
	if err != nil {
		return domain.Final{}, exitError(ExitInvalidArguments, "%v", err)
	}
	packet, err := s.resolvePacket(runCtx, opts)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if err := preflightContext(&packet, panel, opts.Split); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if err := preflightTokenBudget(packet, panel, s.Config.Limits.Passes, s.Config.Limits.TokenBudget); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	workspace, runID, runDir, err := s.resolveRun(opts, packet)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	lock, err := storage.AcquireLock(runCtx, filepath.Join(runDir, "run.lock"), nil)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "acquire run lock: %v", err)
	}
	defer lock.Close()
	if err := storage.ValidateRunDir(workspace, runDir); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "revalidate run directory: %v", err)
	}
	started := s.now()
	if err := s.persistStart(runDir, runID, packet, panel, opts, started); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	budget, err := loadUsageBudget(runDir, s.Config.Limits.TokenBudget)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	runCtx = withUsageBudget(runCtx, budget)
	if err := writeActive(workspace, runID, packet, "running", started); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if !opts.NoWorkers {
		workerFindings := append(adapters.Spellcheck(packet), adapters.ReferenceCheck(packet)...)
		if len(workerFindings) > s.Config.Limits.MaxFindings {
			workerFindings = workerFindings[:s.Config.Limits.MaxFindings]
		}
		if err := storage.WriteJSON(filepath.Join(runDir, "worker-findings.json"), map[string]any{"schema_version": 1, "findings": workerFindings}); err != nil {
			return domain.Final{}, exitError(ExitPreflight, "persist worker findings: %v", err)
		}
	}
	if err := s.transition(runDir, runID, packet, domain.PhaseReviewing, "running", nil); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	results := s.reviewPass(runCtx, runDir, packet, panel)
	valid, allFindings, statuses, reasons := validatePass(packet, results)
	// Worker findings merge and ID canonicalization precede every
	// finalization path so an aborted final still carries the already
	// persisted worker results and collision-free finding IDs.
	workerFindings, err := readWorkerFindings(runDir)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "read worker findings: %v", err)
	}
	allFindings = append(allFindings, workerFindings...)
	canonicalizeFindingIDs(allFindings)
	if runCtx.Err() != nil {
		final, persistErr := s.finalizeAborted(runDir, workspace, runID, packet, panel, started, statuses, allFindings, reasons, runCtx.Err())
		if persistErr != nil {
			return domain.Final{}, exitError(ExitPreflight, "%v", persistErr)
		}
		return final, exitError(ExitAborted, "%v", runCtx.Err())
	}
	if len(valid)*2 <= len(panel.Reviewers) || len(valid) < 2 {
		final, persistErr := s.finalizeDegraded(runDir, workspace, runID, packet, panel, started, statuses, allFindings, reasons)
		if persistErr != nil {
			return domain.Final{}, exitError(ExitPreflight, "%v", persistErr)
		}
		return final, exitError(ExitDegraded, "panel quorum unmet")
	}
	if err := s.transition(runDir, runID, packet, domain.PhaseReviewed, "running", reasons); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	verification := verificationArtifact{SchemaVersion: 1, Evidence: []domain.EvidenceItem{}, VerificationHash: hashText("[]")}
	if !opts.NoWorkers {
		if err := s.transition(runDir, runID, packet, domain.PhaseVerifying, "running", reasons); err != nil {
			return domain.Final{}, exitError(ExitPreflight, "%v", err)
		}
		var verificationReasons []string
		verification, verificationReasons, err = s.verifyFindings(runCtx, runDir, allFindings)
		if err != nil {
			return domain.Final{}, exitError(ExitPreflight, "persist verification: %v", err)
		}
		reasons = append(reasons, verificationReasons...)
		if runCtx.Err() != nil {
			final, persistErr := s.finalizeAborted(runDir, workspace, runID, packet, panel, started, statuses, allFindings, reasons, runCtx.Err())
			if persistErr != nil {
				return domain.Final{}, exitError(ExitPreflight, "%v", persistErr)
			}
			return final, exitError(ExitAborted, "%v", runCtx.Err())
		}
	}
	clusters := domain.ClusterFindings(allFindings)
	if err := storage.WriteJSON(filepath.Join(runDir, "clusters.json"), map[string]any{"schema_version": 1, "clusters": clusters}); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "persist clusters: %v", err)
	}
	if err := s.transition(runDir, runID, packet, domain.PhaseClustered, "running", reasons); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if err := s.transition(runDir, runID, packet, domain.PhaseVoting, "running", reasons); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	voteResults := s.votePass(runCtx, runDir, packet, verification, valid, clusterFindings(clusters))
	if runCtx.Err() != nil {
		final, persistErr := s.finalizeAborted(runDir, workspace, runID, packet, panel, started, statuses, allFindings, reasons, runCtx.Err())
		if persistErr != nil {
			return domain.Final{}, exitError(ExitPreflight, "%v", persistErr)
		}
		return final, exitError(ExitAborted, "%v", runCtx.Err())
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
	for _, reviewer := range panel.Reviewers {
		weights[reviewer.ID] = reviewer.Weight
	}
	var decisions []domain.Decision
	for i := range clusters {
		votes := votesForCluster(clusters[i], votesByFinding)
		clusters[i].Votes = votes
		decision := domain.ResolveVotes(clusters[i].Finding, votes, domain.ConsensusOptions{ConfiguredReviewers: len(panel.Reviewers), ValidReviewers: len(valid), Weighted: true, Weights: weights})
		clusters[i].Decision = &decision
		decisions = append(decisions, decision)
	}
	if err := s.transition(runDir, runID, packet, domain.PhaseConsensus, "running", reasons); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
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
	if err := storage.WriteJSON(filepath.Join(runDir, "votes.json"), map[string]any{"schema_version": 1, "clusters": clusters}); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "persist votes: %v", err)
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "arbitration.json"), map[string]any{"schema_version": 1, "disputes": disputes, "overflow": overflow}); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "persist arbitration: %v", err)
	}
	phase := domain.PhaseRecommended
	if len(disputes) > 0 {
		phase = domain.PhaseArbitrationPending
	}
	if err := s.transition(runDir, runID, packet, phase, "running", reasons); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	final := buildFinal(runID, packet, started, s.now(), statuses, allFindings, verification.Evidence, decisions, disputes, reasons)
	if err := s.publish(runDir, workspace, final, panel); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if final.ExitCode != 0 {
		return final, &ExitError{Code: final.ExitCode, Err: fmt.Errorf("%s", final.Status)}
	}
	return final, nil
}

func (s *Service) resolvePanel(opts ReviewOptions) (domain.Panel, error) {
	if opts.PanelValue != nil {
		panel := *opts.PanelValue
		if err := domain.NormalizePanel(&panel); err != nil {
			return domain.Panel{}, err
		}
		return s.hydratePanel(panel)
	}
	raw := opts.Panel
	if raw == "" {
		raw = s.Config.Panel
	}
	panel, err := domain.ParsePanel(raw)
	if err != nil {
		return domain.Panel{}, err
	}
	for i := range panel.Reviewers {
		panel.Reviewers[i].MaxContextTokens = s.Config.Limits.MaxContextTokens
		panel.Reviewers[i].ReservedOutputTokens = s.Config.Limits.ReservedOutput
	}
	if err := domain.NormalizePanel(&panel); err != nil {
		return domain.Panel{}, err
	}
	return s.hydratePanel(panel)
}

func (s *Service) hydratePanel(panel domain.Panel) (domain.Panel, error) {
	for i := range panel.Reviewers {
		if panel.Reviewers[i].Persona != "" && panel.Reviewers[i].Persona != "plain" {
			persona, err := config.ResolvePersona(panel.Reviewers[i].Persona, s.Config.WorkspaceRoot, s.Config.TrustWorkspace)
			if err != nil {
				return domain.Panel{}, err
			}
			panel.Reviewers[i].PersonaLens = config.PersonaText(persona)
		}
	}
	return panel, nil
}

func (s *Service) resolvePacket(ctx context.Context, opts ReviewOptions) (documents.Packet, error) {
	if opts.Packet != nil {
		if err := documents.ValidatePacket(*opts.Packet); err != nil {
			return documents.Packet{}, fmt.Errorf("invalid replay packet: %w", err)
		}
		return *opts.Packet, nil
	}
	kind := opts.Kind
	if kind == "" {
		kind = s.Config.Kind
	}
	rubric, ok := config.BuiltinRubric(kind)
	if !ok {
		return documents.Packet{}, fmt.Errorf("unknown document kind %q", kind)
	}
	return documents.Build(ctx, opts.Input, documents.BuildOptions{Kind: kind, Rubric: rubric, FailOnSecret: opts.FailOnSecret, KnownSecrets: opts.KnownSecrets, MaxExtractedByte: 16 << 20})
}

func preflightContext(packet *documents.Packet, panel domain.Panel, split bool) error {
	available := int(^uint(0) >> 1)
	for _, reviewer := range panel.Reviewers {
		budget := reviewer.MaxContextTokens - reviewer.ReservedOutputTokens
		if budget < available {
			available = budget
		}
	}
	bytesBudget := available * 3
	total := 0
	for _, item := range packet.Items {
		total += len(item.Content)
	}
	if total <= bytesBudget {
		return nil
	}
	if !split {
		return fmt.Errorf("packet estimate exceeds panel context; rerun with --split")
	}
	return documents.Split(packet, bytesBudget)
}

func preflightTokenBudget(packet documents.Packet, panel domain.Panel, passes, budget int) error {
	if budget <= 0 {
		return fmt.Errorf("token budget must be positive")
	}
	bytes := 0
	for _, item := range packet.Items {
		bytes += len(item.Content)
	}
	estimate := ((bytes + 2) / 3) * len(panel.Reviewers) * max(1, passes)
	if estimate > budget {
		return fmt.Errorf("estimated %d tokens exceeds run budget %d", estimate, budget)
	}
	return nil
}

func (s *Service) resolveRun(opts ReviewOptions, packet documents.Packet) (storage.Workspace, string, string, error) {
	var workspace storage.Workspace
	if opts.Workspace != nil {
		workspace = *opts.Workspace
	} else {
		root, err := documentRoot(packet.InputRoot)
		if err != nil {
			return storage.Workspace{}, "", "", err
		}
		workspace, err = s.Store.Workspace(packet.WorkspaceID, root)
		if err != nil {
			return storage.Workspace{}, "", "", err
		}
	}
	if opts.RunDir != "" && opts.RunID != "" {
		return workspace, opts.RunID, opts.RunDir, nil
	}
	runID, runDir, err := s.Store.CreateRun(workspace)
	return workspace, runID, runDir, err
}

func (s *Service) persistStart(runDir, runID string, packet documents.Packet, panel domain.Panel, opts ReviewOptions, started time.Time) error {
	if err := storage.WriteJSON(filepath.Join(runDir, "packet.json"), packet); err != nil {
		return fmt.Errorf("persist packet: %w", err)
	}
	meta := Meta{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, InputRoot: packet.InputRoot, PacketHash: packet.PacketHash, Panel: panel, DiversityNote: domain.DiversityNote(panel), TrustedSources: s.Config.TrustedSources, IgnoredSources: s.Config.IgnoredSources, ReplayOf: opts.ReplayOf, NoWorkers: opts.NoWorkers, StartedAt: started}
	if err := storage.WriteJSON(filepath.Join(runDir, "meta.json"), meta); err != nil {
		return fmt.Errorf("persist meta: %w", err)
	}
	manifestItems := make([]map[string]any, 0, len(packet.Items))
	for index, item := range packet.Items {
		snapshot := filepath.Join("redacted-snapshot", fmt.Sprintf("%04d.txt", index+1))
		if err := storage.WriteFile(filepath.Join(runDir, snapshot), []byte(item.Content)); err != nil {
			return fmt.Errorf("persist redacted snapshot: %w", err)
		}
		manifestItems = append(manifestItems, map[string]any{"id": item.ID, "logical_path": item.LogicalPath, "media_type": item.MediaType, "source_sha256": item.SourceSHA256, "packet_sha256": item.PacketSHA256, "snapshot": snapshot, "editable": item.Editable})
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "packet-manifest.json"), map[string]any{"schema_version": 1, "packet_hash": packet.PacketHash, "items": manifestItems, "redactions": packet.Redactions, "chunks": packet.Chunks}); err != nil {
		return fmt.Errorf("persist packet manifest: %w", err)
	}
	if err := storage.WriteFile(filepath.Join(runDir, "review.schema.json"), []byte(adapters.ProviderReviewSchema+"\n")); err != nil {
		return err
	}
	if err := storage.WriteFile(filepath.Join(runDir, "vote.schema.json"), []byte(adapters.ProviderVoteSchema+"\n")); err != nil {
		return err
	}
	return s.transition(runDir, runID, packet, domain.PhasePacketBuilt, "running", nil)
}

func (s *Service) transition(runDir, runID string, packet documents.Packet, phase domain.RunPhase, status string, reasons []string) error {
	state := domain.RunState{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, PacketHash: packet.PacketHash, Phase: phase, Status: status, ReasonCodes: unique(reasons), UpdatedAt: s.now()}
	return storage.Transition(runDir, state)
}

func (s *Service) reviewPass(ctx context.Context, runDir string, packet documents.Packet, panel domain.Panel) []panelResult {
	results := make([]panelResult, len(panel.Reviewers))
	var wait sync.WaitGroup
	for index, panelist := range panel.Reviewers {
		wait.Add(1)
		go func(index int, panelist domain.Panelist) {
			defer wait.Done()
			results[index] = s.invokeReview(ctx, runDir, packet, panelist)
		}(index, panelist)
	}
	wait.Wait() // strict pass-1 barrier
	for i := range results {
		path := filepath.Join(runDir, "calls", results[i].panelist.ID, "review", "status.json")
		if err := storage.WriteJSON(path, map[string]any{"schema_version": 1, "status": results[i].status}); err != nil {
			results[i].err = err
			results[i].status.Status, results[i].status.Reason = "persistence_failed", err.Error()
		}
	}
	return results
}

func (s *Service) invokeReview(ctx context.Context, runDir string, packet documents.Packet, panelist domain.Panelist) panelResult {
	result := panelResult{panelist: panelist, status: domain.PanelStatus{ReviewerID: panelist.ID, Adapter: panelist.Adapter, Model: panelist.Model}}
	adapter, err := s.Registry.Get(panelist.Adapter)
	if err != nil {
		result.err, result.status.Status, result.status.Reason = err, "invocation_failed", err.Error()
		return result
	}
	invocationDir := filepath.Join(runDir, "calls", panelist.ID, "review")
	if err := os.MkdirAll(invocationDir, 0o700); err != nil {
		result.err = err
		return result
	}
	prompt := reviewPrompt(packet, panelist)
	if err := storage.WriteFile(filepath.Join(invocationDir, "prompt.txt"), []byte(prompt)); err != nil {
		result.err, result.status.Status, result.status.Reason = err, "persistence_failed", err.Error()
		return result
	}
	delivery := domain.DeliveryRecord{SchemaVersion: 1, InvocationID: panelist.ID + "-review", ReviewerID: panelist.ID, Adapter: panelist.Adapter, Model: panelist.Model, Phase: "review", PacketHash: packet.PacketHash, RubricHash: packet.RubricHash, DeliveredAt: s.now()}
	for _, item := range packet.Items {
		delivery.Items = append(delivery.Items, item.ID)
	}
	for _, chunk := range packet.Chunks {
		delivery.Chunks = append(delivery.Chunks, chunk.ID)
	}
	if err := storage.WriteJSON(filepath.Join(invocationDir, "delivery.json"), delivery); err != nil {
		result.err, result.status.Status, result.status.Reason = err, "persistence_failed", err.Error()
		return result
	}
	req := adapters.Request{RunDir: invocationDir, SystemPrompt: reviewerSystem, Prompt: prompt, Schema: adapters.ProviderReviewSchema, SchemaPath: filepath.Join(runDir, "review.schema.json"), OutputPath: filepath.Join(invocationDir, "output.json"), MaxOutputBytes: s.Config.Limits.MaxOutputBytes, TimeoutSeconds: int(s.Config.Limits.CallTimeout.Seconds()), EnvSecrets: trustedSecrets(s.Config)}
	var response adapters.Response
	for attempt := 1; attempt <= 2; attempt++ {
		response, err = s.invokeWithProviderLock(ctx, runDir, adapter, adapters.RoleReviewer, panelist, req)
		if err == nil {
			break
		}
		_ = storage.WriteFile(filepath.Join(invocationDir, fmt.Sprintf("invocation-error-%d.txt", attempt)), []byte(err.Error()))
	}
	if err != nil {
		result.err, result.status.Status, result.status.Reason = err, "invocation_failed", err.Error()
		return result
	}
	if err := storage.WriteFile(filepath.Join(invocationDir, "raw.json"), response.Raw); err != nil {
		result.err, result.status.Status, result.status.Reason = err, "persistence_failed", err.Error()
		return result
	}
	review, repaired, decodeErr := adapters.DecodeReview(response.Raw, panelist.ID)
	if decodeErr != nil {
		retryReq := req
		retryReq.Prompt += "\n\nYour prior output failed validation: " + decodeErr.Error() + ". Return one valid JSON object only."
		_ = storage.WriteFile(filepath.Join(invocationDir, "retry-prompt.txt"), []byte(retryReq.Prompt))
		response, err = s.invokeWithProviderLock(ctx, runDir, adapter, adapters.RoleReviewer, panelist, retryReq)
		if err == nil {
			_ = storage.WriteFile(filepath.Join(invocationDir, "retry-raw.json"), response.Raw)
			review, repaired, decodeErr = adapters.DecodeReview(response.Raw, panelist.ID)
		}
	}
	if err != nil || decodeErr != nil {
		if decodeErr != nil {
			err = decodeErr
		}
		result.err, result.status.Status, result.status.Reason = err, "invalid_output", err.Error()
		return result
	}
	if len(review.Findings) > s.Config.Limits.MaxFindings {
		sort.SliceStable(review.Findings, func(i, j int) bool { return review.Findings[i].Severity.Rank() > review.Findings[j].Severity.Rank() })
		review.Findings = review.Findings[:s.Config.Limits.MaxFindings]
	}
	if err := storage.WriteJSON(filepath.Join(invocationDir, "parsed.json"), review); err != nil {
		result.err, result.status.Status, result.status.Reason = err, "invalid_output", err.Error()
		return result
	}
	result.review, result.repaired = review, repaired
	result.status.Status = "ok"
	return result
}

func validatePass(packet documents.Packet, results []panelResult) ([]domain.Panelist, []domain.Finding, []domain.PanelStatus, []string) {
	var valid []domain.Panelist
	var findings []domain.Finding
	var statuses []domain.PanelStatus
	var reasons []string
	for _, result := range results {
		statuses = append(statuses, result.status)
		if result.err != nil {
			reasons = append(reasons, "reviewer_unavailable")
			continue
		}
		valid = append(valid, result.panelist)
		if result.repaired {
			reasons = append(reasons, "json_repair_used")
		}
		for _, finding := range result.review.Findings {
			// Host-side trust boundary: a model may not self-declare
			// worker-verified evidence. This must hold even when the
			// verification phase is skipped (--no-workers, degraded runs).
			finding.EvidenceStatus = trustedEvidenceStatus(finding.EvidenceStatus)
			if err := documents.ResolveAnchor(packet, &finding.Anchor); err != nil {
				finding.Quarantined = true
				finding.QuarantineWhy = err.Error()
				finding.EvidenceStatus = domain.EvidenceUnevidenced
			} else {
				documents.MarkRedactedOverlap(packet, &finding)
			}
			findings = append(findings, finding)
		}
	}
	return valid, findings, statuses, unique(reasons)
}

func readWorkerFindings(runDir string) ([]domain.Finding, error) {
	var value findingsArtifact
	if err := storage.ReadJSONStrict(filepath.Join(runDir, "worker-findings.json"), &value); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if value.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported worker findings schema")
	}
	for _, finding := range value.Findings {
		if err := domain.ValidateFinding(finding); err != nil {
			return nil, err
		}
	}
	return value.Findings, nil
}

func clusterFindings(clusters []domain.Cluster) []domain.Finding {
	findings := make([]domain.Finding, 0, len(clusters))
	for _, cluster := range clusters {
		findings = append(findings, cluster.Finding)
	}
	return findings
}

func (s *Service) votePass(ctx context.Context, runDir string, packet documents.Packet, verification verificationArtifact, voters []domain.Panelist, findings []domain.Finding) []panelResult {
	results := make([]panelResult, len(voters))
	var wait sync.WaitGroup
	for index, voter := range voters {
		wait.Add(1)
		go func(index int, voter domain.Panelist) {
			defer wait.Done()
			results[index] = s.invokeVotes(ctx, runDir, packet, verification, voter, findings)
		}(index, voter)
	}
	wait.Wait()
	for i := range results {
		status := "ok"
		reason := ""
		if results[i].err != nil {
			status, reason = "vote_failed", results[i].err.Error()
		}
		path := filepath.Join(runDir, "calls", results[i].panelist.ID, "vote", "status.json")
		if err := storage.WriteJSON(path, map[string]any{"schema_version": 1, "status": status, "reason": reason}); err != nil {
			results[i].err = err
		}
	}
	return results
}

func (s *Service) invokeVotes(ctx context.Context, runDir string, packet documents.Packet, verification verificationArtifact, voter domain.Panelist, findings []domain.Finding) panelResult {
	result := panelResult{panelist: voter}
	adapter, err := s.Registry.Get(voter.Adapter)
	if err != nil {
		result.err = err
		return result
	}
	dir := filepath.Join(runDir, "calls", voter.ID, "vote")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		result.err = err
		return result
	}
	blind, blindToOriginal := blindFindings(findings, packet.PacketHash)
	ballot := buildBlindVotePacket(packet, verification, blind)
	prompt := votePrompt(voter, ballot)
	if err := storage.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		result.err = err
		return result
	}
	delivery := domain.DeliveryRecord{SchemaVersion: 1, InvocationID: voter.ID + "-vote", ReviewerID: voter.ID, Adapter: voter.Adapter, Model: voter.Model, Phase: "vote", PacketHash: packet.PacketHash, RubricHash: packet.RubricHash, VerificationHash: verification.VerificationHash, DeliveredAt: s.now()}
	for _, item := range packet.Items {
		delivery.Items = append(delivery.Items, item.ID)
	}
	for _, chunk := range packet.Chunks {
		delivery.Chunks = append(delivery.Chunks, chunk.ID)
	}
	for _, evidence := range append(append([]domain.EvidenceItem{}, packet.Evidence...), verification.Evidence...) {
		delivery.EvidenceIDs = append(delivery.EvidenceIDs, evidence.ID)
	}
	for _, finding := range blind {
		delivery.FindingIDs = append(delivery.FindingIDs, finding.ID)
	}
	if err := storage.WriteJSON(filepath.Join(dir, "delivery.json"), delivery); err != nil {
		result.err = err
		return result
	}
	if err := storage.WriteJSON(filepath.Join(dir, "blind-vote-packet.json"), ballot); err != nil {
		result.err = err
		return result
	}
	req := adapters.Request{RunDir: dir, SystemPrompt: voterSystem, Prompt: prompt, Schema: adapters.ProviderVoteSchema, SchemaPath: filepath.Join(runDir, "vote.schema.json"), OutputPath: filepath.Join(dir, "output.json"), MaxOutputBytes: s.Config.Limits.MaxOutputBytes, TimeoutSeconds: int(s.Config.Limits.CallTimeout.Seconds()), EnvSecrets: trustedSecrets(s.Config)}
	response, err := s.invokeWithProviderLock(ctx, runDir, adapter, adapters.RoleVoter, voter, req)
	if err != nil {
		result.err = err
		return result
	}
	if err := storage.WriteFile(filepath.Join(dir, "raw.json"), response.Raw); err != nil {
		result.err = err
		return result
	}
	votes, repaired, err := adapters.DecodeVotes(response.Raw, voter.ID)
	if err != nil {
		req.Prompt += "\n\nPrior vote output failed validation: " + err.Error()
		response, invokeErr := s.invokeWithProviderLock(ctx, runDir, adapter, adapters.RoleVoter, voter, req)
		if invokeErr == nil {
			if writeErr := storage.WriteFile(filepath.Join(dir, "retry-raw.json"), response.Raw); writeErr != nil {
				result.err = writeErr
				return result
			}
			votes, repaired, err = adapters.DecodeVotes(response.Raw, voter.ID)
		}
	}
	if err != nil {
		result.err = err
		return result
	}
	for i := range votes {
		original, ok := blindToOriginal[votes[i].FindingID]
		if !ok {
			result.err = fmt.Errorf("vote references unknown finding %q", votes[i].FindingID)
			return result
		}
		votes[i].FindingID = original
	}
	result.votes, result.repaired = votes, repaired
	if err := storage.WriteJSON(filepath.Join(dir, "parsed.json"), map[string]any{"schema_version": 1, "votes": votes}); err != nil {
		result.err = err
		return result
	}
	return result
}

func votesForCluster(cluster domain.Cluster, votes map[string][]domain.Vote) []domain.Vote {
	var result []domain.Vote
	for _, memberID := range cluster.MemberIDs {
		for _, vote := range votes[memberID] {
			vote.FindingID = cluster.Finding.ID
			result = append(result, vote)
		}
	}
	if len(result) == 0 {
		result = votes[cluster.Finding.ID]
	}
	return result
}

func (s *Service) publish(runDir string, workspace storage.Workspace, final domain.Final, panel domain.Panel) error {
	lock, err := storage.AcquireLock(context.Background(), filepath.Join(workspace.Root, "publish.lock"), nil)
	if err != nil {
		return fmt.Errorf("acquire publication lock: %w", err)
	}
	defer lock.Close()
	marker := publicationMarker{SchemaVersion: 1, RunID: final.RunID, PacketHash: final.PacketHash, Status: "pending", UpdatedAt: s.now()}
	if err := storage.WriteJSON(filepath.Join(runDir, "publication.json"), marker); err != nil {
		return fmt.Errorf("persist publication intent: %w", err)
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "final-candidate.json"), final); err != nil {
		return fmt.Errorf("persist final candidate: %w", err)
	}
	if err := writeReports(runDir, final, panel); err != nil {
		return err
	}
	if err := storage.PersistTerminal(runDir, final); err != nil {
		return err
	}
	// Workspace ledgers and pointers are projections. They are updated only
	// after the run's report, state, and final artifact have committed.
	if err := storage.UpdateLedger(workspace, final.RunID, final.Findings, final.Decisions); err != nil {
		return fmt.Errorf("persist findings ledger: %w", err)
	}
	latest := map[string]any{"schema_version": 1, "run_id": final.RunID, "status": final.Status, "updated_at": final.FinishedAt}
	if err := storage.WriteJSON(filepath.Join(workspace.Root, "latest.json"), latest); err != nil {
		return err
	}
	if err := writeActive(workspace, final.RunID, documents.Packet{WorkspaceID: final.WorkspaceID, PacketHash: final.PacketHash}, final.Status, final.FinishedAt); err != nil {
		return err
	}
	marker.Status, marker.UpdatedAt = "committed", s.now()
	return storage.WriteJSON(filepath.Join(runDir, "publication.json"), marker)
}

type publicationMarker struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	PacketHash    string    `json:"packet_hash"`
	Status        string    `json:"status"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func writeActive(workspace storage.Workspace, runID string, packet documents.Packet, status string, at time.Time) error {
	return storage.WriteJSON(filepath.Join(workspace.Root, "active.json"), map[string]any{"schema_version": 1, "run_id": runID, "workspace_id": packet.WorkspaceID, "packet_hash": packet.PacketHash, "status": status, "updated_at": at})
}

func summaryFor(decisions []domain.Decision, disputes []domain.ArbitrationDispute) string {
	accepted := 0
	for _, decision := range decisions {
		if decision.Outcome == "accepted" {
			accepted++
		}
	}
	return fmt.Sprintf("%d accepted recommendations; %d disputes require arbitration.", accepted, len(disputes))
}

func panelIncomplete(statuses []domain.PanelStatus) bool {
	for _, status := range statuses {
		if status.Status != "ok" {
			return true
		}
	}
	return false
}

func degradedReason(statuses []domain.PanelStatus) string {
	invocation, contract := false, false
	for _, status := range statuses {
		invocation = invocation || status.Status == "invocation_failed"
		contract = contract || status.Status == "invalid_output"
	}
	if invocation && contract {
		return "mixed"
	}
	if invocation {
		return "adapter_invocation_failure"
	}
	if contract {
		return "adapter_contract_failure"
	}
	return "quorum_unmet"
}

func trustedSecrets(cfg config.Config) map[string]string {
	values := map[string]string{}
	if key := cfg.OpenAICompatible.APIKeyEnv; key != "" {
		values[key] = os.Getenv(key)
	}
	return values
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func canonicalizeFindingIDs(findings []domain.Finding) {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[finding.ID]++
	}
	for i := range findings {
		if counts[findings[i].ID] > 1 {
			findings[i].ID = findings[i].Reviewer + "-" + findings[i].ID
		}
	}
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func marshal(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func stringJoin(values []string) string { return strings.Join(values, ",") }
