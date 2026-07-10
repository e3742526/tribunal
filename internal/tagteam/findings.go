package tagteam

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const findingsLedgerFilename = "findings.json"

type FindingEntry struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	Severity   string    `json:"severity"`
	File       string    `json:"file,omitempty"`
	Line       int       `json:"line,omitempty"`
	Issue      string    `json:"issue"`
	Fix        string    `json:"fix,omitempty"`
	Status     string    `json:"status"`
	Evidence   string    `json:"evidence,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	FirstRound int       `json:"first_round,omitempty"`
	LastRound  int       `json:"last_round,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FindingsLedger struct {
	SchemaVersion int            `json:"schema_version"`
	RunID         string         `json:"run_id"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Entries       []FindingEntry `json:"entries"`
}

type FindingsSummary struct {
	Path               string `json:"path,omitempty"`
	OpenBlockerOrMajor int    `json:"open_blocker_or_major"`
	OpenTotal          int    `json:"open_total"`
}

func stableFindingID(finding Finding) string {
	canonical := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(finding.Severity)),
		filepath.ToSlash(strings.TrimSpace(finding.File)),
		fmt.Sprintf("%d", finding.Line),
		strings.TrimSpace(finding.Issue),
		strings.TrimSpace(finding.Fix),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return "finding-" + hex.EncodeToString(sum[:8])
}

func stableGateFindingID(finding GateFinding) string {
	return stableFindingID(Finding{Severity: finding.Severity, File: finding.Path, Issue: finding.Message, Fix: "resolve quality gate"})
}

func notApplicableDataLossChecks(evidence string) *DataLossChecks {
	check := DataLossCheck{Status: "not_applicable", Evidence: evidence}
	return &DataLossChecks{
		MalformedInputPreservation: check,
		AnnotationHistoryRetention: check,
		AmbiguousIdentityHandling:  check,
		ReadOnlyNonMutation:        check,
	}
}

func loadFindingsLedger(runDir string) (FindingsLedger, error) {
	path := filepath.Join(runDir, findingsLedgerFilename)
	ledger := FindingsLedger{SchemaVersion: ArtifactSchemaVersion, RunID: filepath.Base(runDir), Entries: []FindingEntry{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ledger, nil
	}
	if err != nil {
		return FindingsLedger{}, err
	}
	if err := json.Unmarshal(data, &ledger); err != nil {
		return FindingsLedger{}, fmt.Errorf("decode findings ledger: %w", err)
	}
	if ledger.SchemaVersion != ArtifactSchemaVersion {
		return FindingsLedger{}, fmt.Errorf("unsupported findings schema_version %d", ledger.SchemaVersion)
	}
	return ledger, nil
}

func updateFindingsLedger(runDir string, round int, review *Review, gates *QualityGateResult) (FindingsSummary, error) {
	ledger, err := loadFindingsLedger(runDir)
	if err != nil {
		return FindingsSummary{}, err
	}
	now := time.Now().UTC()
	entries := make(map[string]FindingEntry, len(ledger.Entries))
	for _, entry := range ledger.Entries {
		entries[entry.ID] = entry
	}
	if review != nil {
		for _, disposition := range review.PriorFindingDispositions {
			entry, ok := entries[disposition.FindingID]
			if !ok {
				return FindingsSummary{}, fmt.Errorf("review disposed unknown finding %q", disposition.FindingID)
			}
			entry.Status = disposition.Status
			entry.Evidence = disposition.Evidence
			entry.UpdatedAt = now
			entries[entry.ID] = entry
		}
		for _, finding := range review.Findings {
			id := finding.ID
			if id == "" {
				id = stableFindingID(finding)
			}
			entry, ok := entries[id]
			if !ok {
				entry.FirstRound = round
			}
			entry.ID = id
			entry.Source = "review"
			entry.Severity = finding.Severity
			entry.File = finding.File
			entry.Line = finding.Line
			entry.Issue = finding.Issue
			entry.Fix = finding.Fix
			entry.Status = "open"
			entry.LastRound = round
			entry.UpdatedAt = now
			entries[id] = entry
		}
	}
	if gates != nil {
		for _, finding := range gates.Findings {
			id := stableGateFindingID(finding)
			entry := entries[id]
			entry.ID = id
			entry.Source = "quality_gate"
			entry.Severity = finding.Severity
			entry.File = finding.Path
			entry.Issue = finding.Message
			entry.Fix = "resolve the gate violation or record evidence through review"
			entry.Status = "open"
			entry.LastRound = round
			if entry.FirstRound == 0 {
				entry.FirstRound = round
			}
			entry.UpdatedAt = now
			entries[id] = entry
		}
	}
	ledger.Entries = ledger.Entries[:0]
	for _, entry := range entries {
		ledger.Entries = append(ledger.Entries, entry)
	}
	sort.Slice(ledger.Entries, func(i, j int) bool { return ledger.Entries[i].ID < ledger.Entries[j].ID })
	ledger.SchemaVersion = ArtifactSchemaVersion
	ledger.UpdatedAt = now
	path := filepath.Join(runDir, findingsLedgerFilename)
	if err := writeJSONWithNewline(path, ledger); err != nil {
		return FindingsSummary{}, err
	}
	return summarizeFindings(path, ledger), nil
}

func summarizeFindings(path string, ledger FindingsLedger) FindingsSummary {
	summary := FindingsSummary{Path: path}
	for _, entry := range ledger.Entries {
		if entry.Status != "open" {
			continue
		}
		summary.OpenTotal++
		if entry.Severity == "blocker" || entry.Severity == "major" {
			summary.OpenBlockerOrMajor++
		}
	}
	return summary
}

func DeferFinding(runDir, findingID, reason string) (FindingsSummary, error) {
	if strings.TrimSpace(reason) == "" {
		return FindingsSummary{}, fmt.Errorf("deferral reason is required")
	}
	ledger, err := loadFindingsLedger(runDir)
	if err != nil {
		return FindingsSummary{}, err
	}
	found := false
	for i := range ledger.Entries {
		if ledger.Entries[i].ID != findingID {
			continue
		}
		if ledger.Entries[i].Severity != "blocker" && ledger.Entries[i].Severity != "major" {
			return FindingsSummary{}, fmt.Errorf("finding %q is not blocker or major", findingID)
		}
		ledger.Entries[i].Status = "deferred_with_approval"
		ledger.Entries[i].Reason = strings.TrimSpace(reason)
		ledger.Entries[i].ApprovedBy = "operator"
		ledger.Entries[i].UpdatedAt = time.Now().UTC()
		found = true
		break
	}
	if !found {
		return FindingsSummary{}, fmt.Errorf("finding %q not found", findingID)
	}
	ledger.UpdatedAt = time.Now().UTC()
	path := filepath.Join(runDir, findingsLedgerFilename)
	if err := writeJSONWithNewline(path, ledger); err != nil {
		return FindingsSummary{}, err
	}
	return summarizeFindings(path, ledger), nil
}
