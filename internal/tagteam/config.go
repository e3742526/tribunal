package tagteam

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/shlex"
)

func DefaultConfig() Config {
	return Config{
		Defaults: DefaultsConfig{
			Coder:     "codex",
			Adversary: "claude",
			Rounds:    2,
			GitSafety: "clean",
		},
		Profiles: map[string]ProfileConfig{
			"fast": {
				Coder:     "codex:gpt-5-codex-mini",
				Adversary: "claude:haiku",
				Rounds:    1,
			},
			"paranoid": {
				Adversary: "claude:opus",
				Rounds:    4,
				Test:      "make check",
			},
		},
		Adapters: AdapterConfigSet{
			Codex: CodexConfig{},
			Claude: ClaudeConfig{
				DefaultModel:      "sonnet",
				CoderAllowedTools: []string{"Edit", "Write", "Read", "Glob", "Grep", "Bash"},
				ExtraArgs:         []string{},
			},
			CodexOSS: CodexConfig{
				DefaultModel: "qwen3-coder",
			},
			Agy: AgyConfig{
				DefaultModel: "gemini-3.5-flash",
				ExtraArgs:    []string{},
			},
		},
	}
}

func LoadConfig(workdir string) (Config, []string, error) {
	cfg := DefaultConfig()
	sources := []string{"built-in defaults"}

	userPath, err := userConfigPath()
	if err != nil {
		return Config{}, nil, err
	}
	if err := mergeConfigFile(&cfg, userPath); err != nil {
		return Config{}, nil, err
	}
	if fileExists(userPath) {
		sources = append(sources, userPath)
	}

	repoPath := filepath.Join(workdir, ".tagteam.toml")
	if err := mergeConfigFile(&cfg, repoPath); err != nil {
		return Config{}, nil, err
	}
	if fileExists(repoPath) {
		sources = append(sources, repoPath)
	}

	mergeEnvConfig(&cfg)
	if hasTagteamEnv() {
		sources = append(sources, "TAGTEAM_* env")
	}

	return cfg, sources, nil
}

func userConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "tagteam", "config.toml"), nil
}

func mergeConfigFile(dst *Config, path string) error {
	if !fileExists(path) {
		return nil
	}
	var src Config
	if _, err := toml.DecodeFile(path, &src); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	mergeConfig(dst, src)
	return nil
}

func mergeConfig(dst *Config, src Config) {
	if src.Defaults.Coder != "" {
		dst.Defaults.Coder = src.Defaults.Coder
	}
	if src.Defaults.Adversary != "" {
		dst.Defaults.Adversary = src.Defaults.Adversary
	}
	if src.Defaults.Rounds != 0 {
		dst.Defaults.Rounds = src.Defaults.Rounds
	}
	if src.Defaults.Test != "" {
		dst.Defaults.Test = src.Defaults.Test
	}
	if src.Defaults.GitSafety != "" {
		dst.Defaults.GitSafety = src.Defaults.GitSafety
	}
	if src.Profiles != nil {
		if dst.Profiles == nil {
			dst.Profiles = map[string]ProfileConfig{}
		}
		for key, profile := range src.Profiles {
			current := dst.Profiles[key]
			if profile.Coder != "" {
				current.Coder = profile.Coder
			}
			if profile.Adversary != "" {
				current.Adversary = profile.Adversary
			}
			if profile.Rounds != 0 {
				current.Rounds = profile.Rounds
			}
			if profile.Test != "" {
				current.Test = profile.Test
			}
			dst.Profiles[key] = current
		}
	}
	if src.Adapters.Codex.DefaultModel != "" {
		dst.Adapters.Codex.DefaultModel = src.Adapters.Codex.DefaultModel
	}
	if len(src.Adapters.Codex.ExtraArgs) > 0 {
		dst.Adapters.Codex.ExtraArgs = append([]string{}, src.Adapters.Codex.ExtraArgs...)
	}
	if src.Adapters.Claude.DefaultModel != "" {
		dst.Adapters.Claude.DefaultModel = src.Adapters.Claude.DefaultModel
	}
	if len(src.Adapters.Claude.CoderAllowedTools) > 0 {
		dst.Adapters.Claude.CoderAllowedTools = append([]string{}, src.Adapters.Claude.CoderAllowedTools...)
	}
	if len(src.Adapters.Claude.ExtraArgs) > 0 {
		dst.Adapters.Claude.ExtraArgs = append([]string{}, src.Adapters.Claude.ExtraArgs...)
	}
	if src.Adapters.Claude.Bare {
		dst.Adapters.Claude.Bare = true
	}
	if src.Adapters.CodexOSS.DefaultModel != "" {
		dst.Adapters.CodexOSS.DefaultModel = src.Adapters.CodexOSS.DefaultModel
	}
	if len(src.Adapters.CodexOSS.ExtraArgs) > 0 {
		dst.Adapters.CodexOSS.ExtraArgs = append([]string{}, src.Adapters.CodexOSS.ExtraArgs...)
	}
	if src.Adapters.Agy.DefaultModel != "" {
		dst.Adapters.Agy.DefaultModel = src.Adapters.Agy.DefaultModel
	}
	if len(src.Adapters.Agy.ExtraArgs) > 0 {
		dst.Adapters.Agy.ExtraArgs = append([]string{}, src.Adapters.Agy.ExtraArgs...)
	}
}

