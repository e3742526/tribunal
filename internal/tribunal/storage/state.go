package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type StateEvent struct {
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	From          domain.RunPhase `json:"from,omitempty"`
	To            domain.RunPhase `json:"to"`
	Status        string          `json:"status"`
	At            time.Time       `json:"at"`
}

func Transition(runDir string, next domain.RunState) error {
	if next.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("run state schema_version must be %d", domain.SchemaVersion)
	}
	var previous domain.RunState
	err := ReadJSONStrict(filepath.Join(runDir, "state.json"), &previous)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if previous.SchemaVersion != 0 && previous.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("unsupported existing run state schema_version %d", previous.SchemaVersion)
	}
	if previous.SchemaVersion != 0 {
		if err := domain.ValidateRunState(previous); err != nil {
			return err
		}
	}
	event := StateEvent{SchemaVersion: domain.SchemaVersion, RunID: next.RunID, From: previous.Phase, To: next.Phase, Status: next.Status, At: next.UpdatedAt}
	if err := appendJSONLine(filepath.Join(runDir, "events.jsonl"), event); err != nil {
		return fmt.Errorf("append transition journal: %w", err)
	}
	if err := WriteJSON(filepath.Join(runDir, "state.json"), next); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	return nil
}

func PersistTerminal(runDir string, final domain.Final) error {
	if err := domain.ValidateFinal(final); err != nil {
		return fmt.Errorf("persist terminal final: %w", err)
	}
	var existing domain.Final
	if err := ReadJSONStrict(filepath.Join(runDir, "final.json"), &existing); err == nil {
		if err := domain.ValidateFinal(existing); err != nil {
			return err
		}
		if existing.RunID != final.RunID || existing.PacketHash != final.PacketHash || existing.WorkspaceID != final.WorkspaceID {
			return fmt.Errorf("terminal final conflicts with existing artifact")
		}
		if reflect.DeepEqual(existing, final) {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	state := StateForFinal(final)
	var existingState domain.RunState
	if err := ReadJSONStrict(filepath.Join(runDir, "state.json"), &existingState); err == nil {
		if err := domain.ValidateRunState(existingState); err != nil {
			return err
		}
		if existingState.RunID != final.RunID || existingState.PacketHash != final.PacketHash {
			return fmt.Errorf("terminal state conflicts with final identity")
		}
		if existingState.Phase != state.Phase || existingState.Status != state.Status {
			if err := Transition(runDir, state); err != nil {
				return fmt.Errorf("persist terminal state: %w", err)
			}
		}
	} else if os.IsNotExist(err) {
		if err := Transition(runDir, state); err != nil {
			return fmt.Errorf("persist terminal state: %w", err)
		}
	} else {
		return err
	}
	if err := WriteJSON(filepath.Join(runDir, "final.json"), final); err != nil {
		return fmt.Errorf("persist terminal final: %w", err)
	}
	return nil
}

type FindingRecord struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Fingerprint   string         `json:"fingerprint"`
	Finding       domain.Finding `json:"finding"`
	Status        string         `json:"status"`
	Reason        string         `json:"reason,omitempty"`
	ApprovedBy    string         `json:"approved_by,omitempty"`
	FirstRunID    string         `json:"first_run_id"`
	LastRunID     string         `json:"last_run_id"`
}

type FindingsLedger struct {
	SchemaVersion int             `json:"schema_version"`
	Findings      []FindingRecord `json:"findings"`
}

func LoadLedger(workspace Workspace) (FindingsLedger, error) {
	path := filepath.Join(workspace.Root, "findings.json")
	var ledger FindingsLedger
	if err := ReadJSONStrict(path, &ledger); err != nil {
		if os.IsNotExist(err) {
			return FindingsLedger{SchemaVersion: domain.SchemaVersion}, nil
		}
		return FindingsLedger{}, err
	}
	if ledger.SchemaVersion != domain.SchemaVersion {
		return FindingsLedger{}, fmt.Errorf("unsupported findings ledger schema_version %d", ledger.SchemaVersion)
	}
	for _, record := range ledger.Findings {
		if record.SchemaVersion != domain.SchemaVersion || record.ID == "" || record.FirstRunID == "" || record.LastRunID == "" {
			return FindingsLedger{}, fmt.Errorf("invalid finding ledger record")
		}
		// A legacy (pre-v0.1.0 8-hex) fingerprint would silently never match
		// recomputed identities, duplicating records; fail closed instead.
		if !domain.ValidFingerprint(record.Fingerprint) {
			return FindingsLedger{}, fmt.Errorf("incompatible finding fingerprint %q; ledger predates the 16-hex identity format", record.Fingerprint)
		}
		if err := domain.ValidateFinding(record.Finding); err != nil {
			return FindingsLedger{}, err
		}
	}
	return ledger, nil
}

func UpdateLedger(workspace Workspace, runID string, findings []domain.Finding, decisions []domain.Decision) error {
	lock, err := AcquireLock(context.Background(), filepath.Join(workspace.Root, "ledger.lock"), nil)
	if err != nil {
		return err
	}
	defer lock.Close()
	ledger, err := LoadLedger(workspace)
	if err != nil {
		return err
	}
	outcomes := map[string]string{}
	for _, decision := range decisions {
		outcomes[decision.FindingID] = decision.Outcome
	}
	seen := map[string]bool{}
	for _, finding := range findings {
		outcome := outcomes[finding.ID]
		fingerprint := domain.FindingFingerprint(finding)
		seen[fingerprint] = true
		found := false
		for i := range ledger.Findings {
			if ledger.Findings[i].Fingerprint == fingerprint {
				ledger.Findings[i].Finding = finding
				ledger.Findings[i].LastRunID = runID
				if ledger.Findings[i].Status != "deferred" {
					ledger.Findings[i].Status = statusForFinding(finding, outcome)
				}
				found = true
				break
			}
		}
		if !found {
			ledger.Findings = append(ledger.Findings, FindingRecord{SchemaVersion: domain.SchemaVersion, ID: fingerprint, Fingerprint: fingerprint, Finding: finding, Status: statusForFinding(finding, outcome), FirstRunID: runID, LastRunID: runID})
		}
	}
	for i := range ledger.Findings {
		if !seen[ledger.Findings[i].Fingerprint] && (ledger.Findings[i].Finding.Severity == domain.SeverityBlocker || ledger.Findings[i].Finding.Severity == domain.SeverityMajor) && ledger.Findings[i].Status == "open" {
			// Major findings remain open until explicitly disposed.
			continue
		}
	}
	return WriteJSON(filepath.Join(workspace.Root, "findings.json"), ledger)
}

func DeferFinding(workspace Workspace, id, reason, operator string) error {
	if reason == "" || operator == "" {
		return fmt.Errorf("defer requires reason and operator")
	}
	lock, err := AcquireLock(context.Background(), filepath.Join(workspace.Root, "ledger.lock"), nil)
	if err != nil {
		return err
	}
	defer lock.Close()
	ledger, err := LoadLedger(workspace)
	if err != nil {
		return err
	}
	for i := range ledger.Findings {
		if ledger.Findings[i].ID == id {
			ledger.Findings[i].Status = "deferred"
			ledger.Findings[i].Reason = reason
			ledger.Findings[i].ApprovedBy = operator
			return WriteJSON(filepath.Join(workspace.Root, "findings.json"), ledger)
		}
	}
	return fmt.Errorf("finding %q not found", id)
}

func statusForFinding(f domain.Finding, outcome string) string {
	if f.Quarantined {
		return "quarantined"
	}
	if outcome == "rejected" {
		return "rejected"
	}
	if outcome == "arbitration" {
		return "disputed"
	}
	if outcome == "deferred" {
		return "deferred"
	}
	if f.EvidenceStatus == domain.EvidenceUnevidenced {
		return "unverified-claim"
	}
	if outcome == "" {
		return "observed"
	}
	return "open"
}

type DecisionRecord struct {
	SchemaVersion int       `json:"schema_version"`
	PacketItem    string    `json:"packet_item"`
	Fingerprint   string    `json:"fingerprint"`
	Ruling        string    `json:"ruling"`
	Date          time.Time `json:"date"`
	Operator      string    `json:"operator"`
}

func AppendDecision(workspace Workspace, decision DecisionRecord) error {
	if decision.SchemaVersion != domain.SchemaVersion || decision.Fingerprint == "" || decision.Ruling == "" {
		return fmt.Errorf("invalid decision record")
	}
	lock, err := AcquireLock(context.Background(), filepath.Join(workspace.Root, "decisions.lock"), nil)
	if err != nil {
		return err
	}
	defer lock.Close()
	records, err := ReadDecisions(workspace)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.PacketItem == decision.PacketItem && record.Fingerprint == decision.Fingerprint && record.Ruling == decision.Ruling && record.Operator == decision.Operator {
			return nil
		}
	}
	return appendJSONLine(filepath.Join(workspace.Root, "decisions.jsonl"), decision)
}

