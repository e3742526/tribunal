// Package config resolves Tribunal's trusted layered configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

const DefaultPanel = "claude/claude-opus-4-8,codex/gpt-5.6-sol,agy/Gemini 3.5 Flash (Medium)"

type Limits struct {
	Passes           int           `toml:"passes" json:"passes"`
	MaxFindings      int           `toml:"max_findings" json:"max_findings"`
	MaxOutputBytes   int64         `toml:"max_output_bytes" json:"max_output_bytes"`
	CallTimeout      time.Duration `toml:"-" json:"call_timeout"`
	CallTimeoutText  string        `toml:"call_timeout" json:"-"`
	RunTimeout       time.Duration `toml:"-" json:"run_timeout"`
	RunTimeoutText   string        `toml:"run_timeout" json:"-"`
	TokenBudget      int           `toml:"token_budget" json:"token_budget"`
	MaxVerification  int           `toml:"max_verification" json:"max_verification"`
	MaxArbitration   int           `toml:"max_arbitration" json:"max_arbitration"`
	MaxContextTokens int           `toml:"max_context_tokens" json:"max_context_tokens"`
	ReservedOutput   int           `toml:"reserved_output_tokens" json:"reserved_output_tokens"`
}

type OpenAICompatible struct {
	BaseURL      string            `toml:"base_url" json:"base_url"`
	Model        string            `toml:"model" json:"model"`
	APIKeyEnv    string            `toml:"api_key_env" json:"api_key_env"`
	Headers      map[string]string `toml:"headers" json:"headers"`
	MaxContext   int               `toml:"max_context_tokens" json:"max_context_tokens"`
	OutputTokens int               `toml:"reserved_output_tokens" json:"reserved_output_tokens"`
}

type WorkerConfig struct {
	AllowedDomains   []string `toml:"allowed_domains" json:"allowed_domains"`
	WebSearchURL     string   `toml:"websearch_url" json:"websearch_url"`
	WebSearchAuthEnv string   `toml:"websearch_auth_env" json:"websearch_auth_env"`
}

type Config struct {
	SchemaVersion    int              `toml:"schema_version" json:"schema_version"`
	StateRoot        string           `toml:"state_root" json:"state_root"`
	Panel            string           `toml:"panel" json:"panel"`
	Kind             string           `toml:"kind" json:"kind"`
	Limits           Limits           `toml:"limits" json:"limits"`
	OpenAICompatible OpenAICompatible `toml:"openai_compatible" json:"openai_compatible"`
	Workers          WorkerConfig     `toml:"workers" json:"workers"`
	TrustedSources   []string         `toml:"-" json:"trusted_sources"`
	IgnoredSources   []string         `toml:"-" json:"ignored_sources"`
	WorkspaceRoot    string           `toml:"-" json:"workspace_root,omitempty"`
	TrustWorkspace   bool             `toml:"-" json:"trust_workspace_config"`
}

type LoadOptions struct {
	Workspace            string
	TrustWorkspaceConfig bool
	ExplicitStateRoot    string
	ExplicitPanel        string
	ExplicitKind         string
	ExplicitPasses       int
	ExplicitOutputBytes  int64
	ExplicitRunTimeout   time.Duration
	ExplicitTokenBudget  int
}

func Default() Config {
	return Config{
		SchemaVersion: domain.SchemaVersion,
		Panel:         DefaultPanel,
		Kind:          "generic",
		Limits: Limits{
			Passes:           2,
			MaxFindings:      25,
			MaxOutputBytes:   1 << 20,
			CallTimeout:      15 * time.Minute,
			RunTimeout:       time.Hour,
			TokenBudget:      500000,
			MaxVerification:  10,
			MaxArbitration:   10,
			MaxContextTokens: 131072,
			ReservedOutput:   16384,
		},
		OpenAICompatible: OpenAICompatible{BaseURL: "http://127.0.0.1:11434/v1", Model: "gemma4:latest", Headers: map[string]string{}, MaxContext: 131072, OutputTokens: 16384},
	}
}

