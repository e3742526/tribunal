package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(args)
	err := root.Execute()
	return output.String(), err
}

func TestPanelShowRejectsPanelAndPolicyTogether(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runRoot(t, "--state-root", t.TempDir(), "panel", "show", t.TempDir(), "--panel", "claude/a,codex/b", "--panel-policy", "balanced")
	var exit *app.ExitError
	if !errors.As(err, &exit) || exit.Code != app.ExitInvalidArguments {
		t.Fatalf("Execute() error = %v, want ExitInvalidArguments", err)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error text %q", err.Error())
	}
}

func TestPanelListReportsBuiltinPolicies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	output, err := runRoot(t, "--json", "--state-root", t.TempDir(), "panel", "list", t.TempDir())
	if err != nil {
		t.Fatalf("panel list: %v", err)
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		Policies      []struct {
			Origin string `json:"origin"`
			Policy struct {
				Name string `json:"name"`
			} `json:"policy"`
		} `json:"policies"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, output)
	}
	if envelope.SchemaVersion != 1 || len(envelope.Policies) != 3 {
		t.Fatalf("unexpected policy listing: %s", output)
	}
	names := map[string]bool{}
	for _, entry := range envelope.Policies {
		if entry.Origin != "builtin" {
			t.Fatalf("unexpected origin %q", entry.Origin)
		}
		names[entry.Policy.Name] = true
	}
	for _, want := range []string{"balanced", "high-stakes", "frugal"} {
		if !names[want] {
			t.Fatalf("missing built-in policy %q: %s", want, output)
		}
	}
}

// panel show must resolve a panel without freezing a packet or writing run
// state; it is the command an operator uses before committing to a review.
func TestPanelShowResolvesPolicyWithoutWritingRunState(t *testing.T) {
	configHome, stateRoot, workspace := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	dir := filepath.Join(configHome, "tribunal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := `schema_version = 1
panel_policy = "frugal"

[[models]]
id = "opus"
adapter = "claude"
model = "claude-opus-5"
family = "anthropic"
capabilities = ["reasoning", "document-analysis"]
quality = 0.9
reliability = 0.9
cost = 1.0

[[models]]
id = "mistral"
adapter = "openai-compatible"
model = "mistral-large"
family = "mistral"
capabilities = ["literal-reading"]
quality = 0.6
reliability = 0.75
cost = 0.1
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(catalog), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runRoot(t, "--json", "--state-root", stateRoot, "panel", "show", workspace)
	if err != nil {
		t.Fatalf("panel show: %v", err)
	}
	var preview app.PanelPreview
	if err := json.Unmarshal([]byte(output), &preview); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, output)
	}
	if preview.Source != "panel policy" || preview.Policy != "frugal" || preview.Selection == nil {
		t.Fatalf("unexpected preview: %s", output)
	}
	if len(preview.Panel.Reviewers) != 2 || len(preview.Selection.Families) != 2 {
		t.Fatalf("frugal policy must seat two independent families: %s", output)
	}
	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("panel show wrote run state: %v", entries)
	}
	if workspaceEntries, err := os.ReadDir(workspace); err != nil || len(workspaceEntries) != 0 {
		t.Fatalf("panel show wrote into the document workspace: %v %v", workspaceEntries, err)
	}
}
