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
			Mode:       "supervisor",
			Coder:      "codex",
			Adversary:  "claude",
			Worker:     "agy:Gemini 3.5 Flash (High)",
			Supervisor: "claude:opus",
			Rounds:     2,
			GitSafety:  "clean",
		},
		Profiles: map[string]ProfileConfig{
			"fast": {
				Mode:      "adversarial",
				Coder:     "codex:gpt-5-codex-mini",
				Adversary: "claude:haiku",
				Rounds:    1,
			},
			"paranoid": {
				Mode:      "adversarial",
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
			Gosling: GoslingConfig{
				DefaultModel: "",
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
	legacyDefaultsOnly := src.Defaults.Mode == "" &&
		(src.Defaults.Coder != "" || src.Defaults.Adversary != "") &&
		src.Defaults.Worker == "" &&
		src.Defaults.Supervisor == ""
	if legacyDefaultsOnly {
		dst.Defaults.Mode = string(ModeAdversarial)
	}
	if src.Defaults.Mode != "" {
		dst.Defaults.Mode = src.Defaults.Mode
	}
	if src.Defaults.Coder != "" {
		dst.Defaults.Coder = src.Defaults.Coder
	}
	if src.Defaults.Adversary != "" {
		dst.Defaults.Adversary = src.Defaults.Adversary
	}
	if src.Defaults.Worker != "" {
		dst.Defaults.Worker = src.Defaults.Worker
	}
	if src.Defaults.Supervisor != "" {
		dst.Defaults.Supervisor = src.Defaults.Supervisor
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
			if profile.Mode != "" {
				current.Mode = profile.Mode
			}
			if profile.Coder != "" {
				current.Coder = profile.Coder
			}
			if profile.Adversary != "" {
				current.Adversary = profile.Adversary
			}
			if profile.Worker != "" {
				current.Worker = profile.Worker
			}
			if profile.Supervisor != "" {
				current.Supervisor = profile.Supervisor
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
	if src.Adapters.Gosling.DefaultModel != "" {
		dst.Adapters.Gosling.DefaultModel = src.Adapters.Gosling.DefaultModel
	}
	if len(src.Adapters.Gosling.ExtraArgs) > 0 {
		dst.Adapters.Gosling.ExtraArgs = append([]string{}, src.Adapters.Gosling.ExtraArgs...)
	}
}

func hasTagteamEnv() bool {
	for _, key := range []string{
		"TAGTEAM_MODE",
		"TAGTEAM_CODER",
		"TAGTEAM_ADVERSARY",
		"TAGTEAM_WORKER",
		"TAGTEAM_SUPERVISOR",
		"TAGTEAM_ROUNDS",
		"TAGTEAM_TEST",
		"TAGTEAM_GIT_SAFETY",
		"TAGTEAM_CODEX_ARGS",
		"TAGTEAM_CLAUDE_ARGS",
		"TAGTEAM_AGY_ARGS",
		"TAGTEAM_GOSLING_ARGS",
	} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

func mergeEnvConfig(cfg *Config) {
	legacyRoleEnvSet := false
	if value := os.Getenv("TAGTEAM_CODER"); value != "" {
		cfg.Defaults.Coder = value
		legacyRoleEnvSet = true
	}
	if value := os.Getenv("TAGTEAM_ADVERSARY"); value != "" {
		cfg.Defaults.Adversary = value
		legacyRoleEnvSet = true
	}
	if value := os.Getenv("TAGTEAM_WORKER"); value != "" {
		cfg.Defaults.Worker = value
	}
	if value := os.Getenv("TAGTEAM_SUPERVISOR"); value != "" {
		cfg.Defaults.Supervisor = value
	}
	if value := os.Getenv("TAGTEAM_MODE"); value != "" {
		cfg.Defaults.Mode = value
	} else if legacyRoleEnvSet {
		// TAGTEAM_CODER/TAGTEAM_ADVERSARY predate TAGTEAM_MODE and the
		// supervisor default; keep them selecting adversarial mode instead
		// of being silently ignored now that default resolution reads
		// Defaults.Worker/Defaults.Supervisor.
		cfg.Defaults.Mode = string(ModeAdversarial)
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
	if value := os.Getenv("TAGTEAM_GOSLING_ARGS"); value != "" {
		if parsed, err := shlex.Split(value); err == nil {
			cfg.Adapters.Gosling.ExtraArgs = parsed
		}
	}
}

func ResolveOptions(cfg Config, sources []string, flags FlagInputs, changed map[string]bool, prompt string) (RunOptions, error) {
	modeRaw := cfg.Defaults.Mode
	rounds := cfg.Defaults.Rounds
	testCmd := cfg.Defaults.Test
	gitSafety := cfg.Defaults.GitSafety

	var profile ProfileConfig
	hasProfile := false
	profileSetsMode := false
	if flags.Profile != "" {
		p, ok := cfg.Profiles[flags.Profile]
		if !ok {
			return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("unknown profile %q", flags.Profile)}
		}
		profile = p
		hasProfile = true
		switch {
		case profile.Mode != "":
			modeRaw = profile.Mode
			profileSetsMode = true
		case profile.Coder != "" || profile.Adversary != "":
			// Legacy profiles that only set coder/adversary predate the
			// mode field; keep them resolving as adversarial-mode profiles
			// instead of being silently ignored under the supervisor default.
			modeRaw = string(ModeAdversarial)
			profileSetsMode = true
		case profile.Worker != "" || profile.Supervisor != "":
			modeRaw = string(ModeSupervisor)
			profileSetsMode = true
		}
		if profile.Rounds != 0 {
			rounds = profile.Rounds
		}
		if profile.Test != "" {
			testCmd = profile.Test
		}
	}
	if changed["mode"] {
		modeRaw = flags.Mode
	}
	// modeExplicit only reflects choices that actually pin the mode for
	// this invocation (a --mode flag, or a profile that sets `mode` or the
	// legacy coder/adversary keys). A profile that only overrides
	// rounds/test must not block Fix() from resuming a saved run's mode.
	modeExplicit := profileSetsMode || changed["mode"]
	mode, err := ParseMode(modeRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: err}
	}

	// editor is the "coder" (adversarial mode) / "worker" (supervisor mode)
	// role; reviewer is "adversary" / "supervisor" respectively.
	//
	// editorExplicit/reviewerExplicit mirror modeExplicit: they're only set
	// when the profile actually supplies the role key for the resolved
	// mode, not merely because a profile was selected. Otherwise
	// `tagteam fix --profile ...` would have its profile-derived mode and
	// targets silently overwritten by Fix's saved-run resume logic below
	// even when the profile left those targets untouched.
	var editorRaw, reviewerRaw string
	editorExplicit := false
	reviewerExplicit := false
	editorExplicitMode := Mode("")
	reviewerExplicitMode := Mode("")
	if mode == ModeAdversarial {
		editorRaw = cfg.Defaults.Coder
		reviewerRaw = cfg.Defaults.Adversary
		if hasProfile {
			if profile.Coder != "" {
				editorRaw = profile.Coder
				editorExplicit = true
				editorExplicitMode = ModeAdversarial
			}
			if profile.Adversary != "" {
				reviewerRaw = profile.Adversary
				reviewerExplicit = true
				reviewerExplicitMode = ModeAdversarial
			}
		}
	} else {
		editorRaw = cfg.Defaults.Worker
		reviewerRaw = cfg.Defaults.Supervisor
		if hasProfile {
			if profile.Worker != "" {
				editorRaw = profile.Worker
				editorExplicit = true
				editorExplicitMode = ModeSupervisor
			}
			if profile.Supervisor != "" {
				reviewerRaw = profile.Supervisor
				reviewerExplicit = true
				reviewerExplicitMode = ModeSupervisor
			}
		}
	}

	// Legacy -mc/-ma flags always win over defaults/profile and are valid in
	// either mode. The newer --worker/--supervisor flags are the supervisor-
	// mode names for the same two slots, and --reviewer is the adversarial-
	// mode name for the reviewer slot; using one of these three outside its
	// matching mode is rejected rather than silently ignored or misapplied.
	if changed["mc"] {
		editorRaw = flags.Coder
		editorExplicit = true
		editorExplicitMode = ""
	}
	if changed["worker"] {
		if mode != ModeSupervisor {
			return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("--worker is only valid in supervisor mode (current mode %q); use -mc/--mc in adversarial mode", mode)}
		}
		editorRaw = flags.Worker
		editorExplicit = true
		editorExplicitMode = ModeSupervisor
	}
	if changed["ma"] {
		reviewerRaw = flags.Adversary
		reviewerExplicit = true
		reviewerExplicitMode = ""
	}
	if changed["reviewer"] {
		if mode != ModeAdversarial {
			return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("--reviewer is only valid in adversarial mode (current mode %q); use --supervisor in supervisor mode", mode)}
		}
		reviewerRaw = flags.Reviewer
		reviewerExplicit = true
		reviewerExplicitMode = ModeAdversarial
	}
	if changed["supervisor"] {
		if mode != ModeSupervisor {
			return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("--supervisor is only valid in supervisor mode (current mode %q); use --reviewer or -ma in adversarial mode", mode)}
		}
		reviewerRaw = flags.Supervisor
		reviewerExplicit = true
		reviewerExplicitMode = ModeSupervisor
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

	editorLabel, reviewerLabel := roleLabels(mode)
	editorTarget, err := ParseRoleTarget(editorRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("resolve %s target: %w", editorLabel, err)}
	}
	reviewerTarget, err := ParseRoleTarget(reviewerRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("resolve %s target: %w", reviewerLabel, err)}
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
	goslingArgs, err := mergePassthrough(cfg.Adapters.Gosling.ExtraArgs, flags.GoslingArgsRaw)
	if err != nil {
		return RunOptions{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("parse --gosling-args: %w", err)}
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
		Prompt:                    strings.TrimSpace(prompt),
		Workdir:                   workdir,
		Mode:                      mode,
		ModeExplicit:              modeExplicit,
		Coder:                     editorTarget,
		Adversary:                 reviewerTarget,
		CoderExplicit:             editorExplicit,
		AdversaryExplicit:         reviewerExplicit,
		CoderExplicitMode:         editorExplicitMode,
		AdversaryExplicitMode:     reviewerExplicitMode,
		SupervisorCanEdit:         flags.SupervisorCanEdit,
		SupervisorCanEditExplicit: changed["supervisor-can-edit"],
		Rounds:                    rounds,
		TestCmd:                   testCmd,
		NoTest:                    flags.NoTest,
		JSON:                      flags.JSON,
		DryRun:                    flags.DryRun,
		ShowReview:                flags.ShowReview,
		FailOnReview:              flags.FailOnReview,
		AllowDirty:                flags.AllowDirty,
		Autostash:                 flags.Autostash,
		Timeout:                   flags.Timeout,
		Quiet:                     flags.Quiet,
		Verbose:                   flags.Verbose,
		GitSafety:                 gitSafety,
		CodexArgs:                 codexArgs,
		ClaudeArgs:                claudeArgs,
		AgyArgs:                   agyArgs,
		GoslingArgs:               goslingArgs,
		ConfigSources:             sources,
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
