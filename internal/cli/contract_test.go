package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

// A failed command in --json mode must emit a schema-versioned error envelope
// with the real exit code, never a zero-value result object claiming
// schema_version 0 and exit_code 0.
func TestJSONFailureEmitsErrorEnvelopeNotZeroValueFinal(t *testing.T) {
	stateRoot := t.TempDir()
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--json", "--state-root", stateRoot, "review", "/nonexistent-tribunal-document"})
	err := root.Execute()
	var exit *app.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("Execute() error = %v, want ExitError", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, output.String())
	}
	if envelope["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v, want 1", envelope["schema_version"])
	}
	if envelope["error"] == "" || envelope["error"] == nil {
		t.Fatalf("error envelope missing error text: %v", envelope)
	}
	if envelope["exit_code"] != float64(exit.Code) {
		t.Fatalf("exit_code = %v, want %d", envelope["exit_code"], exit.Code)
	}
	if _, hasStatus := envelope["status"]; hasStatus {
		t.Fatalf("failure output must not impersonate a final result: %v", envelope)
	}
}

func TestRecommendRejectsRubricAndKindTogether(t *testing.T) {
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--kind", "governance", "recommend", "--rubric", "manuscript", "."})
	err := root.Execute()
	var exit *app.ExitError
	if !errors.As(err, &exit) || exit.Code != app.ExitInvalidArguments {
		t.Fatalf("Execute() error = %v, want invalid-arguments ExitError", err)
	}
}

func TestExitCodeForClassifiesErrors(t *testing.T) {
	if code := app.ExitCodeFor(nil); code != app.ExitSuccess {
		t.Fatalf("nil error code = %d, want %d", code, app.ExitSuccess)
	}
	if code := app.ExitCodeFor(&app.ExitError{Code: app.ExitInternal, Err: errors.New("x")}); code != app.ExitInternal {
		t.Fatalf("ExitError code = %d, want %d", code, app.ExitInternal)
	}
	// Plain errors at the CLI boundary come from cobra parsing, so they keep
	// the contract's invalid-arguments code.
	if code := app.ExitCodeFor(errors.New("unknown flag: --bogus")); code != app.ExitInvalidArguments {
		t.Fatalf("unclassified error code = %d, want %d", code, app.ExitInvalidArguments)
	}
}

// Cobra parse failures (unknown flags, wrong argument counts) must keep the
// documented invalid-arguments exit code.
func TestParseErrorsKeepInvalidArgumentsExitCode(t *testing.T) {
	for _, args := range [][]string{{"--bogus-flag", "review", "x"}, {"review"}, {"frobnicate"}} {
		root := NewRootCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(args)
		if code := app.ExitCodeFor(root.Execute()); code != app.ExitInvalidArguments {
			t.Fatalf("args %v exit code = %d, want %d", args, code, app.ExitInvalidArguments)
		}
	}
}

// Errors raised before any result rendering (here: the rubric/kind conflict)
// must still emit the JSON error envelope via the RunE decorator.
func TestEarlyCommandErrorStillEmitsJSONEnvelope(t *testing.T) {
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--json", "--kind", "governance", "recommend", "--rubric", "manuscript", "."})
	err := root.Execute()
	var exit *app.ExitError
	if !errors.As(err, &exit) || exit.Code != app.ExitInvalidArguments {
		t.Fatalf("Execute() error = %v, want invalid-arguments ExitError", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, output.String())
	}
	if envelope["schema_version"] != float64(1) || envelope["exit_code"] != float64(app.ExitInvalidArguments) {
		t.Fatalf("unexpected envelope: %v", envelope)
	}
}
