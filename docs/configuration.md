# Configuration

## Precedence

Lowest to highest: built-in defaults, `~/.config/tribunal/config.toml`, a
workspace `.tribunal.toml` only with `--trust-workspace-config`, shell
`TRIBUNAL_*`, then explicitly supplied flags. A workspace `.env` is never read.

Trusted workspace configuration is hashed and listed before any invocation.
State roots, endpoints, headers, credential-variable selectors, panels, worker
network policy, edit permissions, and budgets are authority-bearing.

## Defaults

- Panel: `claude/claude-opus-5`, `codex/gpt-5.6-sol`,
  `agy/Gemini 3.5 Flash (Medium)`.
- Panel policy: unset; the panel string is used verbatim.
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
`TRIBUNAL_PANEL_POLICY`, `TRIBUNAL_PASSES`, `TRIBUNAL_MAX_OUTPUT_BYTES`,
`TRIBUNAL_MAX_WALL_TIME`, and `TRIBUNAL_TOKEN_BUDGET`. `TRIBUNAL_PANEL` and
`TRIBUNAL_PANEL_POLICY` are mutually exclusive. Adapter credentials use the
environment-variable name selected by trusted user configuration and are
redacted from all output.

## OpenAI-compatible adapter

Trusted config supplies `base_url`, `model`, optional static headers, context
budget, and `api_key_env`. Requests use temperature 0, a role JSON schema,
bounded response reads, and no automatic redirect to another origin.

## Panel policies and the model catalog

`panel_policy = "<name>"` composes the panel from a policy instead of the
verbatim `panel` string. `tribunal panel list` shows built-in policies
(`balanced`, `high-stakes`, `frugal`) and any `[[policies]]` from trusted
configuration; a configured policy shadows a built-in of the same name.
`tribunal panel show` resolves the panel a review would use without freezing a
packet or calling a model.

`[[models]]` declares the selectable catalog. `quality`, `reliability`, and
`cost` are operator-declared priors — Tribunal measures none of them and ships
no vendor figures of its own. Without a catalog, candidates are derived from
the configured panel with uniform priors, and every resolved panel discloses
that derivation.

```toml
panel_policy = "balanced"

[[models]]
id = "opus"
adapter = "claude"
model = "claude-opus-5"
family = "anthropic"
capabilities = ["reasoning", "document-analysis"]
quality = 0.95
reliability = 0.95
cost = 1.0

[[policies]]
schema_version = 1
name = "house-audit"
summary = "Expert and foundation lenses, with an optional adversarial seat."
minimum_panel = 3
independent_families = 3
diversity_weight = 0.6
reliability_weight = 0.3
cost_weight = 0.2

  [[policies.roles]]
  name = "expert-reviewer"
  persona = "methodologist"
  prefer = ["reasoning", "document-analysis"]

  [[policies.roles]]
  name = "foundation-reviewer"
  persona = "foundation"
  prefer = ["literal-reading", "instruction-following"]

  [[policies.roles]]
  name = "adversarial-reviewer"
  persona = "skeptic"
  prefer = ["contradiction-detection"]
  optional = true
```

`family` is the independence unit and defaults to the adapter. `require`
capability tags are a hard filter and `prefer` tags are ranked, earlier first.
An optional role that cannot be filled is dropped and reported; a non-optional
one fails the run before any packet is frozen.
