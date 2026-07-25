package app

import (
	"fmt"
	"os"

	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func summaryFor(decisions []domain.Decision, disputes []domain.ArbitrationDispute, findings []domain.Finding) string {
	accepted := 0
	for _, decision := range decisions {
		if decision.Outcome == "accepted" {
			accepted++
		}
	}
	summary := fmt.Sprintf("%d accepted recommendations; %d disputes require arbitration.", accepted, len(disputes))
	// A run whose findings were quarantined before voting must not read as
	// an unqualified clean result (live playtest L-02: a science run lost
	// every finding to quarantine and still summarized as clean).
	quarantined := 0
	for _, finding := range findings {
		if finding.Quarantined {
			quarantined++
		}
	}
	if quarantined > 0 {
		summary += fmt.Sprintf(" %d finding(s) were quarantined before voting; inspect quarantine_reason per finding.", quarantined)
	}
	return summary
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
