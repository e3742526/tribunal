package tagteam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleCoder     Role = "coder"
	RoleAdversary Role = "adversary"
)

const (
	ExitSuccess          = 0
	ExitBlockingFindings = 1
	ExitTestsFailed      = 2
	ExitAdapterFailure   = 3
	ExitInvalidArguments = 4
	ExitPreflightFailed  = 5
)

type ExitError struct {
	Code int
	Err  error
}

type OutputContractError struct {
	Err error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *OutputContractError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *OutputContractError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitAdapterFailure
}

func IsOutputContractError(err error) bool {
	var contractErr *OutputContractError
	return errors.As(err, &contractErr)
}

type CapabilitySet struct {
	SupportsSchema bool
	SupportsResume bool
	SupportsStdin  bool
}

type VersionInfo struct {
	Found    bool   `json:"found"`
	Version  string `json:"version,omitempty"`
	Auth     string `json:"auth,omitempty"`
	Binary   string `json:"binary,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Runnable bool   `json:"runnable"`
}

type Request struct {
	Context      context.Context
	Prompt       string
	SystemPrompt string
	Model        string
	Workdir      string
	RunDir       string
	OutputPath   string
	SchemaPath   string
	Timeout      time.Duration
	Passthrough  []string
	ResumeID     string
	Stdin        []byte
}

type Adapter interface {
	ID() string
	Detect(ctx context.Context) (VersionInfo, error)
	Capabilities() CapabilitySet
	BuildCmd(role Role, req Request) (*CommandSpec, error)
	ParseResult(role Role, raw []byte) (Result, error)
}

type CommandSpec struct {
	Argv   []string `json:"argv"`
	Dir    string   `json:"dir"`
	Stdin  []byte   `json:"-"`
	Output string   `json:"output"`
}

type Result struct {
	Text      string   `json:"text,omitempty"`
	Review    *Review  `json:"review,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	CostUSD   float64  `json:"cost_usd,omitempty"`
	Raw       []byte   `json:"-"`
	Command   []string `json:"command,omitempty"`
}

type Review struct {
	Verdict         string    `json:"verdict"`
	Summary         string    `json:"summary"`
	Findings        []Finding `json:"findings"`
	TestSuggestions []string  `json:"test_suggestions,omitempty"`
}

type Finding struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Issue    string `json:"issue"`
	Fix      string `json:"fix"`
}

func (r Review) HasBlockingFindings() bool {
	for _, finding := range r.Findings {
		if finding.Severity == "blocker" || finding.Severity == "major" {
			return true
		}
	}
	return false
}

func (r Review) OnlyMinorOrNit() bool {
	if len(r.Findings) == 0 {
		return true
	}
	for _, finding := range r.Findings {
		if finding.Severity != "minor" && finding.Severity != "nit" {
			return false
		}
	}
	return true
}

func (r Review) Validate() error {
	if r.Verdict != "pass" && r.Verdict != "needs_changes" {
		return fmt.Errorf("invalid verdict %q", r.Verdict)
	}
	for i, finding := range r.Findings {
		switch finding.Severity {
		case "blocker", "major", "minor", "nit":
		default:
			return fmt.Errorf("finding %d has invalid severity %q", i, finding.Severity)
		}
		if strings.TrimSpace(finding.File) == "" {
			return fmt.Errorf("finding %d missing file", i)
		}
		if strings.TrimSpace(finding.Issue) == "" {
			return fmt.Errorf("finding %d missing issue", i)
		}
		if strings.TrimSpace(finding.Fix) == "" {
			return fmt.Errorf("finding %d missing fix", i)
		}
	}
	if r.Verdict == "pass" && r.HasBlockingFindings() {
		return fmt.Errorf("pass verdict cannot include blocker or major findings")
	}
	return nil
}

type RoleTarget struct {
	Adapter string `json:"adapter"`
	Model   string `json:"model,omitempty"`
}

func ParseRoleTarget(raw string) (RoleTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RoleTarget{}, fmt.Errorf("empty adapter target")
	}
	parts := strings.SplitN(raw, ":", 2)
	target := RoleTarget{Adapter: strings.TrimSpace(parts[0])}
	if len(parts) == 2 {
		target.Model = strings.TrimSpace(parts[1])
	}
	if target.Adapter == "" {
		return RoleTarget{}, fmt.Errorf("invalid adapter target %q", raw)
	}
	return target, nil
}

type Config struct {
	Defaults DefaultsConfig           `toml:"defaults"`
	Profiles map[string]ProfileConfig `toml:"profiles"`
	Adapters AdapterConfigSet         `toml:"adapters"`
}

