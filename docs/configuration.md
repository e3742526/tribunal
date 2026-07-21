# Configuration

## Precedence

Lowest to highest: built-in defaults, `~/.config/tribunal/config.toml`, a
workspace `.tribunal.toml` only with `--trust-workspace-config`, shell
`TRIBUNAL_*`, then explicitly supplied flags. A workspace `.env` is never read.

Trusted workspace configuration is hashed and listed before any invocation.
State roots, endpoints, headers, credential-variable selectors, panels, worker
network policy, edit permissions, and budgets are authority-bearing.

## Defaults

- Panel: `claude/claude-opus-4-8`, `codex/gpt-5.6-sol`,
  `agy/Gemini 3.5 Flash (Medium)`.
- Kind: `generic`; passes: 2; max findings/reviewer: 25.
- Context: 131072 tokens; reserve: 16384; total token cap: 500000.
- Per call: 15 minutes and 1 MiB; run: 60 minutes.
- Clustering: rules; quorum: majority with minimum 2.
- Verification and arbitration cap: 10 each.
- State root: `~/.local/state/tribunal`.
- Worker network: disabled unless a task and exact domain are allowed.

## Shell variables

The supported prefix is `TRIBUNAL_`; no `TAGTEAM_*` value is inspected. Core
variables mirror documented flags: `TRIBUNAL_STATE_ROOT`, `TRIBUNAL_PANEL`,
`TRIBUNAL_PASSES`, `TRIBUNAL_MAX_OUTPUT_BYTES`, `TRIBUNAL_MAX_WALL_TIME`, and
`TRIBUNAL_TOKEN_BUDGET`. Adapter credentials use the environment-variable name
selected by trusted user configuration and are redacted from all output.

## OpenAI-compatible adapter

Trusted config supplies `base_url`, `model`, optional static headers, context
budget, and `api_key_env`. Requests use temperature 0, a role JSON schema,
bounded response reads, and no automatic redirect to another origin.