func Load(opts LoadOptions) (Config, error) {
	cfg := Default()
	cfg.WorkspaceRoot = opts.Workspace
	cfg.TrustWorkspace = opts.TrustWorkspaceConfig
	cfg.TrustedSources = []string{"built-in defaults"}
	userPath, err := userConfigPath()
	if err != nil {
		return Config{}, err
	}
	if err := mergeFile(&cfg, userPath); err != nil {
		return Config{}, err
	}
	if exists(userPath) {
		cfg.TrustedSources = append(cfg.TrustedSources, userPath)
	}
	if opts.Workspace != "" {
		workspacePath := filepath.Join(opts.Workspace, ".tribunal.toml")
		if exists(workspacePath) {
			if opts.TrustWorkspaceConfig {
				if err := mergeFile(&cfg, workspacePath); err != nil {
					return Config{}, err
				}
				cfg.TrustedSources = append(cfg.TrustedSources, workspacePath)
			} else {
				cfg.IgnoredSources = append(cfg.IgnoredSources, workspacePath)
			}
		}
		if exists(filepath.Join(opts.Workspace, ".env")) {
			cfg.IgnoredSources = append(cfg.IgnoredSources, filepath.Join(opts.Workspace, ".env"))
		}
	}
	if err := mergeEnv(&cfg); err != nil {
		return Config{}, err
	}
	if hasTribunalEnv() {
		cfg.TrustedSources = append(cfg.TrustedSources, "TRIBUNAL_* shell environment")
	}
	applyFlags(&cfg, opts)
	if err := normalize(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	if !exists(path) {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("config %s must be a regular non-symlink file", path)
	}
	metadata, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	if unknown := metadata.Undecoded(); len(unknown) > 0 {
		return fmt.Errorf("load config %s: unknown key %s", path, unknown[0].String())
	}
	return nil
}

func mergeEnv(cfg *Config) error {
	known := map[string]bool{"TRIBUNAL_STATE_ROOT": true, "TRIBUNAL_PANEL": true, "TRIBUNAL_PASSES": true, "TRIBUNAL_MAX_OUTPUT_BYTES": true, "TRIBUNAL_MAX_WALL_TIME": true, "TRIBUNAL_TOKEN_BUDGET": true}
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.HasPrefix(key, "TRIBUNAL_") && !known[key] {
			return fmt.Errorf("unknown Tribunal environment key %s", key)
		}
	}
	if value := os.Getenv("TRIBUNAL_STATE_ROOT"); value != "" {
		cfg.StateRoot = value
	}
	if value := os.Getenv("TRIBUNAL_PANEL"); value != "" {
		cfg.Panel = value
	}
	if value := os.Getenv("TRIBUNAL_PASSES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("TRIBUNAL_PASSES must be an integer")
		}
		cfg.Limits.Passes = parsed
	}
	if value := os.Getenv("TRIBUNAL_MAX_OUTPUT_BYTES"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("TRIBUNAL_MAX_OUTPUT_BYTES must be an integer")
		}
		cfg.Limits.MaxOutputBytes = parsed
	}
	if value := os.Getenv("TRIBUNAL_MAX_WALL_TIME"); value != "" {
		cfg.Limits.RunTimeoutText = value
	}
	if value := os.Getenv("TRIBUNAL_TOKEN_BUDGET"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("TRIBUNAL_TOKEN_BUDGET must be an integer")
		}
		cfg.Limits.TokenBudget = parsed
	}
	return nil
}

func applyFlags(cfg *Config, opts LoadOptions) {
	if opts.ExplicitStateRoot != "" {
		cfg.StateRoot = opts.ExplicitStateRoot
	}
	if opts.ExplicitPanel != "" {
		cfg.Panel = opts.ExplicitPanel
	}
	if opts.ExplicitKind != "" {
		cfg.Kind = opts.ExplicitKind
	}
	if opts.ExplicitPasses != 0 {
		cfg.Limits.Passes = opts.ExplicitPasses
	}
	if opts.ExplicitOutputBytes != 0 {
		cfg.Limits.MaxOutputBytes = opts.ExplicitOutputBytes
	}
	if opts.ExplicitRunTimeout != 0 {
		cfg.Limits.RunTimeout = opts.ExplicitRunTimeout
	}
	if opts.ExplicitTokenBudget != 0 {
		cfg.Limits.TokenBudget = opts.ExplicitTokenBudget
	}
}

