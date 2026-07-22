package app

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func writeReports(runDir string, final domain.Final, panel domain.Panel) error {
	var markdown strings.Builder
	markdown.WriteString("# Tribunal report\n\n")
	fmt.Fprintf(&markdown, "Run `%s` · packet `%s` · status **%s**\n\n", final.RunID, final.PacketHash, final.Status)
	if final.PanelIncomplete {
		markdown.WriteString("> [!WARNING]\n> Panel incomplete; agreement is reported against the configured panel.\n\n")
	}
	markdown.WriteString("## Panel\n\n| Reviewer | Adapter | Model | Family | Persona | Weight | Status |\n|---|---|---|---|---|---:|---|\n")
	status := map[string]string{}
	for _, item := range final.PanelStatus {
		status[item.ReviewerID] = item.Status
	}
	for _, reviewer := range panel.Reviewers {
		fmt.Fprintf(&markdown, "| %s | %s | %s | %s | %s | %.2f | %s |\n", reviewer.ID, reviewer.Adapter, reviewer.Model, reviewer.Family, reviewer.Persona, reviewer.Weight, status[reviewer.ID])
	}
	if len(panel.Reviewers) > 0 {
		fmt.Fprintf(&markdown, "\n%s\n", domain.DiversityNote(panel))
	}
	markdown.WriteString("\n## Findings and decisions\n")
	decisionByID := map[string]domain.Decision{}
	for _, decision := range final.Decisions {
		decisionByID[decision.FindingID] = decision
	}
	for _, finding := range final.Findings {
		fmt.Fprintf(&markdown, "\n### %s · %s · %s\n\n%s\n\nRecommendation: %s\n", finding.ID, finding.Severity, finding.Category, finding.Issue, finding.Recommendation)
		if decision, ok := decisionByID[finding.ID]; ok {
			fmt.Fprintf(&markdown, "\nDecision: **%s** (%d accept, %d reject, %d abstain of %d configured)\n", decision.Outcome, decision.Accepts, decision.Rejects, decision.Abstains, decision.Configured)
			for _, dissent := range decision.Dissent {
				fmt.Fprintf(&markdown, "- Dissent %s: %s\n", dissent.ReviewerID, dissent.Reason)
			}
		}
		if finding.Quarantined {
			fmt.Fprintf(&markdown, "\nQuarantined: %s\n", finding.QuarantineWhy)
		}
	}
	if len(final.Arbitration) > 0 {
		markdown.WriteString("\n## Arbitration required\n")
		for _, dispute := range final.Arbitration {
			fmt.Fprintf(&markdown, "\n- **%s** %s — default: %s\n", dispute.ID, dispute.Finding.Issue, dispute.Default)
			if dispute.MemoryHint != "" {
				fmt.Fprintf(&markdown, "  - decision memory: %s\n", dispute.MemoryHint)
			}
		}
	}
	if err := storage.WriteFile(filepath.Join(runDir, "report.md"), []byte(markdown.String())); err != nil {
		return fmt.Errorf("persist Markdown report: %w", err)
	}
	const htmlTemplate = `<!doctype html><html><head><meta charset="utf-8"><title>Tribunal report</title><style>body{font:16px system-ui;max-width:900px;margin:2rem auto;padding:0 1rem}article{border-top:1px solid #ccc;padding:1rem 0}.major,.blocker{color:#9b1c1c}</style></head><body><h1>Tribunal report</h1><p>Run {{.RunID}} · status <strong>{{.Status}}</strong></p>{{range .Findings}}<article><h2 class="{{.Severity}}">{{.ID}} · {{.Severity}} · {{.Category}}</h2><p>{{.Issue}}</p><p><strong>Recommendation:</strong> {{.Recommendation}}</p></article>{{end}}</body></html>`
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	var html strings.Builder
	if err := tmpl.Execute(&html, final); err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(runDir, "report.html"), []byte(html.String()))
}

func readReport(runDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	return string(data), err
}
