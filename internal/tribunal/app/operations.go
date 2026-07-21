package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type RunRef struct {
	Input string
	RunID string
}

type Transcript struct {
	SchemaVersion int                     `json:"schema_version"`
	RunID         string                  `json:"run_id"`
	Events        []storage.StateEvent    `json:"events"`
	Deliveries    []domain.DeliveryRecord `json:"deliveries"`
}

type Explanation struct {
	SchemaVersion int              `json:"schema_version"`
	RunID         string           `json:"run_id"`
	Finding       domain.Finding   `json:"finding"`
	Decision      *domain.Decision `json:"decision,omitempty"`
}

type ArbitrationRuling struct {
	ID       string `json:"id"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason"`
	Operator string `json:"operator"`
}

type ArbitrationFile struct {
	SchemaVersion int                 `json:"schema_version"`
	RunID         string              `json:"run_id"`
	Rulings       []ArbitrationRuling `json:"rulings"`
}

type ArbitrationOptions struct {
	RunRef
	DecisionsPath  string
	Rulings        []ArbitrationRuling
	AcceptMajority bool
	Except         []string
	Operator       string
}

const arbitrationFileSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["schema_version","run_id","rulings"],"properties":{"schema_version":{"const":1},"run_id":{"type":"string"},"rulings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","outcome","reason","operator"],"properties":{"id":{"type":"string"},"outcome":{"enum":["accepted","rejected","deferred"]},"reason":{"type":"string","minLength":1},"operator":{"type":"string","minLength":1}}}}}}`

type DoctorReport struct {
	SchemaVersion int                    `json:"schema_version"`
	StateRoot     string                 `json:"state_root"`
	Adapters      []adapters.VersionInfo `json:"adapters"`
	PDFToText     adapters.VersionInfo   `json:"pdftotext"`
}

func (s *Service) Status(ref RunRef) (storage.Snapshot, error) {
	_, runDir, _, err := s.locateRun(ref)
	if err != nil {
		return storage.Snapshot{}, exitError(ExitPreflight, "%v", err)
	}
	snapshot, err := storage.BuildSnapshot(runDir)
	if err != nil {
		return storage.Snapshot{}, exitError(ExitPreflight, "load status: %v", err)
	}
	return snapshot, nil
}

func (s *Service) Transcript(ref RunRef) (Transcript, error) {
	_, runDir, runID, err := s.locateRun(ref)
	if err != nil {
		return Transcript{}, exitError(ExitPreflight, "%v", err)
	}
	result := Transcript{SchemaVersion: 1, RunID: runID}
	file, err := os.Open(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		return Transcript{}, exitError(ExitPreflight, "open transcript: %v", err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event storage.StateEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.SchemaVersion != 1 {
			file.Close()
			return Transcript{}, exitError(ExitPreflight, "invalid transcript event")
		}
		result.Events = append(result.Events, event)
	}
	closeErr := file.Close()
	if err := firstError(scanner.Err(), closeErr); err != nil {
		return Transcript{}, exitError(ExitPreflight, "read transcript: %v", err)
	}
	deliveryPaths, _ := filepath.Glob(filepath.Join(runDir, "calls", "*", "*", "delivery.json"))
	sort.Strings(deliveryPaths)
	for _, path := range deliveryPaths {
		var delivery domain.DeliveryRecord
		if err := storage.ReadJSON(path, &delivery); err != nil || delivery.SchemaVersion != 1 {
			return Transcript{}, exitError(ExitPreflight, "invalid delivery record %s", path)
		}
		result.Deliveries = append(result.Deliveries, delivery)
	}
	return result, nil
}

func (s *Service) Explain(ref RunRef, findingID string) (Explanation, error) {
	_, runDir, runID, err := s.locateRun(ref)
	if err != nil {
		return Explanation{}, exitError(ExitPreflight, "%v", err)
	}
	final, err := readFinal(filepath.Join(runDir, "final.json"))
	if err != nil {
		return Explanation{}, exitError(ExitPreflight, "%v", err)
	}
	for _, finding := range final.Findings {
		if finding.ID != findingID {
			continue
		}
		result := Explanation{SchemaVersion: 1, RunID: runID, Finding: finding}
		for i := range final.Decisions {
			if final.Decisions[i].FindingID == findingID {
				result.Decision = &final.Decisions[i]
			}
		}
		return result, nil
	}
	return Explanation{}, exitError(ExitInvalidArguments, "finding %q not found", findingID)
}

func (s *Service) Resume(ctx context.Context, ref RunRef) (domain.Final, error) {
	workspace, runDir, runID, err := s.locateRun(ref)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if final, err := readFinal(filepath.Join(runDir, "final.json")); err == nil {
		if final.ExitCode == 0 {
			return final, nil
		}
		return final, &ExitError{Code: final.ExitCode, Err: fmt.Errorf("%s", final.Status)}
	}
	packet, err := readPacket(filepath.Join(runDir, "packet.json"))
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "resume packet: %v", err)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil || meta.PacketHash != packet.PacketHash {
		return domain.Final{}, exitError(ExitPreflight, "resume metadata does not match packet")
	}
	return s.Review(ctx, ReviewOptions{Packet: &packet, PanelValue: &meta.Panel, RunID: runID, RunDir: runDir, Workspace: &workspace, ReplayOf: meta.ReplayOf})
}

func (s *Service) Replay(ctx context.Context, ref RunRef) (domain.Final, error) {
	workspace, runDir, runID, err := s.locateRun(ref)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	packet, err := readPacket(filepath.Join(runDir, "packet.json"))
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "replay packet: %v", err)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil || meta.PacketHash != packet.PacketHash {
		return domain.Final{}, exitError(ExitPreflight, "replay metadata does not match packet")
	}
	return s.Review(ctx, ReviewOptions{Packet: &packet, PanelValue: &meta.Panel, Workspace: &workspace, ReplayOf: runID})
}

func (s *Service) Findings(ref RunRef) (storage.FindingsLedger, error) {
	workspace, err := s.locateWorkspace(ref.Input)
	if err != nil {
		return storage.FindingsLedger{}, exitError(ExitPreflight, "%v", err)
	}
	ledger, err := storage.LoadLedger(workspace)
	if err != nil {
		return storage.FindingsLedger{}, exitError(ExitPreflight, "%v", err)
	}
	return ledger, nil
}

func (s *Service) DeferFinding(ref RunRef, id, reason, operator string) error {
	workspace, err := s.locateWorkspace(ref.Input)
	if err != nil {
		return exitError(ExitPreflight, "%v", err)
	}
	if err := storage.DeferFinding(workspace, id, reason, operator); err != nil {
		return exitError(ExitInvalidArguments, "%v", err)
	}
	return nil
}

func (s *Service) Decisions(ref RunRef) ([]storage.DecisionRecord, error) {
	workspace, err := s.locateWorkspace(ref.Input)
	if err != nil {
		return nil, exitError(ExitPreflight, "%v", err)
	}
	records, err := storage.ReadDecisions(workspace)
	if err != nil {
		return nil, exitError(ExitPreflight, "%v", err)
	}
	return records, nil
}

func (s *Service) Arbitrate(opts ArbitrationOptions) (domain.Final, error) {
	workspace, runDir, runID, err := s.locateRun(opts.RunRef)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	lock, err := storage.AcquireLock(context.Background(), filepath.Join(runDir, "run.lock"), nil)
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "acquire run lock: %v", err)
	}
	defer lock.Close()
	if err := storage.ValidateRunDir(workspace, runDir); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "revalidate run: %v", err)
	}
	final, err := readFinal(filepath.Join(runDir, "final.json"))
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if final.Status != "arbitration_pending" || len(final.Arbitration) == 0 {
		return domain.Final{}, exitError(ExitInvalidArguments, "run %s has no pending arbitration", runID)
	}
	rulings, err := arbitrationRulings(opts, final)
	if err != nil {
		return domain.Final{}, exitError(ExitInvalidArguments, "%v", err)
	}
	byID := map[string]ArbitrationRuling{}
	pendingIDs := map[string]bool{}
	for _, dispute := range final.Arbitration {
		pendingIDs[dispute.ID] = true
	}
	for _, ruling := range rulings {
		if !pendingIDs[ruling.ID] {
			return domain.Final{}, exitError(ExitInvalidArguments, "arbitration %q is not pending", ruling.ID)
		}
		if _, duplicate := byID[ruling.ID]; duplicate {
			return domain.Final{}, exitError(ExitInvalidArguments, "duplicate arbitration ruling %q", ruling.ID)
		}
		if ruling.Outcome != "accepted" && ruling.Outcome != "rejected" && ruling.Outcome != "deferred" {
			return domain.Final{}, exitError(ExitInvalidArguments, "arbitration %s has invalid outcome %q", ruling.ID, ruling.Outcome)
		}
		if ruling.Operator == "" {
			ruling.Operator = opts.Operator
		}
		if ruling.Operator == "" || ruling.Reason == "" {
			return domain.Final{}, exitError(ExitInvalidArguments, "arbitration %s requires operator and reason", ruling.ID)
		}
		byID[ruling.ID] = ruling
	}
	var pending []domain.ArbitrationDispute
	for _, dispute := range final.Arbitration {
		ruling, ok := byID[dispute.ID]
		if !ok {
			pending = append(pending, dispute)
			continue
		}
		updated := dispute.Decision
		updated.Outcome = ruling.Outcome
		updated.Reason = "operator_arbitration: " + ruling.Reason
		upsertDecision(&final.Decisions, updated)
		if err := storage.AppendDecision(workspace, storage.DecisionRecord{SchemaVersion: 1, PacketItem: dispute.Finding.Anchor.PacketItem, Fingerprint: domain.FindingFingerprint(dispute.Finding), Ruling: ruling.Outcome, Date: s.now(), Operator: ruling.Operator}); err != nil {
			return domain.Final{}, exitError(ExitPreflight, "persist decision: %v", err)
		}
	}
	final.Arbitration = pending
	final.FinishedAt = s.now()
	if len(pending) > 0 {
		final.Status, final.ExitCode = "arbitration_pending", ExitArbitration
	} else {
		final.Status, final.ExitCode = terminalOutcome(final.Decisions)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if err := s.publish(runDir, workspace, final, meta.Panel); err != nil {
		return domain.Final{}, exitError(ExitPreflight, "%v", err)
	}
	if final.ExitCode != 0 {
		return final, &ExitError{Code: final.ExitCode, Err: fmt.Errorf("%s", final.Status)}
	}
	return final, nil
}

func (s *Service) Doctor(ctx context.Context) DoctorReport {
	report := DoctorReport{SchemaVersion: 1, StateRoot: s.Store.Root, Adapters: s.Registry.Doctor(ctx)}
	path, err := exec.LookPath("pdftotext")
	if err != nil {
		report.PDFToText = adapters.VersionInfo{Adapter: "pdftotext", Hint: "install Poppler to review PDF documents"}
		return report
	}
	output, runErr := exec.CommandContext(ctx, path, "-v").CombinedOutput()
	report.PDFToText = adapters.VersionInfo{Adapter: "pdftotext", Found: true, Runnable: runErr == nil, Binary: path, Version: strings.TrimSpace(string(output))}
	return report
}

func (s *Service) Adopt(input string) (storage.Workspace, error) {
	workspace, err := s.locateWorkspace(input)
	if err != nil {
		return storage.Workspace{}, exitError(ExitPreflight, "%v", err)
	}
	_, root, err := documents.WorkspaceIdentity(input)
	if err != nil {
		return storage.Workspace{}, exitError(ExitPreflight, "%v", err)
	}
	alias := map[string]any{"schema_version": 1, "workspace_id": workspace.ID, "canonical_root": root, "adopted_at": s.now()}
	if err := storage.WriteJSON(filepath.Join(workspace.Root, "aliases.json"), alias); err != nil {
		return storage.Workspace{}, exitError(ExitPreflight, "persist adoption: %v", err)
	}
	return workspace, nil
}

func (s *Service) locateWorkspace(input string) (storage.Workspace, error) {
	if input == "" {
		input = "."
	}
	id, root, err := documents.WorkspaceIdentity(input)
	if err != nil {
		return storage.Workspace{}, err
	}
	return s.Store.Workspace(id, root)
}

func (s *Service) locateRun(ref RunRef) (storage.Workspace, string, string, error) {
	workspace, err := s.locateWorkspace(ref.Input)
	if err != nil {
		return storage.Workspace{}, "", "", err
	}
	runID := ref.RunID
	if runID == "" {
		var latest struct {
			SchemaVersion int    `json:"schema_version"`
			RunID         string `json:"run_id"`
		}
		if err := storage.ReadJSON(filepath.Join(workspace.Root, "latest.json"), &latest); err != nil {
			return storage.Workspace{}, "", "", fmt.Errorf("no latest Tribunal run: %w", err)
		}
		if latest.SchemaVersion != 1 || latest.RunID == "" {
			return storage.Workspace{}, "", "", fmt.Errorf("latest run pointer has unsupported schema")
		}
		runID = latest.RunID
	}
	if filepath.Base(runID) != runID || strings.ContainsAny(runID, `/\\`) {
		return storage.Workspace{}, "", "", fmt.Errorf("invalid run id %q", runID)
	}
	runDir := filepath.Join(workspace.RunsDir, runID)
	if err := storage.ValidateRunDir(workspace, runDir); err != nil {
		return storage.Workspace{}, "", "", fmt.Errorf("run %q not found", runID)
	}
	return workspace, runDir, runID, nil
}

func arbitrationRulings(opts ArbitrationOptions, final domain.Final) ([]ArbitrationRuling, error) {
	if len(opts.Rulings) > 0 {
		return opts.Rulings, nil
	}
	if opts.DecisionsPath != "" {
		var file ArbitrationFile
		raw, err := os.ReadFile(opts.DecisionsPath)
		if err != nil {
			return nil, err
		}
		if err := adapters.DecodeStrict(raw, arbitrationFileSchema, &file); err != nil {
			return nil, err
		}
		if file.SchemaVersion != 1 || (file.RunID != "" && file.RunID != final.RunID) {
			return nil, fmt.Errorf("decisions file schema or run id mismatch")
		}
		return file.Rulings, nil
	}
	if !opts.AcceptMajority {
		return nil, fmt.Errorf("non-interactive arbitration requires --decisions or --accept-majority")
	}
	excluded := map[string]bool{}
	for _, id := range opts.Except {
		excluded[id] = true
	}
	var rulings []ArbitrationRuling
	for _, dispute := range final.Arbitration {
		if excluded[dispute.ID] {
			continue
		}
		outcome := "rejected"
		if strings.HasPrefix(dispute.Default, "accept") {
			outcome = "accepted"
		}
		rulings = append(rulings, ArbitrationRuling{ID: dispute.ID, Outcome: outcome, Reason: "applied recorded panel recommendation", Operator: opts.Operator})
	}
	return rulings, nil
}

func readFinal(path string) (domain.Final, error) {
	var final domain.Final
	if err := storage.ReadJSON(path, &final); err != nil {
		return domain.Final{}, err
	}
	if final.SchemaVersion != 1 || final.RunID == "" || final.PacketHash == "" {
		return domain.Final{}, fmt.Errorf("unsupported or incomplete final artifact")
	}
	return final, nil
}

func upsertDecision(decisions *[]domain.Decision, value domain.Decision) {
	for i := range *decisions {
		if (*decisions)[i].FindingID == value.FindingID {
			(*decisions)[i] = value
			return
		}
	}
	*decisions = append(*decisions, value)
}

func terminalOutcome(decisions []domain.Decision) (string, int) {
	for _, decision := range decisions {
		if decision.Outcome == "accepted" && decision.Severity.Rank() >= domain.SeverityMajor.Rank() {
			return "findings", ExitBlockingFindings
		}
	}
	return "final", ExitSuccess
}

func firstError(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}
