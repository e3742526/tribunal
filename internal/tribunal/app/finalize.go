package app

import (
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

// ledgerScopeEligible reports whether a final's status proves the panel
// completed its examination, which is the precondition for marking absent
// ledger records stale.
func ledgerScopeEligible(status string) bool {
	return status == "final" || status == "findings" || status == "arbitration_pending"
}

func buildFinal(runID string, packet documents.Packet, started, finished time.Time, statuses []domain.PanelStatus, findings []domain.Finding, evidence []domain.EvidenceItem, decisions []domain.Decision, disputes []domain.ArbitrationDispute, reasons []string) domain.Final {
	exit, status := ExitSuccess, "final"
	if len(disputes) > 0 {
		exit, status = ExitArbitration, "arbitration_pending"
	} else {
		for _, decision := range decisions {
			if decision.Outcome == "accepted" && decision.Severity.Rank() >= domain.SeverityMajor.Rank() {
				exit, status = ExitBlockingFindings, "findings"
				break
			}
		}
	}
	return domain.Final{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, PacketHash: packet.PacketHash, Status: status, ExitCode: exit, Summary: summaryFor(decisions, disputes, findings), PanelIncomplete: panelIncomplete(statuses), ReasonCodes: unique(reasons), PanelStatus: statuses, Findings: findings, Evidence: evidence, Decisions: decisions, Arbitration: disputes, StartedAt: started, FinishedAt: finished}
}

func (s *Service) finalizeDegraded(runDir string, workspace storage.Workspace, runID string, packet documents.Packet, panel domain.Panel, started time.Time, statuses []domain.PanelStatus, findings []domain.Finding, reasons []string) (domain.Final, error) {
	final := domain.Final{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, PacketHash: packet.PacketHash, Status: "degraded", ExitCode: ExitDegraded, Summary: "Panel quorum was not met; independent findings only.", PanelIncomplete: true, DegradedReason: degradedReason(statuses), ReasonCodes: unique(append(reasons, "quorum_unmet")), PanelStatus: statuses, Findings: findings, StartedAt: started, FinishedAt: s.now()}
	if err := s.publish(runDir, workspace, final, panel); err != nil {
		return domain.Final{}, err
	}
	return final, nil
}

// finalizeAborted carries the run's accumulated reason codes into the aborted
// final instead of replacing them, so diagnostics like json_repair_used are
// not lost at the abort boundary.
func (s *Service) finalizeAborted(runDir string, workspace storage.Workspace, runID string, packet documents.Packet, panel domain.Panel, started time.Time, statuses []domain.PanelStatus, findings []domain.Finding, reasons []string, cause error) (domain.Final, error) {
	final := domain.Final{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, PacketHash: packet.PacketHash, Status: "aborted", ExitCode: ExitAborted, Summary: "Run aborted after persisting all available results.", PanelIncomplete: true, DegradedReason: cause.Error(), ReasonCodes: unique(append(append([]string{}, reasons...), "run_aborted")), PanelStatus: statuses, Findings: findings, StartedAt: started, FinishedAt: s.now()}
	if err := s.publish(runDir, workspace, final, panel); err != nil {
		return domain.Final{}, err
	}
	return final, nil
}
