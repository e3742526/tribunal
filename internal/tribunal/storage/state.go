package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	err := ReadJSON(filepath.Join(runDir, "state.json"), &previous)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if previous.SchemaVersion != 0 && previous.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("unsupported existing run state schema_version %d", previous.SchemaVersion)
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
	state := StateForFinal(final)
	if err := Transition(runDir, state); err != nil {
		return fmt.Errorf("persist terminal state: %w", err)
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
	if err := ReadJSON(path, &ledger); err != nil {
		if os.IsNotExist(err) {
			return FindingsLedger{SchemaVersion: domain.SchemaVersion}, nil
		}
		return FindingsLedger{}, err
	}
	if ledger.SchemaVersion != domain.SchemaVersion {
		return FindingsLedger{}, fmt.Errorf("unsupported findings ledger schema_version %d", ledger.SchemaVersion)
	}
	return ledger, nil
}

func UpdateLedger(workspace Workspace, runID string, findings []domain.Finding) error {
	ledger, err := LoadLedger(workspace)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, finding := range findings {
		fingerprint := domain.FindingFingerprint(finding)
		seen[fingerprint] = true
		found := false
		for i := range ledger.Findings {
			if ledger.Findings[i].Fingerprint == fingerprint {
				ledger.Findings[i].Finding = finding
				ledger.Findings[i].LastRunID = runID
				found = true
				break
			}
		}
		if !found {
			ledger.Findings = append(ledger.Findings, FindingRecord{SchemaVersion: domain.SchemaVersion, ID: fingerprint, Fingerprint: fingerprint, Finding: finding, Status: statusForFinding(finding), FirstRunID: runID, LastRunID: runID})
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

func statusForFinding(f domain.Finding) string {
	if f.Quarantined {
		return "quarantined"
	}
	if f.EvidenceStatus == domain.EvidenceUnevidenced {
		return "unverified-claim"
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
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode decision record: %w", err)
		}
		if record.SchemaVersion != domain.SchemaVersion {
			return nil, fmt.Errorf("unsupported decision schema_version %d", record.SchemaVersion)
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
	if err := ReadJSON(filepath.Join(runDir, "state.json"), &state); err != nil {
		return Snapshot{}, err
	}
	if state.SchemaVersion != domain.SchemaVersion {
		return Snapshot{}, fmt.Errorf("unsupported run state schema_version %d", state.SchemaVersion)
	}
	snapshot := Snapshot{SchemaVersion: domain.SchemaVersion, State: state}
	var final domain.Final
	if err := ReadJSON(filepath.Join(runDir, "final.json"), &final); err == nil {
		if final.SchemaVersion != domain.SchemaVersion {
			return Snapshot{}, fmt.Errorf("unsupported final schema_version %d", final.SchemaVersion)
		}
		snapshot.Final = &final
	} else if !os.IsNotExist(err) {
		return Snapshot{}, err
	}
	return snapshot, nil
}
