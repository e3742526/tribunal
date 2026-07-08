package tagteam

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCodexBuildCmd(t *testing.T) {
	adapter := &CodexAdapter{IDValue: "codex", DefaultModel: "gpt-5-codex", ExtraArgs: []string{"--foo", "bar"}}
	spec, err := adapter.BuildCmd(RoleCoder, Request{
		Prompt:     "make it work",
		Model:      "",
		Workdir:    "/repo",
		OutputPath: "/tmp/out.md",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"codex", "exec", "-C", "/repo", "-s", "workspace-write", "-m", "gpt-5-codex", "-o", "/tmp/out.md", "--foo", "bar", "make it work"}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestClaudeBuildCmdAdversary(t *testing.T) {
	tmp := t.TempDir()
	schemaPath := filepath.Join(tmp, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := &ClaudeAdapter{DefaultModel: "opus"}
	spec, err := adapter.BuildCmd(RoleAdversary, Request{
		Prompt:     "review",
		Model:      "",
		Workdir:    "/repo",
		SchemaPath: schemaPath,
		Stdin:      []byte("diff"),
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	if spec.Argv[0] != "claude" {
		t.Fatalf("binary = %q", spec.Argv[0])
	}
	if spec.Argv[4] != "opus" {
		t.Fatalf("model = %q", spec.Argv[4])
	}
	if len(spec.Stdin) == 0 {
		t.Fatalf("stdin was not forwarded")
	}
}

func TestClaudeParseResult(t *testing.T) {
	adapter := &ClaudeAdapter{}
	raw := []byte(`{"result":"ok","session_id":"sess_1","total_cost_usd":1.25,"structured_output":{"verdict":"pass","summary":"looks good","findings":[],"test_suggestions":[]}}`)
	result, err := adapter.ParseResult(RoleAdversary, raw)
	if err != nil {
		t.Fatalf("ParseResult() error = %v", err)
	}
	if result.SessionID != "sess_1" {
		t.Fatalf("session_id = %q", result.SessionID)
	}
	if result.Review == nil || result.Review.Verdict != "pass" {
		t.Fatalf("review = %#v", result.Review)
	}
}

func TestClaudeParseResultFallsBackToResultJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}
	raw := []byte(`{"result":"{\"verdict\":\"pass\",\"summary\":\"looks good\",\"findings\":[],\"test_suggestions\":[]}","session_id":"sess_1","total_cost_usd":1.25}`)
	result, err := adapter.ParseResult(RoleAdversary, raw)
	if err != nil {
		t.Fatalf("ParseResult() error = %v", err)
	}
	if result.Review == nil || result.Review.Verdict != "pass" {
		t.Fatalf("review = %#v", result.Review)
	}
}

func TestClaudeParseResultExtractsFencedResultJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}
	raw := []byte("{\"result\":\"```json\\n{\\\"verdict\\\":\\\"pass\\\",\\\"summary\\\":\\\"looks good\\\",\\\"findings\\\":[],\\\"test_suggestions\\\":[]}\\n```\"}")
	result, err := adapter.ParseResult(RoleAdversary, raw)
	if err != nil {
		t.Fatalf("ParseResult() error = %v", err)
	}
	if result.Review == nil || result.Review.Verdict != "pass" {
		t.Fatalf("review = %#v", result.Review)
	}
}

func TestClaudeParseResultWrapsContractErrors(t *testing.T) {
	adapter := &ClaudeAdapter{}
	_, err := adapter.ParseResult(RoleAdversary, []byte(`{"result":"ok","structured_output":{"verdict":"pass","summary":"bad","findings":[{"severity":"major","file":"main.go","issue":"bug","fix":"fix"}],"test_suggestions":[]}}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsOutputContractError(err) {
		t.Fatalf("expected output contract error, got %T", err)
	}
}

func TestAgyBuildCmdCoder(t *testing.T) {
	adapter := &AgyAdapter{DefaultModel: "gemini-3.5-flash", ExtraArgs: []string{"--project", "demo"}}
	spec, err := adapter.BuildCmd(RoleCoder, Request{
		Prompt:  "make it work",
		Workdir: "/repo",
		Timeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{
		"agy", "--print", "make it work",
		"--model", "gemini-3.5-flash",
		"--print-timeout", "15s",
		"--dangerously-skip-permissions",
		"--project", "demo",
	}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestAgyBuildCmdAdversary(t *testing.T) {
	adapter := &AgyAdapter{DefaultModel: "gemini-3.5-flash"}
	spec, err := adapter.BuildCmd(RoleAdversary, Request{
		Prompt:  "review",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"agy", "--print", "review", "--model", "gemini-3.5-flash", "--sandbox"}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestAgyParseResultExtractsFencedReviewJSON(t *testing.T) {
	adapter := &AgyAdapter{}
	raw := []byte("```json\n{\"verdict\":\"pass\",\"summary\":\"looks good\",\"findings\":[],\"test_suggestions\":[]}\n```")
	result, err := adapter.ParseResult(RoleAdversary, raw)
	if err != nil {
		t.Fatalf("ParseResult() error = %v", err)
	}
	if result.Review == nil || result.Review.Verdict != "pass" {
		t.Fatalf("review = %#v", result.Review)
	}
}