type DefaultsConfig struct {
	Coder     string `toml:"coder"`
	Adversary string `toml:"adversary"`
	Rounds    int    `toml:"rounds"`
	Test      string `toml:"test"`
	GitSafety string `toml:"git_safety"`
}

type ProfileConfig struct {
	Coder     string `toml:"coder"`
	Adversary string `toml:"adversary"`
	Rounds    int    `toml:"rounds"`
	Test      string `toml:"test"`
}

type AdapterConfigSet struct {
	Codex    CodexConfig  `toml:"codex"`
	Claude   ClaudeConfig `toml:"claude"`
	CodexOSS CodexConfig  `toml:"codex-oss"`
	Agy      AgyConfig    `toml:"agy"`
}

type CodexConfig struct {
	DefaultModel string   `toml:"default_model"`
	ExtraArgs    []string `toml:"extra_args"`
}

type ClaudeConfig struct {
	DefaultModel      string   `toml:"default_model"`
	CoderAllowedTools []string `toml:"coder_allowed_tools"`
	ExtraArgs         []string `toml:"extra_args"`
	Bare              bool     `toml:"bare"`
}

type AgyConfig struct {
	DefaultModel string   `toml:"default_model"`
	ExtraArgs    []string `toml:"extra_args"`
}

type FlagInputs struct {
	Coder         string
	Adversary     string
	Profile       string
	Workdir       string
	Rounds        int
	Test          string
	NoTest        bool
	JSON          bool
	DryRun        bool
	ShowReview    bool
	FailOnReview  bool
	AllowDirty    bool
	Autostash     bool
	Timeout       time.Duration
	Quiet         bool
	Verbose       bool
	CodexArgsRaw  string
	ClaudeArgsRaw string
	AgyArgsRaw    string
}

type RunOptions struct {
	Prompt         string
	Workdir        string
	Coder          RoleTarget
	Adversary      RoleTarget
	Rounds         int
	TestCmd        string
	NoTest         bool
	JSON           bool
	DryRun         bool
	ShowReview     bool
	FailOnReview   bool
	AllowDirty     bool
	Autostash      bool
	Timeout        time.Duration
	Quiet          bool
	Verbose        bool
	GitSafety      string
	CodexArgs      []string
	ClaudeArgs     []string
	AgyArgs        []string
	ConfigSources  []string
	Baseline       string
	SkipDirtyCheck bool
}

type Meta struct {
	RunID         string            `json:"run_id"`
	Workdir       string            `json:"workdir"`
	Baseline      string            `json:"baseline"`
	Command       string            `json:"command"`
	Prompt        string            `json:"prompt"`
	StartedAt     time.Time         `json:"started_at"`
	Adapters      map[string]string `json:"adapters"`
	Models        map[string]string `json:"models"`
	ConfigSources []string          `json:"config_sources,omitempty"`
}

type TestRun struct {
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Passed  bool   `json:"passed"`
}

type FinalRun struct {
	RunID            string             `json:"run_id"`
	RunDir           string             `json:"run_dir"`
	Workdir          string             `json:"workdir"`
	Baseline         string             `json:"baseline"`
	Verdict          string             `json:"verdict"`
	Summary          string             `json:"summary"`
	ExitCode         int                `json:"exit_code"`
	RoundsRequested  int                `json:"rounds_requested"`
	RoundsCompleted  int                `json:"rounds_completed"`
	ChangedFiles     []string           `json:"changed_files,omitempty"`
	LatestDiffPath   string             `json:"latest_diff_path,omitempty"`
	LatestReviewPath string             `json:"latest_review_path,omitempty"`
	Review           *Review            `json:"review,omitempty"`
	Tests            []TestRun          `json:"tests,omitempty"`
	Costs            map[string]float64 `json:"costs,omitempty"`
	Adapters         map[string]string  `json:"adapters,omitempty"`
	Models           map[string]string  `json:"models,omitempty"`
	StartedAt        time.Time          `json:"started_at"`
	FinishedAt       time.Time          `json:"finished_at"`
}

type LatestRun struct {
	RunID     string    `json:"run_id"`
	RunDir    string    `json:"run_dir"`
	FinalPath string    `json:"final_path"`
	Verdict   string    `json:"verdict"`
	ExitCode  int       `json:"exit_code"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (f FinalRun) MarshalJSON() ([]byte, error) {
	type alias FinalRun
	return json.Marshal(alias(f))
}
