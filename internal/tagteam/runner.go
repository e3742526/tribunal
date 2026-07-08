package tagteam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	Config Config
}

const maxReviewInputBytes = 10 * 1024 * 1024
const maxInlineReviewPromptBytes = 128 * 1024

type reviewInput struct {
	PromptRef string
	Stdin     []byte
	ViaStdin  bool
}

func NewApp(cfg Config) *App {
	return &App{Config: cfg}
}

func (a *App) Run(ctx context.Context, opts RunOptions) (FinalRun, error) {
	if strings.TrimSpace(opts.Prompt) == "" {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("prompt is required")}
	}
	return a.runLoop(ctx, opts, nil)
}

func (a *App) Review(ctx context.Context, opts RunOptions, prompt string) (FinalRun, error) {
	baseline, err := ensureGitRepo(opts.Workdir)
	if err != nil {
		return FinalRun{}, err
	}
	runID := newRunID()
	runDir, err := createRunDir(opts.Workdir, runID)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitAdapterFailure, Err: err}
	}
	if prompt == "" {
		if latestPrompt, _ := readLatestPrompt(opts.Workdir); latestPrompt != "" {
			prompt = latestPrompt
		} else {
			prompt = "Review the current working tree diff."
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "input.md"), []byte(prompt), 0o644); err != nil {
		return FinalRun{}, err
	}
	meta := Meta{
		RunID:         runID,
		Workdir:       opts.Workdir,
		Baseline:      baseline,
		Command:       "review",
		Prompt:        prompt,
		StartedAt:     time.Now().UTC(),
		Adapters:      map[string]string{"adversary": opts.Adversary.Adapter},
		Models:        map[string]string{"adversary": opts.Adversary.Model},
		ConfigSources: opts.ConfigSources,
	}
	if err := writeJSON(filepath.Join(runDir, "meta.json"), meta); err != nil {
		return FinalRun{}, err
	}
	schemaPath := filepath.Join(runDir, "review-schema.json")
	if err := os.WriteFile(schemaPath, []byte(ReviewSchema), 0o644); err != nil {
		return FinalRun{}, err
	}
	diff, err := gitDiffAgainst(opts.Workdir, baseline)
	if err != nil {
		return FinalRun{}, err
	}
	diffPath := filepath.Join(runDir, "diff-round-1.patch")
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		return FinalRun{}, err
	}
	review, cost, outputPath, err := a.runAdversary(ctx, opts, 1, runDir, schemaPath, prompt, baseline, diff, diffPath, "")
	if err != nil {
		return FinalRun{}, err
	}
	final := FinalRun{
		RunID:            runID,
		RunDir:           runDir,
		Workdir:          opts.Workdir,
		Baseline:         baseline,
		Verdict:          review.Verdict,
		Summary:          review.Summary,
		ExitCode:         ExitSuccess,
		RoundsRequested:  1,
		RoundsCompleted:  1,
		ChangedFiles:     changedFiles(opts.Workdir, baseline),
		LatestDiffPath:   diffPath,
		LatestReviewPath: outputPath,
		Review:           review,
		Costs:            map[string]float64{"adversary": cost},
		Adapters:         meta.Adapters,
		Models:           meta.Models,
		StartedAt:        meta.StartedAt,
		FinishedAt:       time.Now().UTC(),
	}
	if opts.FailOnReview && review.HasBlockingFindings() {
		final.ExitCode = ExitBlockingFindings
	}
	if err := a.persistFinal(opts.Workdir, final); err != nil {
		return FinalRun{}, err
	}
	if final.ExitCode != ExitSuccess {
		return final, &ExitError{Code: final.ExitCode, Err: fmt.Errorf("review found blocking issues")}
	}
	return final, nil
}

func (a *App) Fix(ctx context.Context, opts RunOptions) (FinalRun, error) {
	latest, err := readLatest(opts.Workdir)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: err}
	}
	final, err := readFinal(latest.FinalPath)
	if err != nil {
		return FinalRun{}, err
	}
	if final.Review == nil {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("latest run has no saved review")}
	}
	prompt, err := readRunPrompt(final.RunDir, "")
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitAdapterFailure, Err: err}
	}
	opts.Prompt = prompt
	opts.Baseline = final.Baseline
	opts.SkipDirtyCheck = true
	return a.runLoop(ctx, opts, final.Review)
}

