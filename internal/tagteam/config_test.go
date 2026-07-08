package tagteam

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveOptions_ProfileAndFlags(t *testing.T) {
	cfg := DefaultConfig()
	opts, err := ResolveOptions(cfg, []string{"defaults"}, FlagInputs{
		Profile: "fast",
		Rounds:  3,
		Timeout: 15 * time.Minute,
	}, map[string]bool{"rounds": true}, "ship it")
	if err != nil {
		t.Fatalf("ResolveOptions() error = %v", err)
	}
	if opts.Coder.Adapter != "codex" {
		t.Fatalf("coder adapter = %q", opts.Coder.Adapter)
	}
	if opts.Adversary.Model != "haiku" {
		t.Fatalf("adversary model = %q", opts.Adversary.Model)
	}
	if opts.Rounds != 3 {
		t.Fatalf("rounds = %d", opts.Rounds)
	}
}

func TestLoadConfig_RepoOverridesUser(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "tagteam"), 0o755); err != nil {
		t.Fatal(err)
	}
	userConfig := []byte("[defaults]\ncoder = \"codex:gpt-5\"\n")
	if err := os.WriteFile(filepath.Join(home, "tagteam", "config.toml"), userConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	repoConfig := []byte("[defaults]\ncoder = \"claude:opus\"\n")
	if err := os.WriteFile(filepath.Join(repo, ".tagteam.toml"), repoConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfig(repo)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Defaults.Coder != "claude:opus" {
		t.Fatalf("coder = %q", cfg.Defaults.Coder)
	}
}

func TestResolveOptions_RejectsInvalidGitSafety(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.GitSafety = "bad-mode"
	_, err := ResolveOptions(cfg, []string{"defaults"}, FlagInputs{Timeout: 15 * time.Minute}, map[string]bool{}, "ship it")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveOptions_AgyPassthrough(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Adapters.Agy.ExtraArgs = []string{"--project", "repo"}
	opts, err := ResolveOptions(cfg, []string{"defaults"}, FlagInputs{
		AgyArgsRaw: "--new-project",
		Timeout:    15 * time.Minute,
	}, map[string]bool{}, "ship it")
	if err != nil {
		t.Fatalf("ResolveOptions() error = %v", err)
	}
	want := []string{"--project", "repo", "--new-project"}
	if len(opts.AgyArgs) != len(want) {
		t.Fatalf("agy args length = %d, want %d: %#v", len(opts.AgyArgs), len(want), opts.AgyArgs)
	}
	for i := range want {
		if opts.AgyArgs[i] != want[i] {
			t.Fatalf("agy args[%d] = %q, want %q", i, opts.AgyArgs[i], want[i])
		}
	}
}
