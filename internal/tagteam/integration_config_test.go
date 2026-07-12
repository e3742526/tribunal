package tagteam

import (
	"bytes"
	"testing"
)

func TestIntegrationMarkedBlockPreservesUserContent(t *testing.T) {
	original := []byte("# user configuration\nsetting = true\n")
	plan, err := PlanIntegration("codex", original)
	if err != nil || !plan.Changed {
		t.Fatalf("plan = %#v, %v", plan, err)
	}
	if !bytes.Contains(plan.Content, original) {
		t.Fatalf("unowned content changed: %q", plan.Content)
	}
	doctor := DoctorIntegration("codex", plan.Content)
	if doctor.Status != "installed" {
		t.Fatalf("doctor = %#v", doctor)
	}
	removed, err := UninstallIntegration("codex", plan.Content)
	if err != nil || !bytes.Equal(removed.Content, original) {
		t.Fatalf("uninstall = %q, %v", removed.Content, err)
	}
	if DoctorIntegration("codex", []byte("# BEGIN tagteam managed integration\n")).Status != "corrupt" {
		t.Fatal("corrupt markers not detected")
	}
}

func TestIntegrationJSONPreservesUnknownKeys(t *testing.T) {
	plan, err := PlanIntegration("mcp-json", []byte(`{"other":{"keep":true},"mcpServers":{"existing":{"command":"x"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(plan.Content, []byte(`"other"`)) || !bytes.Contains(plan.Content, []byte(`"tagteam"`)) {
		t.Fatalf("JSON plan = %s", plan.Content)
	}
}