func (a *App) Doctor(ctx context.Context, opts RunOptions) (map[string]VersionInfo, error) {
	registry := Registry(a.Config, opts)
	status := map[string]VersionInfo{}
	for _, key := range []string{"codex", "codex-oss", "claude", "agy"} {
		info, err := registry[key].Detect(ctx)
		if err != nil {
			return nil, err
		}
		status[key] = info
	}
	for _, target := range []RoleTarget{opts.Coder, opts.Adversary} {
		if target.Adapter == "" {
			continue
		}
		info, ok := status[target.Adapter]
		if !ok || !info.Found || !info.Runnable {
			hint := ""
			if ok {
				hint = info.Hint
			}
			return status, &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("%s not runnable; try %s", target.Adapter, hint)}
		}
	}
	return status, nil
}

func (a *App) runLoop(ctx context.Context, opts RunOptions, initialReview *Review) (FinalRun, error) {
	runID := newRunID()
	baseline, cleanup, err := preflight(opts, runID)
	if err != nil {
		return FinalRun{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	runDir, err := createRunDir(opts.Workdir, runID)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitAdapterFailure, Err: err}
	}
	if err := os.WriteFile(filepath.Join(runDir, "input.md"), []byte(opts.Prompt), 0o644); err != nil {
		return FinalRun{}, err
	}
	schemaPath := filepath.Join(runDir, "review-schema.json")
	if err := os.WriteFile(schemaPath, []byte(ReviewSchema), 0o644); err != nil {
		return FinalRun{}, err
	}
	meta := Meta{
		RunID:         runID,
		Workdir:       opts.Workdir,
		Baseline:      baseline,
		Command:       "run",
		Prompt:        opts.Prompt,
		StartedAt:     time.Now().UTC(),
		Adapters:      map[string]string{"coder": opts.Coder.Adapter, "adversary": opts.Adversary.Adapter},
		Models:        map[string]string{"coder": opts.Coder.Model, "adversary": opts.Adversary.Model},
		ConfigSources: opts.ConfigSources,
	}
	if err := writeJSON(filepath.Join(runDir, "meta.json"), meta); err != nil {
		return FinalRun{}, err
	}

	registry := Registry(a.Config, opts)
	coder, ok := registry[opts.Coder.Adapter]
	if !ok {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("unknown coder adapter %q", opts.Coder.Adapter)}
	}
	adversary, ok := registry[opts.Adversary.Adapter]
	if !ok {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("unknown adversary adapter %q", opts.Adversary.Adapter)}
	}
	if err := checkAdapters(ctx, coder, adversary); err != nil {
		return FinalRun{}, err
	}

	final := FinalRun{
		RunID:           runID,
		RunDir:          runDir,
		Workdir:         opts.Workdir,
		Baseline:        baseline,
		RoundsRequested: opts.Rounds,
		Costs:           map[string]float64{},
		Adapters:        meta.Adapters,
		Models:          meta.Models,
		StartedAt:       meta.StartedAt,
	}

	var sessionID string
	var latestReview Review
	for round := 1; round <= opts.Rounds; round++ {
		coderPrompt := BuildCoderPrompt(opts.Workdir, opts.Prompt)
		if round == 1 && initialReview != nil {
			diff, err := gitDiffAgainst(opts.Workdir, baseline)
			if err != nil {
				return final, err
			}
			coderPrompt = BuildFixPrompt(round, opts.Prompt, diff, *initialReview)
		} else if round > 1 {
			diff, err := gitDiffAgainst(opts.Workdir, baseline)
			if err != nil {
				return final, err
			}
			coderPrompt = BuildFixPrompt(round, opts.Prompt, diff, latestReview)
		}
		coderOutputPath := filepath.Join(runDir, fmt.Sprintf("coder-round-%d.md", round))
		coderResult, err := a.runAdapter(ctx, coder, RoleCoder, Request{
			Context:      ctx,
			Prompt:       coderPrompt,
			SystemPrompt: coderSystemPrompt,
			Model:        opts.Coder.Model,
			Workdir:      opts.Workdir,
			RunDir:       runDir,
			OutputPath:   coderOutputPath,
			ResumeID:     sessionID,
			Timeout:      opts.Timeout,
		}, opts.DryRun)
		if err != nil {
			return final, err
		}
		final.Costs["coder"] += coderResult.CostUSD
		if coderResult.SessionID != "" {
			sessionID = coderResult.SessionID
		}

		diff, err := gitDiffAgainst(opts.Workdir, baseline)
		if err != nil {
			return final, err
		}
		diffPath := filepath.Join(runDir, fmt.Sprintf("diff-round-%d.patch", round))
		if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
			return final, err
		}
		final.LatestDiffPath = diffPath

		testOutput := ""
		if opts.TestCmd != "" && !opts.NoTest {
			testPath := filepath.Join(runDir, fmt.Sprintf("test-round-%d.txt", round))
			testRun, err := runTestCommand(ctx, opts.Workdir, opts.TestCmd, opts.Timeout, testPath, opts.DryRun)
			if err != nil {
				return final, err
			}
			final.Tests = append(final.Tests, testRun)
			testOutput = testRun.Output
		}

		review, cost, reviewPath, err := a.runAdversary(ctx, opts, round, runDir, schemaPath, opts.Prompt, baseline, diff, diffPath, testOutput)
		if err != nil {
			return final, err
		}
		final.Costs["adversary"] += cost
		final.RoundsCompleted = round
		final.Review = review
		final.LatestReviewPath = reviewPath
		latestReview = *review

		if review.Verdict == "pass" {
			final.Verdict = "pass"
			final.Summary = review.Summary
			break
		}
		if review.OnlyMinorOrNit() {
			final.Verdict = review.Verdict
			final.Summary = review.Summary
			break
		}
		if round == opts.Rounds {
			final.Verdict = review.Verdict
			final.Summary = review.Summary
		}
	}

	final.ChangedFiles = changedFiles(opts.Workdir, baseline)
	final.FinishedAt = time.Now().UTC()
	final.ExitCode = a.computeExitCode(final)
	if err := a.persistFinal(opts.Workdir, final); err != nil {
		return final, err
	}
	if final.ExitCode != ExitSuccess {
		return final, &ExitError{Code: final.ExitCode, Err: fmt.Errorf("run completed with exit code %d", final.ExitCode)}
	}
	return final, nil
}

