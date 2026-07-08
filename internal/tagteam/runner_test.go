package tagteam

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewValidation(t *testing.T) {
	review := Review{
		Verdict: "pass",
		Summary: "ok",
		Findings: []Finding{
			{Severity: "major", File: "main.go", Issue: "bug", Fix: "fix it"},
		},
	}
	if err := review.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPrepareReviewInput_UsesStdinWhenSupported(t *testing.T) {
	input := prepareReviewInput(&ClaudeAdapter{}, "diff --git a b", "/tmp/diff.patch")
	if !input.ViaStdin {
		t.Fatal("expected stdin input")
	}
	if len(input.Stdin) == 0 {
		t.Fatal("expected stdin bytes")
	}
}

func TestPrepareReviewInput_UsesFileReferenceForLargeDiff(t *testing.T) {
	diff := strings.Repeat("x", maxReviewInputBytes+1)
	input := prepareReviewInput(&ClaudeAdapter{}, diff, "/tmp/diff.patch")
	if input.ViaStdin {
		t.Fatal("did not expect stdin input")
	}
	if !strings.Contains(input.PromptRef, "/tmp/diff.patch") {
		t.Fatalf("prompt ref = %q", input.PromptRef)
	}
}

func TestPrepareReviewInput_UsesFileReferenceForLargeInlinePrompt(t *testing.T) {
	diff := strings.Repeat("x", maxInlineReviewPromptBytes+1)
	input := prepareReviewInput(&CodexAdapter{}, diff, "/tmp/diff.patch")
	if input.ViaStdin {
		t.Fatal("did not expect stdin input")
	}
	if !strings.Contains(input.PromptRef, "/tmp/diff.patch") {
		t.Fatalf("prompt ref = %q", input.PromptRef)
	}
}

func TestPreflightBranchModeCreatesBranch(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")

	_, cleanup, err := preflight(RunOptions{Workdir: repo, GitSafety: "branch"}, "2026-07-07T120000Z")
	if err != nil {
		t.Fatalf("preflight() error = %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	branch := strings.TrimSpace(runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "tagteam/2026-07-07T120000Z" {
		t.Fatalf("branch = %q", branch)
	}
}

func TestRunAdversaryDoesNotRetryInvocationFailures(t *testing.T) {
	app := NewApp(DefaultConfig())
	opts := RunOptions{
		Workdir:   t.TempDir(),
		Adversary: RoleTarget{Adapter: "missing"},
		Timeout:   time.Second,
	}
	_, _, _, err := app.runAdversary(context.Background(), opts, 1, opts.Workdir, filepath.Join(opts.Workdir, "schema.json"), "prompt", "HEAD", "diff", filepath.Join(opts.Workdir, "diff.patch"), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureGitignoreEntry_AppendsOnce(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignoreEntry(repo, ".tagteam/"); err != nil {
		t.Fatalf("ensureGitignoreEntry() error = %v", err)
	}
	if err := ensureGitignoreEntry(repo, ".tagteam/"); err != nil {
		t.Fatalf("ensureGitignoreEntry() second error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), ".tagteam/") != 1 {
		t.Fatalf(".gitignore contents = %q", string(data))
	}
}

func TestGitDirty_IgnoresTeamCLIRoot(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	if err := os.MkdirAll(filepath.Join(repo, ".tagteam", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".tagteam", "runs", "artifact.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := gitDirty(repo)
	if err != nil {
		t.Fatalf("gitDirty() error = %v", err)
	}
	if dirty {
		t.Fatal("expected .tagteam artifacts to be ignored")
	}
}

func TestComputeExitCode_UsesLastTestOnly(t *testing.T) {
	app := NewApp(DefaultConfig())
	final := FinalRun{
		Tests: []TestRun{
			{Passed: false},
			{Passed: true},
		},
	}
	if got := app.computeExitCode(final); got != ExitSuccess {
		t.Fatalf("exit code = %d", got)
	}
}

func TestRunAdapter_WritesMissingTranscript(t *testing.T) {
	app := NewApp(DefaultConfig())
	tmp := t.TempDir()
	outputPath := filepath.Join(tmp, "adversary-round-1.json")
	adapter := fakeAdapter{
		build: func(role Role, req Request) (*CommandSpec, error) {
			return &CommandSpec{
				Argv: []string{"sh", "-lc", "printf '{\"verdict\":\"pass\",\"summary\":\"ok\",\"findings\":[],\"test_suggestions\":[]}'"},
				Dir:  tmp,
			}, nil
		},
		parse: func(role Role, raw []byte) (Result, error) {
			var review Review
			if err := json.Unmarshal(raw, &review); err != nil {
				return Result{}, err
			}
			return Result{Raw: raw, Review: &review}, nil
		},
	}
	_, err := app.runAdapter(context.Background(), adapter, RoleAdversary, Request{
		Context:    context.Background(),
		OutputPath: outputPath,
		Timeout:    time.Second,
	}, false)
	if err != nil {
		t.Fatalf("runAdapter() error = %v", err)
	}
	if !fileExists(outputPath) {
		t.Fatal("expected transcript file to be written")
	}
}

func TestReadRunPrompt_FallsBackToMeta(t *testing.T) {
	runDir := t.TempDir()
	meta := Meta{Prompt: "review prompt"}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	prompt, err := readRunPrompt(runDir, "")
	if err != nil {
		t.Fatalf("readRunPrompt() error = %v", err)
	}
	if prompt != "review prompt" {
		t.Fatalf("prompt = %q", prompt)
	}
}

type fakeAdapter struct {
	build func(role Role, req Request) (*CommandSpec, error)
	parse func(role Role, raw []byte) (Result, error)
}

func (f fakeAdapter) ID() string { return "fake" }
func (f fakeAdapter) Detect(ctx context.Context) (VersionInfo, error) {
	return VersionInfo{Found: true, Runnable: true}, nil
}
func (f fakeAdapter) Capabilities() CapabilitySet { return CapabilitySet{} }
func (f fakeAdapter) BuildCmd(role Role, req Request) (*CommandSpec, error) {
	return f.build(role, req)
}
func (f fakeAdapter) ParseResult(role Role, raw []byte) (Result, error) {
	return f.parse(role, raw)
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
