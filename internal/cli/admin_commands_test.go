package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

func TestDoctorHumanOutputShowsNonRunnableAdapterHint(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--state-root", t.TempDir(), "doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "hint: install claude") {
		t.Fatalf("doctor output omitted the non-runnable adapter hint:\n%s", output.String())
	}
	if strings.Contains(output.String(), `"schema_version"`) {
		t.Fatalf("human doctor output unexpectedly used JSON: %s", output.String())
	}

	root = NewRootCommand()
	output.Reset()
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--json", "--state-root", t.TempDir(), "doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("JSON Execute() error = %v\n%s", err, output.String())
	}
	var report app.DoctorReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("doctor JSON is not a stable report: %v\n%s", err, output.String())
	}
	if report.SchemaVersion != 1 || len(report.Adapters) == 0 {
		t.Fatalf("unexpected doctor JSON report: %+v", report)
	}
}