func normalize(cfg *Config) error {
	if cfg.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("config schema_version must be %d", domain.SchemaVersion)
	}
	if cfg.Limits.CallTimeoutText != "" {
		value, err := time.ParseDuration(cfg.Limits.CallTimeoutText)
		if err != nil {
			return fmt.Errorf("limits.call_timeout: %w", err)
		}
		cfg.Limits.CallTimeout = value
	}
	if cfg.Limits.RunTimeoutText != "" {
		value, err := time.ParseDuration(cfg.Limits.RunTimeoutText)
		if err != nil {
			return fmt.Errorf("limits.run_timeout: %w", err)
		}
		cfg.Limits.RunTimeout = value
	}
	if cfg.Limits.Passes < 1 || cfg.Limits.Passes > 3 {
		return fmt.Errorf("limits.passes must be between 1 and 3")
	}
	if cfg.Limits.MaxFindings < 1 || cfg.Limits.MaxOutputBytes < 1024 || cfg.Limits.TokenBudget < 1 {
		return fmt.Errorf("configured limits must be positive and max_output_bytes at least 1024")
	}
	// Sub-second timeouts truncate to zero seconds at the adapter boundary,
	// which the adapters would then read as "use the 15-minute default" —
	// the opposite of the configured intent.
	if cfg.Limits.CallTimeout < time.Second || cfg.Limits.RunTimeout < time.Second {
		return fmt.Errorf("limits.call_timeout and limits.run_timeout must be at least 1s")
	}
	if cfg.Limits.MaxVerification < 1 || cfg.Limits.MaxArbitration < 1 {
		return fmt.Errorf("limits.max_verification and limits.max_arbitration must be at least 1; a non-positive value would silently disable the pipeline it gates")
	}
	if _, ok := BuiltinRubric(cfg.Kind); !ok {
		return fmt.Errorf("unknown document kind %q", cfg.Kind)
	}
	if cfg.Workers.WebSearchURL != "" {
		endpoint, err := url.Parse(cfg.Workers.WebSearchURL)
		if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
			return fmt.Errorf("workers.websearch_url must be an absolute HTTPS URL")
		}
		allowed := false
		for _, domainName := range cfg.Workers.AllowedDomains {
			allowed = allowed || strings.EqualFold(strings.TrimSuffix(domainName, "."), strings.TrimSuffix(endpoint.Hostname(), "."))
		}
		if !allowed {
			return fmt.Errorf("workers.websearch_url domain must be explicitly allowlisted")
		}
	}
	if cfg.OpenAICompatible.BaseURL != "" {
		endpoint, err := url.Parse(cfg.OpenAICompatible.BaseURL)
		if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
			return fmt.Errorf("openai_compatible.base_url must be an absolute HTTP(S) URL")
		}
		if endpoint.Scheme == "http" {
			ip := net.ParseIP(endpoint.Hostname())
			if endpoint.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
				return fmt.Errorf("openai_compatible.base_url requires HTTPS except on loopback")
			}
		}
	}
	panel, err := domain.ParsePanel(cfg.Panel)
	if err != nil {
		return err
	}
	for i := range panel.Reviewers {
		panel.Reviewers[i].MaxContextTokens = cfg.Limits.MaxContextTokens
		panel.Reviewers[i].ReservedOutputTokens = cfg.Limits.ReservedOutput
	}
	return domain.ValidatePanel(panel)
}

func userConfigPath() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "tribunal", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tribunal", "config.toml"), nil
}

func hasTribunalEnv() bool {
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "TRIBUNAL_") {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
