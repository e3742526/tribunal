package tagteam

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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

func TestCodexBuildCmdSupervisor(t *testing.T) {
	adapter := &CodexAdapter{IDValue: "codex", DefaultModel: "gpt-5-codex"}
	spec, err := adapter.BuildCmd(RoleSupervisor, Request{
		Prompt:  "write a brief",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"codex", "exec", "-C", "/repo", "-s", "read-only", "-m", "gpt-5-codex", "write a brief"}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestCodexBuildCmdReporterIsReadOnly(t *testing.T) {
	adapter := &CodexAdapter{IDValue: "codex", DefaultModel: "gpt-5-codex"}
	spec, err := adapter.BuildCmd(RoleReporter, Request{
		Prompt:  "report remaining work",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"codex", "exec", "-C", "/repo", "-s", "read-only", "-m", "gpt-5-codex", "report remaining work"}
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

func TestClaudeBuildCmdSupervisor(t *testing.T) {
	adapter := &ClaudeAdapter{DefaultModel: "opus"}
	spec, err := adapter.BuildCmd(RoleSupervisor, Request{
		Prompt:  "write a brief",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	if spec.Argv[0] != "claude" {
		t.Fatalf("binary = %q", spec.Argv[0])
	}
	argv := strings.Join(spec.Argv, " ")
	if !strings.Contains(argv, "--permission-mode dontAsk") {
		t.Fatalf("expected read-only permission mode, got argv = %v", spec.Argv)
	}
	if strings.Contains(argv, "--json-schema") {
		t.Fatalf("supervisor brief step should not force a JSON schema, got argv = %v", spec.Argv)
	}
}

func TestClaudeBuildCmdReporterDoesNotUseSchema(t *testing.T) {
	adapter := &ClaudeAdapter{DefaultModel: "opus"}
	spec, err := adapter.BuildCmd(RoleReporter, Request{
		Prompt:  "report remaining work",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	argv := strings.Join(spec.Argv, " ")
	if !strings.Contains(argv, "--permission-mode dontAsk") {
		t.Fatalf("expected read-only permission mode, got argv = %v", spec.Argv)
	}
	if strings.Contains(argv, "--json-schema") {
		t.Fatalf("reporter step should not force a JSON schema, got argv = %v", spec.Argv)
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

func TestAgyBuildCmdSupervisor(t *testing.T) {
	adapter := &AgyAdapter{DefaultModel: "gemini-3.5-flash"}
	spec, err := adapter.BuildCmd(RoleSupervisor, Request{
		Prompt:  "write a brief",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"agy", "--print", "write a brief", "--model", "gemini-3.5-flash", "--sandbox"}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestAgyBuildCmdReporter(t *testing.T) {
	adapter := &AgyAdapter{DefaultModel: "gemini-3.5-flash"}
	spec, err := adapter.BuildCmd(RoleReporter, Request{
		Prompt:  "report remaining work",
		Workdir: "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{"agy", "--print", "report remaining work", "--model", "gemini-3.5-flash", "--sandbox"}
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

func TestGoslingBuildCmdCoder(t *testing.T) {
	adapter := &GoslingAdapter{DefaultModel: "gemini-2.5-flash", ExtraArgs: []string{"--foo", "bar"}}
	spec, err := adapter.BuildCmd(RoleCoder, Request{
		Prompt:       "do something",
		SystemPrompt: "you are a helper",
		Workdir:      "/repo",
	})
	if err != nil {
		t.Fatalf("BuildCmd() error = %v", err)
	}
	want := []string{
		"gosling", "run", "--no-session", "--quiet", "--output-format", "json",
		"--model", "gemini-2.5-flash",
		"--text", "do something",
		"--foo", "bar",
	}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv mismatch\nwant: %#v\ngot:  %#v", want, spec.Argv)
	}
}

func TestGoslingBuildCmdAdversary(t *testing.T) {
	adapter := &GoslingAdapter{DefaultModel: "gemini-2.5-flash"}
	_, err := adapter.BuildCmd(RoleAdversary, Request{
		Prompt:  "review code",
		Workdir: "/repo",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gosling is not supported as an adversary adapter") {
		t.Fatalf("expected error containing 'gosling is not supported as an adversary adapter', got: %v", err)
	}
}

func TestGoslingBuildCmdSupervisor(t *testing.T) {
	adapter := &GoslingAdapter{DefaultModel: "gemini-2.5-flash"}
	_, err := adapter.BuildCmd(RoleSupervisor, Request{
		Prompt:  "write a brief",
		Workdir: "/repo",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gosling is not supported as a supervisor adapter") {
		t.Fatalf("expected error containing 'gosling is not supported as a supervisor adapter', got: %v", err)
	}
}

func TestGoslingBuildCmdRejectsUnknownRole(t *testing.T) {
	adapter := &GoslingAdapter{}
	_, err := adapter.BuildCmd(Role("invalid"), Request{Prompt: "x", Workdir: "/repo"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported role") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestGoslingParseResultCoder(t *testing.T) {
	adapter := &GoslingAdapter{}
	raw := []byte(`{
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "Say hello"
        }
      ]
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "text",
          "text": "Hello, world!"
        }
      ]
    }
  ]
}`)
	result, err := adapter.ParseResult(RoleCoder, raw)
	if err != nil {
		t.Fatalf("ParseResult() error = %v", err)
	}
	if result.Text != "Hello, world!" {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestGoslingParseResultAdversaryUnsupported(t *testing.T) {
	adapter := &GoslingAdapter{}
	_, err := adapter.ParseResult(RoleAdversary, []byte(`{"messages":[]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsOutputContractError(err) {
		t.Fatalf("expected output contract error, got %T", err)
	}
	if !strings.Contains(err.Error(), "not supported as an adversary adapter") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestGoslingDetectNonZeroExit(t *testing.T) {
	oldLookPath := execLookPath
	oldCommandContext := execCommandContext
	defer func() {
		execLookPath = oldLookPath
		execCommandContext = oldCommandContext
	}()

	execLookPath = func(file string) (string, error) {
		if file == "gosling" {
			return "/mock/bin/gosling", nil
		}
		return "", os.ErrNotExist
	}

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "gosling" {
			return exec.CommandContext(ctx, "false")
		}
		return exec.CommandContext(ctx, name, args...)
	}

	adapter := &GoslingAdapter{}
	info, err := adapter.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !info.Found {
		t.Fatal("expected Found = true")
	}
	if info.Runnable {
		t.Fatal("expected Runnable = false")
	}
	if !strings.Contains(info.Hint, "probe failed") {
		t.Fatalf("expected hint to mention probe failure, got: %q", info.Hint)
	}
}