func (a *App) runAdversary(ctx context.Context, opts RunOptions, round int, runDir, schemaPath, prompt, baseline, diff, diffPath, testOutput string) (*Review, float64, string, error) {
	outputPath := filepath.Join(runDir, fmt.Sprintf("adversary-round-%d.json", round))
	registry := Registry(a.Config, opts)
	adversary, ok := registry[opts.Adversary.Adapter]
	if !ok {
		return nil, 0, "", &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("unknown adversary adapter %q", opts.Adversary.Adapter)}
	}
	if err := checkAdapters(ctx, adversary); err != nil {
		return nil, 0, "", err
	}
	input := prepareReviewInput(adversary, diff, diffPath)
	reviewPrompt := BuildAdversaryPrompt(prompt, baseline, input.PromptRef, safeTestOutput(testOutput), input.ViaStdin)
	if !adversary.Capabilities().SupportsSchema {
		reviewPrompt += "\n\nJSON schema:\n" + ReviewSchema
	}
	req := Request{
		Context:     ctx,
		Prompt:      reviewPrompt,
		Model:       opts.Adversary.Model,
		Workdir:     opts.Workdir,
		RunDir:      runDir,
		OutputPath:  outputPath,
		SchemaPath:  schemaPath,
		Passthrough: opts.ClaudeArgs,
		Timeout:     opts.Timeout,
	}
	req.Stdin = input.Stdin
	result, err := a.runAdapter(ctx, adversary, RoleAdversary, req, opts.DryRun)
	if err != nil {
		if !IsOutputContractError(err) {
			return nil, 0, "", err
		}
		req.Prompt = req.Prompt + "\n\nValidation error from the previous response:\n" + err.Error() + "\n\nReturn JSON exactly matching the schema."
		result, err = a.runAdapter(ctx, adversary, RoleAdversary, req, opts.DryRun)
		if err != nil {
			if IsOutputContractError(err) {
				return nil, 0, "", &ExitError{Code: ExitAdapterFailure, Err: fmt.Errorf("adversary output failed validation after retry: %w", err)}
			}
			return nil, 0, "", err
		}
		return result.Review, result.CostUSD, outputPath, nil
	}
	return result.Review, result.CostUSD, outputPath, nil
}

