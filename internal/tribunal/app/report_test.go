package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestHTMLReportEscapesUntrustedFindingContent(t *testing.T) {
	runDir := t.TempDir()
	final := domain.Final{SchemaVersion: 1, RunID: "run", PacketHash: "packet", Status: "final", Findings: []domain.Finding{{ID: "F-1", Severity: domain.SeverityMinor, Category: domain.CategoryIntegrity, Issue: `<script>alert("x")</script>`, Recommendation: `<img src=x onerror=alert(1)>`}}}
	if err := writeReports(runDir, final, domain.Panel{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>") || strings.Contains(string(html), "<img src=x") {
		t.Fatalf("untrusted HTML was not escaped: %s", html)
	}
}
