package tagteam

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func Registry(cfg Config, opts RunOptions) map[string]Adapter {
	return map[string]Adapter{
		"codex": &CodexAdapter{
			IDValue:      "codex",
			DefaultModel: cfg.Adapters.Codex.DefaultModel,
			ExtraArgs:    opts.CodexArgs,
		},
		"codex-oss": &CodexAdapter{
			IDValue:      "codex-oss",
			DefaultModel: cfg.Adapters.CodexOSS.DefaultModel,
			ExtraArgs:    opts.CodexArgs,
			OSS:          true,
		},
		"claude": &ClaudeAdapter{
			DefaultModel:      cfg.Adapters.Claude.DefaultModel,
			CoderAllowedTools: cfg.Adapters.Claude.CoderAllowedTools,
			Bare:              cfg.Adapters.Claude.Bare,
			ExtraArgs:         opts.ClaudeArgs,
		},
		"agy": &AgyAdapter{
			DefaultModel: cfg.Adapters.Agy.DefaultModel,
			ExtraArgs:    opts.AgyArgs,
		},
	}
}

type CodexAdapter struct {
	IDValue      string
	DefaultModel string
	ExtraArgs    []string
	OSS          bool
}

func (a *CodexAdapter) ID() string {
	return a.IDValue
}

func (a *CodexAdapter) Capabilities() CapabilitySet {
	return CapabilitySet{SupportsSchema: true}
}

func (a *CodexAdapter) Detect(ctx context.Context) (VersionInfo, error) {
	return detectBinary(ctx, "codex", []string{"--version"}, "codex login")
}

func (a *CodexAdapter) BuildCmd(role Role, req Request) (*CommandSpec, error) {
	model := req.Model
	if model == "" {
		model = a.DefaultModel
	}
	argv := []string{"codex", "exec", "-C", req.Workdir}
	if a.OSS {
		argv = append(argv, "--oss")
	}
	switch role {
	case RoleCoder:
		argv = append(argv, "-s", "workspace-write")
	case RoleAdversary:
		argv = append(argv, "-s", "read-only")
	default:
		return nil, fmt.Errorf("unsupported role %q", role)
	}
	if model != "" {
		argv = append(argv, "-m", model)
	}
	if role == RoleAdversary && req.SchemaPath != "" {
		argv = append(argv, "--output-schema", req.SchemaPath)
	}
	if req.OutputPath != "" {
		argv = append(argv, "-o", req.OutputPath)
	}
	argv = append(argv, a.ExtraArgs...)
	argv = append(argv, req.Prompt)
	return &CommandSpec{Argv: argv, Dir: req.Workdir, Output: req.OutputPath}, nil
}

func (a *CodexAdapter) ParseResult(role Role, raw []byte) (Result, error) {
	result := Result{Raw: raw, Text: strings.TrimSpace(string(raw))}
	if role == RoleAdversary {
		var review Review
		if err := json.Unmarshal(raw, &review); err != nil {
			return Result{}, &OutputContractError{Err: fmt.Errorf("decode codex adversary JSON: %w", err)}
		}
		if err := review.Validate(); err != nil {
			return Result{}, &OutputContractError{Err: err}
		}
		result.Review = &review
		result.Text = review.Summary
	}
	return result, nil
}

type ClaudeAdapter struct {
	DefaultModel      string
	CoderAllowedTools []string
	Bare              bool
	ExtraArgs         []string
}

func (a *ClaudeAdapter) ID() string {
	return "claude"
}

func (a *ClaudeAdapter) Capabilities() CapabilitySet {
	return CapabilitySet{SupportsSchema: true, SupportsResume: true, SupportsStdin: true}
}

func (a *ClaudeAdapter) Detect(ctx context.Context) (VersionInfo, error) {
	hint := "claude login"
	if a.Bare {
		hint = "set ANTHROPIC_API_KEY or disable adapters.claude.bare"
	}
	return detectBinary(ctx, "claude", []string{"--version"}, hint)
}

func (a *ClaudeAdapter) BuildCmd(role Role, req Request) (*CommandSpec, error) {
	model := req.Model
	if model == "" {
		model = a.DefaultModel
	}
	argv := []string{"claude", "-p", req.Prompt}
	if model != "" {
		argv = append(argv, "--model", model)
	}
	if a.Bare {
		argv = append(argv, "--bare")
	}
	switch role {
	case RoleCoder:
		argv = append(argv,
			"--permission-mode", "acceptEdits",
			"--allowedTools", strings.Join(a.CoderAllowedTools, ","),
		)
		if req.SystemPrompt != "" {
			argv = append(argv, "--append-system-prompt", req.SystemPrompt)
		}
	case RoleAdversary:
		argv = append(argv,
			"--permission-mode", "dontAsk",
			"--allowedTools", "Read,Glob,Grep,Bash(git diff *),Bash(git log *),Bash(git status *)",
		)
		if req.SchemaPath != "" {
			schemaBytes, err := osReadFile(req.SchemaPath)
			if err != nil {
				return nil, err
			}
			argv = append(argv, "--json-schema", string(schemaBytes))
		}
	default:
		return nil, fmt.Errorf("unsupported role %q", role)
	}
	if req.ResumeID != "" {
		argv = append(argv, "--resume", req.ResumeID)
	}
	argv = append(argv, "--output-format", "json")
	argv = append(argv, a.ExtraArgs...)
	return &CommandSpec{Argv: argv, Dir: req.Workdir, Stdin: req.Stdin, Output: req.OutputPath}, nil
}