func (a *App) runAdapter(ctx context.Context, adapter Adapter, role Role, req Request, dryRun bool) (Result, error) {
	spec, err := adapter.BuildCmd(role, req)
	if err != nil {
		return Result{}, &ExitError{Code: ExitInvalidArguments, Err: err}
	}
	if dryRun {
		payload, _ := json.MarshalIndent(spec, "", "  ")
		result := Result{Text: string(payload), Command: spec.Argv}
		if role == RoleAdversary {
			result.Review = &Review{
				Verdict:         "pass",
				Summary:         "dry-run",
				Findings:        []Finding{},
				TestSuggestions: []string{},
			}
		}
		return result, nil
	}
	runCtx := req.Context
	if runCtx == nil {
		runCtx = ctx
	}
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, req.Timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{}, &ExitError{Code: ExitAdapterFailure, Err: fmt.Errorf("%s failed: %s", adapter.ID(), msg)}
	}
	raw := stdout.Bytes()
	if req.OutputPath != "" && fileExists(req.OutputPath) {
		var readErr error
		raw, readErr = os.ReadFile(req.OutputPath)
		if readErr != nil {
			return Result{}, readErr
		}
	}
	if len(raw) == 0 {
		raw = stdout.Bytes()
	}
	if req.OutputPath != "" && !fileExists(req.OutputPath) {
		if writeErr := os.WriteFile(req.OutputPath, raw, 0o644); writeErr != nil {
			return Result{}, writeErr
		}
	}
	result, err := adapter.ParseResult(role, raw)
	if err != nil {
		return Result{}, err
	}
	result.Command = spec.Argv
	return result, nil
}

func (a *App) computeExitCode(final FinalRun) int {
	if len(final.Tests) > 0 && !final.Tests[len(final.Tests)-1].Passed {
		return ExitTestsFailed
	}
	if final.Review != nil && final.Review.HasBlockingFindings() {
		return ExitBlockingFindings
	}
	return ExitSuccess
}

func (a *App) persistFinal(workdir string, final FinalRun) error {
	finalPath := filepath.Join(final.RunDir, "final.json")
	if err := writeJSON(finalPath, final); err != nil {
		return err
	}
	latest := LatestRun{
		RunID:     final.RunID,
		RunDir:    final.RunDir,
		FinalPath: finalPath,
		Verdict:   final.Verdict,
		ExitCode:  final.ExitCode,
		UpdatedAt: time.Now().UTC(),
	}
	return writeJSON(filepath.Join(workdir, ".tagteam", "latest.json"), latest)
}

func preflight(opts RunOptions, runID string) (string, func(), error) {
	baseline := opts.Baseline
	if baseline == "" {
		var err error
		baseline, err = ensureGitRepo(opts.Workdir)
		if err != nil {
			return "", nil, err
		}
	}
	if opts.AllowDirty || opts.GitSafety == "allow-dirty" {
		return baseline, nil, nil
	}
	if opts.SkipDirtyCheck {
		return baseline, nil, nil
	}
	if opts.GitSafety == "branch" {
		if err := gitCreateBranch(opts.Workdir, "tagteam/"+runID); err != nil {
			return "", nil, err
		}
		return baseline, nil, nil
	}
	dirty, err := gitDirty(opts.Workdir)
	if err != nil {
		return "", nil, err
	}
	if !dirty {
		return baseline, nil, nil
	}
	if opts.Autostash || opts.GitSafety == "autostash" {
		stashRef, err := gitAutostash(opts.Workdir)
		if err != nil {
			return "", nil, err
		}
		return baseline, func() {
			_, _ = runCommand(context.Background(), opts.Workdir, "git", "stash", "pop", stashRef)
		}, nil
	}
	return "", nil, &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("worktree is dirty; use --allow-dirty or --autostash")}
}

func ensureGitRepo(workdir string) (string, error) {
	out, err := runCommand(context.Background(), workdir, "git", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("workdir is not a git repo or has no HEAD: %w", err)}
	}
	return strings.TrimSpace(out), nil
}

func checkAdapters(ctx context.Context, adapters ...Adapter) error {
	for _, adapter := range adapters {
		if adapter == nil {
			return &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("adapter is not configured")}
		}
		info, err := adapter.Detect(ctx)
		if err != nil {
			return &ExitError{Code: ExitPreflightFailed, Err: err}
		}
		if !info.Found || !info.Runnable {
			return &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("%s not runnable; try %s", adapter.ID(), info.Hint)}
		}
	}
	return nil
}

func runTestCommand(ctx context.Context, workdir, testCmd string, timeout time.Duration, outputPath string, dryRun bool) (TestRun, error) {
	if dryRun {
		return TestRun{Command: testCmd, Passed: true, Output: "dry-run"}, nil
	}
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-lc", testCmd)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	testRun := TestRun{
		Command: testCmd,
		Output:  string(out),
		Passed:  err == nil,
	}
	_ = os.WriteFile(outputPath, out, 0o644)
	return testRun, nil
}