func hasTagteamEnv() bool {
	for _, key := range []string{
		"TAGTEAM_CODER",
		"TAGTEAM_ADVERSARY",
		"TAGTEAM_ROUNDS",
		"TAGTEAM_TEST",
		"TAGTEAM_GIT_SAFETY",
		"TAGTEAM_CODEX_ARGS",
		"TAGTEAM_CLAUDE_ARGS",
		"TAGTEAM_AGY_ARGS",
	} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

func mergeEnvConfig(cfg *Config) {
	if value := os.Getenv("TAGTEAM_CODER"); value != "" {
		cfg.Defaults.Coder = value
	}
	if value := os.Getenv("TAGTEAM_ADVERSARY"); value != "" {
		cfg.Defaults.Adversary = value
	}
	if value := os.Getenv("TAGTEAM_ROUNDS"); value != "" {
		if rounds, err := strconv.Atoi(value); err == nil && rounds > 0 {
			cfg.Defaults.Rounds = rounds
		}
	}
	if value := os.Getenv("TAGTEAM_TEST"); value != "" {
		cfg.Defaults.Test = value
	}
	if value := os.Getenv("TAGTEAM_GIT_SAFETY"); value != "" {
		cfg.Defaults.GitSafety = value
	}
	if value := os.Getenv("TAGTEAM_CODEX_ARGS"); value != "" {
		if parsed, err := shlex.Split(value); err == nil {
			cfg.Adapters.Codex.ExtraArgs = parsed
		}
	}
	if value := os.Getenv("TAGTEAM_CLAUDE_ARGS"); value != "" {
		if parsed, err := shlex.Split(value); err == nil {
			cfg.Adapters.Claude.ExtraArgs = parsed
		}
	}
	if value := os.Getenv("TAGTEAM_AGY_ARGS"); value != "" {
		if parsed, err := shlex.Split(value); err == nil {
			cfg.Adapters.Agy.ExtraArgs = parsed
		}
	}
}

func ResolveOptions(cfg Config, sources []string, flags FlagInputs, changed map[string]bool, prompt string) (RunOptions, error) {
	coder := cfg.Defaults.Coder
	adversary := cfg.Defaults.Adversary
	rounds := cfg.Defaults.Rounds
	testCmd := cfg.Defaults.Test
	gitSafety := cfg.Defaults.GitSafety

	if flags.Profile != "" {
		profile, ok := cfg.Profiles[flags.Profile]
		if !ok {
			return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("unknown profile %q", flags.Profile)}
		}
		if profile.Coder != "" {
			coder = profile.Coder
		}
		if profile.Adversary != "" {
			adversary = profile.Adversary
		}
		if profile.Rounds != 0 {
			rounds = profile.Rounds
		}
		if profile.Test != "" {
			testCmd = profile.Test
		}
	}

	if changed["mc"] {
		coder = flags.Coder
	}
	if changed["ma"] {
		adversary = flags.Adversary
	}
	if changed["rounds"] {
		rounds = flags.Rounds
	}
	if changed["test"] {
		testCmd = flags.Test
	}
	if changed["allow-dirty"] {
		gitSafety = "allow-dirty"
	}
	if changed["autostash"] {
		gitSafety = "autostash"
	}
	if gitSafety != "clean" && gitSafety != "autostash" && gitSafety != "branch" && gitSafety != "allow-dirty" {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("invalid git_safety %q", gitSafety)}
	}

	coderTarget, err := ParseRoleTarget(coder)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("resolve coder target: %w", err)}
	}
	adversaryTarget, err := ParseRoleTarget(adversary)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("resolve adversary target: %w", err)}
	}

	codexArgs, err := mergePassthrough(cfg.Adapters.Codex.ExtraArgs, flags.CodexArgsRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("parse --codex-args: %w", err)}
	}
	claudeArgs, err := mergePassthrough(cfg.Adapters.Claude.ExtraArgs, flags.ClaudeArgsRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("parse --claude-args: %w", err)}
	}
	agyArgs, err := mergePassthrough(cfg.Adapters.Agy.ExtraArgs, flags.AgyArgsRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("parse --agy-args: %w", err)}
	}

	workdir := flags.Workdir
	if workdir == "" {
		workdir = "."
	}
	workdir, err = filepath.Abs(workdir)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("resolve workdir: %w", err)}
	}
	if rounds <= 0 {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("rounds must be > 0")}
	}

	return RunOptions{
		Prompt:        strings.TrimSpace(prompt),
		Workdir:       workdir,
		Coder:         coderTarget,
		Adversary:     adversaryTarget,
		Rounds:        rounds,
		TestCmd:       testCmd,
		NoTest:        flags.NoTest,
		JSON:          flags.JSON,
		DryRun:        flags.DryRun,
		ShowReview:    flags.ShowReview,
		FailOnReview:  flags.FailOnReview,
		AllowDirty:    flags.AllowDirty,
		Autostash:     flags.Autostash,
		Timeout:       flags.Timeout,
		Quiet:         flags.Quiet,
		Verbose:       flags.Verbose,
		GitSafety:     gitSafety,
		CodexArgs:     codexArgs,
		ClaudeArgs:    claudeArgs,
		AgyArgs:       agyArgs,
		ConfigSources: sources,
	}, nil
}

func mergePassthrough(base []string, raw string) ([]string, error) {
	merged := append([]string{}, base...)
	if strings.TrimSpace(raw) == "" {
		return merged, nil
	}
	parts, err := shlex.Split(raw)
	if err != nil {
		return nil, err
	}
	return append(merged, parts...), nil
}

func EncodeConfig(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
