# tagteam

`tagteam` is a standalone Go CLI that runs two headless coding agents as one command:

- a `coder` agent that edits the repo
- an `adversary` agent that reviews the resulting diff

The tool loops findings back into the coder until the change passes review, tests fail, or the round limit is reached.

## Status

This repository is an early implementation of the v2 design. The core run loop, adapter abstraction, persisted run artifacts, and main command surface are in place. Some hardening and release work is still pending.

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

Authentication is owned by the vendor CLIs. `tagteam` does not proxy API keys.

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

Default run:

```bash
tagteam "add OAuth login"
```

Choose explicit adapters and a test command:

```bash
tagteam \
  -mc codex:gpt-5-codex \
  -ma claude:opus \
  -r 3 \
  -t "go test ./..." \
  "refactor billing flow"
```

Use Agy with its configured default Gemini model:

```bash
tagteam -mc agy -ma claude:sonnet "clean up the CLI help"
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

## Run Artifacts

Each run writes artifacts under:

```text
.tagteam/runs/<run-id>/
```

Typical contents include:

- `meta.json`
- `input.md`
- `coder-round-N.md`
- `diff-round-N.patch`
- `test-round-N.txt`
- `adversary-round-N.json`
- `final.json`

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
