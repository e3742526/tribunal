# tagteam

`tagteam` is a standalone Go CLI that runs two headless coding agents as one command.

By default it runs in **supervisor mode**:

- a `supervisor` agent that writes a compact implementation brief, then reviews the resulting diff (it does not edit files by default)
- a `worker` agent that implements the brief and fixes findings

It can also run the original **adversarial mode** (`--mode adversarial`):

- a `coder` agent that edits the repo
- an `adversary` agent that reviews the resulting diff

In both modes the tool loops findings back into the editor role until the change passes review, tests fail, or the user-defined round limit is reached. When the limit is reached with unresolved blocker/major findings, `tagteam` stops asking for edits and asks both agents for final "what remains incomplete / what do you dispute" reports instead of continuing indefinitely.

## Vision

The goal is to make adversarial review and supervisor-worker coding transparent in a CLI.

Instead of hiding the interaction inside a vendor UI, `tagteam` makes the roles explicit, saves the brief/review/diff/test artifacts for each run, and keeps the handoff between "build" and "criticize" inspectable from the repository.

## Status

This repository is an early implementation of the v2 design. The core run loop, adapter abstraction, persisted run artifacts, and main command surface are in place. Some hardening and release work is still pending.

Recent additions in this repo:

- supervisor/worker mode is now the default flow
- adversarial coder/adversary mode remains available for backward compatibility
- saved run artifacts include briefs, diffs, reviews, tests, and final summaries
- command surface now includes `review`, `fix`, `status`, `transcript`, `doctor`, and `init`
- config layering supports repo config, user config, env overrides, flags, and named profiles
- machine-readable output and dry-run support make the CLI easier to script and debug

Current commands:

- `tagteam "<prompt>"`
- `tagteam review`
- `tagteam fix`
- `tagteam status`
- `tagteam transcript [RUN_ID]`
- `tagteam doctor`
- `tagteam init`

## Requirements

- Go 1.22+
- Git
- At least one supported agent CLI on `PATH`

Supported adapters in this repo today:

- `codex`
- `codex-oss`
- `claude`
- `agy`
- `gosling` (coder-only)

Authentication is owned by the vendor CLIs. `tagteam` does not proxy API keys.

## Compatibility Risks

`tagteam` depends on third-party agent CLIs whose command-line contracts are not stable. Flags, output formats, auth flows, and model-selection syntax may change upstream without warning.

That means adapters in this repository can break when tools like `codex`, `claude`, `agy`, `codex-oss`, or `gosling` change their flags or behavior. Expect periodic adapter maintenance as those CLIs evolve.

## Install

Build from source:

```bash
go build -o tagteam .
```

Run locally:

```bash
go run . "add OAuth login"
```

## Quick Start

Default run (supervisor mode, built-in worker `agy:Gemini 3.5 Flash (High)` and supervisor `claude:opus`):

```bash
tagteam "add OAuth login"
```

Choose explicit worker/supervisor adapters, rounds, and a test command:

```bash
tagteam \
  --worker codex:gpt-5-codex \
  --supervisor claude:opus \
  -r 3 \
  -t "go test ./..." \
  "refactor billing flow"
```

The supervisor is read-only by default (it writes the brief and review findings but does not edit files). Allow it to make small exploratory edits with `--supervisor-can-edit`.

### Adversarial mode (backward compatible)

The original coder/adversary loop is still available via `--mode adversarial`. The legacy `-mc`/`-ma` flags keep working and map onto the active mode's roles: `-mc` selects the worker in supervisor mode and the coder in adversarial mode; `-ma` selects the supervisor in supervisor mode and the adversary in adversarial mode.

```bash
tagteam --mode adversarial \
  -mc codex:gpt-5-codex \
  -ma claude:opus \
  -r 3 \
  -t "go test ./..." \
  "refactor billing flow"
```

`--reviewer` is an adversarial-mode-flavored alias for `-ma`/`--supervisor`:

```bash
tagteam --mode adversarial -mc agy --reviewer claude:sonnet "clean up the CLI help"
```

Use Agy with its configured default Gemini model:

```bash
tagteam --worker agy --supervisor claude:sonnet "clean up the CLI help"
```

The built-in `agy` default model is `gemini-3.5-flash`; override it with `agy:<model>`.

Review the current diff only:

```bash
tagteam review --fail-on-review
```

Apply fixes from the latest saved review:

```bash
tagteam fix
```

## Configuration

Configuration precedence is:

`flags > TAGTEAM_* env > repo .tagteam.toml > user config > built-in defaults`

User config path:

- macOS/Linux: `~/.config/tagteam/config.toml`

Starter config:

```bash
tagteam init
```

Relevant `defaults` keys:

- `mode` — `supervisor` (default) or `adversarial`
- `worker` / `supervisor` — `adapter[:model]` targets used in supervisor mode
- `coder` / `adversary` — `adapter[:model]` targets used in adversarial mode
- `rounds` — hard cap on implementation/review cycles; exhausted runs stop and collect final reports from both agents
- `test`, `git_safety`

Profiles may override `mode`, `worker`, `supervisor`, `coder`, `adversary`, `rounds`, and `test`. A profile that sets `coder`/`adversary` but omits `mode` resolves as an adversarial-mode profile, so profiles written before `mode` existed keep working unchanged:

```toml
[defaults]
mode = "supervisor"
worker = "agy:Gemini 3.5 Flash (High)"
supervisor = "claude:opus"
rounds = 2

[profiles.fast]
coder = "codex:gpt-5-codex-mini"
adversary = "claude:haiku"
rounds = 1
```

## Run Artifacts

Each run writes artifacts under:

```text
.tagteam/runs/<run-id>/
```

Typical contents include:

- `meta.json`
- `input.md`
- `supervisor-brief.md` (supervisor mode only, round 1)
- `worker-round-N.md` (supervisor mode) / `coder-round-N.md` (adversarial mode)
- `diff-round-N.patch`
- `diff-round-N.numstat`
- `diff-round-N.files.json`
- `diff-round-N.sha256`
- `test-round-N.txt`
- `supervisor-round-N.json` (supervisor mode) / `adversary-round-N.json` (adversarial mode)
- `worker-final-report.md` / `coder-final-report.md` and `supervisor-final-report.md` / `adversary-final-report.md` when the round limit is exhausted
- `final.json`

Diff artifacts are captured through a temporary Git index, not the real staging
area. The canonical patch includes tracked changes, deletions, renames, binary
patches, and untracked files, while always excluding `.tagteam/`.

## Development

Format and test:

```bash
gofmt -w main.go internal/cli/root.go internal/tagteam/*.go
go test ./...
go vet ./...
```

## Scope

This is not intended to be:

- a vendor CLI shim
- a general multi-agent framework
- a credential manager
- a raw model runner

## License

MIT