func ReadDecisions(workspace Workspace) ([]DecisionRecord, error) {
	path := filepath.Join(workspace.Root, "decisions.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var records []DecisionRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record DecisionRecord
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("decode decision record: %w", err)
		}
		if record.SchemaVersion != domain.SchemaVersion || record.Ruling == "" || record.Operator == "" || record.Date.IsZero() {
			return nil, fmt.Errorf("unsupported decision schema_version %d", record.SchemaVersion)
		}
		if !domain.ValidFingerprint(record.Fingerprint) {
			return nil, fmt.Errorf("incompatible decision fingerprint %q; memory predates the 16-hex identity format", record.Fingerprint)
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

type Snapshot struct {
	SchemaVersion int             `json:"schema_version"`
	State         domain.RunState `json:"state"`
	Final         *domain.Final   `json:"final,omitempty"`
	Waiting       bool            `json:"waiting"`
	WaitPID       int             `json:"wait_pid,omitempty"`
}

func BuildSnapshot(runDir string) (Snapshot, error) {
	var state domain.RunState
	if err := ReadJSONStrict(filepath.Join(runDir, "state.json"), &state); err != nil {
		return Snapshot{}, err
	}
	if err := domain.ValidateRunState(state); err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{SchemaVersion: domain.SchemaVersion, State: state}
	waiting, pid, err := LockStatus(filepath.Join(runDir, "run.lock"))
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Waiting, snapshot.WaitPID = waiting, pid
	var final domain.Final
	if err := ReadJSONStrict(filepath.Join(runDir, "final.json"), &final); err == nil {
		if err := domain.ValidateFinal(final); err != nil {
			return Snapshot{}, err
		}
		snapshot.Final = &final
	} else if !os.IsNotExist(err) {
		return Snapshot{}, err
	}
	return snapshot, nil
}