type claudeEnvelope struct {
	Result           string          `json:"result"`
	SessionID        string          `json:"session_id"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func (a *ClaudeAdapter) ParseResult(role Role, raw []byte) (Result, error) {
	var envelope claudeEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Result{}, fmt.Errorf("decode claude JSON: %w", err)
	}
	result := Result{
		Raw:       raw,
		Text:      strings.TrimSpace(envelope.Result),
		SessionID: envelope.SessionID,
		CostUSD:   envelope.TotalCostUSD,
	}
	if role == RoleAdversary {
		var review Review
		reviewRaw := envelope.StructuredOutput
		if len(reviewRaw) == 0 || string(reviewRaw) == "null" {
			reviewRaw = []byte(envelope.Result)
		}
		if err := json.Unmarshal(reviewRaw, &review); err != nil {
			extracted, extractErr := extractJSONObject(reviewRaw)
			if extractErr != nil {
				return Result{}, &OutputContractError{Err: fmt.Errorf("decode claude review JSON: %w", err)}
			}
			if err := json.Unmarshal(extracted, &review); err != nil {
				return Result{}, &OutputContractError{Err: fmt.Errorf("decode claude review JSON: %w", err)}
			}
		}
		if err := review.Validate(); err != nil {
			return Result{}, &OutputContractError{Err: err}
		}
		result.Review = &review
		result.Text = review.Summary
	}
	return result, nil
}

type AgyAdapter struct {
	DefaultModel string
	ExtraArgs    []string
}

func (a *AgyAdapter) ID() string {
	return "agy"
}

func (a *AgyAdapter) Capabilities() CapabilitySet {
	return CapabilitySet{}
}

func (a *AgyAdapter) Detect(ctx context.Context) (VersionInfo, error) {
	path, err := exec.LookPath("agy")
	if err != nil {
		return VersionInfo{Found: false, Auth: "unknown", Hint: "install agy", Runnable: false}, nil
	}
	cmd := exec.CommandContext(ctx, "agy", "--help")
	if err := cmd.Run(); err != nil {
		return VersionInfo{
			Found:    true,
			Version:  "installed",
			Auth:     "unknown",
			Binary:   path,
			Hint:     "run agy login or configure agy",
			Runnable: false,
		}, nil
	}
	return VersionInfo{
		Found:    true,
		Version:  "installed",
		Auth:     "unknown",
		Binary:   path,
		Hint:     "run agy login or configure agy",
		Runnable: true,
	}, nil
}

func (a *AgyAdapter) BuildCmd(role Role, req Request) (*CommandSpec, error) {
	model := req.Model
	if model == "" {
		model = a.DefaultModel
	}
	argv := []string{"agy", "--print", req.Prompt}
	if model != "" {
		argv = append(argv, "--model", model)
	}
	if req.Timeout > 0 {
		argv = append(argv, "--print-timeout", req.Timeout.String())
	}
	switch role {
	case RoleCoder:
		argv = append(argv, "--dangerously-skip-permissions")
	case RoleAdversary:
		argv = append(argv, "--sandbox")
	default:
		return nil, fmt.Errorf("unsupported role %q", role)
	}
	argv = append(argv, a.ExtraArgs...)
	return &CommandSpec{Argv: argv, Dir: req.Workdir, Output: req.OutputPath}, nil
}

func (a *AgyAdapter) ParseResult(role Role, raw []byte) (Result, error) {
	result := Result{Raw: raw, Text: strings.TrimSpace(string(raw))}
	if role == RoleAdversary {
		var review Review
		reviewRaw := raw
		if err := json.Unmarshal(reviewRaw, &review); err != nil {
			extracted, extractErr := extractJSONObject(reviewRaw)
			if extractErr != nil {
				return Result{}, &OutputContractError{Err: fmt.Errorf("decode agy review JSON: %w", err)}
			}
			if err := json.Unmarshal(extracted, &review); err != nil {
				return Result{}, &OutputContractError{Err: fmt.Errorf("decode agy review JSON: %w", err)}
			}
		}
		if err := review.Validate(); err != nil {
			return Result{}, &OutputContractError{Err: err}
		}
		result.Review = &review
		result.Text = review.Summary
	}
	return result, nil
}

func detectBinary(ctx context.Context, binary string, versionArgs []string, hint string) (VersionInfo, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return VersionInfo{Found: false, Auth: "unknown", Hint: hint, Runnable: false}, nil
	}
	cmd := exec.CommandContext(ctx, binary, versionArgs...)
	out, err := cmd.CombinedOutput()
	version := strings.TrimSpace(string(out))
	if err != nil && version == "" {
		version = "unknown"
	}
	return VersionInfo{
		Found:    true,
		Version:  version,
		Auth:     "unknown",
		Binary:   path,
		Hint:     hint,
		Runnable: true,
	}, nil
}

func extractJSONObject(raw []byte) ([]byte, error) {
	text := string(raw)
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return nil, fmt.Errorf("no JSON object found")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []byte(text[start : i+1]), nil
			}
		}
	}
	return nil, fmt.Errorf("unterminated JSON object")
}