func createRunDir(workdir, runID string) (string, error) {
	rootDir := filepath.Join(workdir, ".tagteam")
	if err := os.MkdirAll(filepath.Join(rootDir, "runs"), 0o755); err != nil {
		return "", err
	}
	if err := ensureRunRootIgnore(rootDir); err != nil {
		return "", err
	}
	root := filepath.Join(rootDir, "runs", runID)
	if err := os.Mkdir(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readLatest(workdir string) (LatestRun, error) {
	var latest LatestRun
	data, err := os.ReadFile(filepath.Join(workdir, ".tagteam", "latest.json"))
	if err != nil {
		return LatestRun{}, err
	}
	if err := json.Unmarshal(data, &latest); err != nil {
		return LatestRun{}, err
	}
	return latest, nil
}

func readFinal(path string) (FinalRun, error) {
	var final FinalRun
	data, err := os.ReadFile(path)
	if err != nil {
		return FinalRun{}, err
	}
	if err := json.Unmarshal(data, &final); err != nil {
		return FinalRun{}, err
	}
	return final, nil
}

func readMeta(path string) (Meta, error) {
	var meta Meta
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func readLatestPrompt(workdir string) (string, error) {
	latest, err := readLatest(workdir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(latest.RunDir, "input.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func changedFiles(workdir, baseline string) []string {
	out, err := runCommand(context.Background(), workdir, "git", "diff", "--name-only", baseline)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func gitDiffAgainst(workdir, baseline string) (string, error) {
	return runCommand(context.Background(), workdir, "git", "diff", baseline)
}

func gitDirty(workdir string) (bool, error) {
	out, err := runCommand(context.Background(), workdir, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, ".tagteam/") || strings.HasSuffix(line, ".tagteam") {
			continue
		}
		return true, nil
	}
	return false, nil
}

func gitAutostash(workdir string) (string, error) {
	if _, err := runCommand(context.Background(), workdir, "git", "stash", "push", "-u", "-m", "tagteam-autostash"); err != nil {
		return "", &ExitError{Code: ExitPreflightFailed, Err: err}
	}
	return "stash@{0}", nil
}

func gitCreateBranch(workdir, branch string) error {
	if _, err := runCommand(context.Background(), workdir, "git", "switch", "-c", branch); err == nil {
		return nil
	}
	if _, err := runCommand(context.Background(), workdir, "git", "checkout", "-b", branch); err != nil {
		return &ExitError{Code: ExitPreflightFailed, Err: err}
	}
	return nil
}

func runCommand(ctx context.Context, workdir, binary string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", binary, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func safeTestOutput(output string) string {
	if strings.TrimSpace(output) == "" {
		return "(no tests run)"
	}
	return output
}

func prepareReviewInput(adversary Adapter, diff, diffPath string) reviewInput {
	diffBytes := []byte(diff)
	if adversary.Capabilities().SupportsStdin && len(diffBytes) <= maxReviewInputBytes {
		return reviewInput{
			Stdin:    diffBytes,
			ViaStdin: true,
		}
	}
	if len(diffBytes) <= maxInlineReviewPromptBytes {
		return reviewInput{
			PromptRef: diff,
		}
	}
	if diffPath != "" {
		return reviewInput{
			PromptRef: fmt.Sprintf("Diff is stored at %s. Read that file from the workspace.", diffPath),
		}
	}
	return reviewInput{
		PromptRef: diff,
	}
}

func countExisting(dir, pattern string) int {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return 0
	}
	return len(matches)
}

func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func ensureGitignoreEntry(workdir, entry string) error {
	gitignorePath := filepath.Join(workdir, ".gitignore")
	if !fileExists(gitignorePath) {
		return os.WriteFile(gitignorePath, []byte(entry+"\n"), 0o644)
	}
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	contents := strings.TrimRight(string(data), "\n")
	if contents == "" {
		contents = entry
	} else {
		contents += "\n" + entry
	}
	contents += "\n"
	return os.WriteFile(gitignorePath, []byte(contents), 0o644)
}

func ensureRunRootIgnore(rootDir string) error {
	gitignorePath := filepath.Join(rootDir, ".gitignore")
	contents := "*\n!.gitignore\n"
	if fileExists(gitignorePath) {
		data, err := os.ReadFile(gitignorePath)
		if err != nil {
			return err
		}
		if string(data) == contents {
			return nil
		}
	}
	return os.WriteFile(gitignorePath, []byte(contents), 0o644)
}

func newRunID() string {
	return time.Now().UTC().Format("2006-01-02T150405.000000000Z")
}

func readRunPrompt(runDir, fallback string) (string, error) {
	inputPath := filepath.Join(runDir, "input.md")
	if fileExists(inputPath) {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	metaPath := filepath.Join(runDir, "meta.json")
	if fileExists(metaPath) {
		meta, err := readMeta(metaPath)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(meta.Prompt) != "" {
			return meta.Prompt, nil
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("run prompt not found in %s", runDir)
}
