package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCommandHelpIncludesModeModelAndFlags(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}

	help := out.String()
	checks := []string{
		"Modes",
		"supervisor (default)",
		"relay",
		"solo",
		"adversarial",
		"Role flags by mode",
		"--mode",
		"--worker",
		"--supervisor",
		"--mc",
		"--ma",
		"--reviewer",
		"--scout",
		"--no-scout-retrieval",
		"tagteam --solo codex:gpt-5.5",
		"tagteam --relay --no-scout-retrieval",
		"tagteam --mode adversarial -mc codex:gpt-5-codex -ma claude:opus",
	}
	for _, want := range checks {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q\nfull output:\n%s", want, help)
		}
	}
}
