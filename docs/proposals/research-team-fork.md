# Proposal: Tribunal as a multi-agent research team (separate fork)

Status: proposed, not started. No code in this repository implements any part
of this yet. This document records the plan discussed for turning Tribunal's
review/consensus core into a foundation for a multi-agent research team, to be
built in a separate fork/repo rather than merged upstream here.

## Goal

Several agents (Claude, GPT, Gemini, Mistral, Grok, Qwen, Llama, or as many as
are configured) independently scour for information on a topic, present
findings with evidence and provenance, and reason about them — with
disagreement across agents surfaced explicitly rather than hidden or averaged
away.

## What Tribunal already provides

- `domain.Decision`/`Dissent`/`ArbitrationDispute` (`internal/tribunal/domain/types.go`)
  already model disagreement as a first-class, undissolved record —
  `WeightedLean` and `Dissent` exist specifically so conflict isn't averaged
  away.
- Blind independent first pass, persisted before any sharing
  (`internal/tribunal/app/review.go`) — matches "each agent digs
  independently, then compare."
- `EvidenceItem` + `Anchor` + per-reviewer provenance artifacts
  (`calls/<reviewer>/<phase>/`, redacted snapshots, content hashes) — matches
  "evidence and provenance."
- `adapters.WorkerService` (`internal/tribunal/adapters/workers.go`) already
  has SSRF guards, domain allowlisting, and redirect revalidation — a safe
  base to extend rather than a new fetch layer to build.
- Panel policies with `independent_families` diversity weighting
  (`internal/tribunal/app/panel_selection.go`,
  `internal/tribunal/config/config.go`) — already designed to force
  model-family spread.

## Gaps and phased plan

### Phase 0 — Adapter breadth

Today: 3 subprocess adapters (`codex`, `claude`, `agy`) plus **one**
`[openai_compatible]` HTTP endpoint (`config/config.go`). That caps concurrent
model families well below "all my agents."

- Change `OpenAICompatible` from a single struct to a named list
  (`[[openai_compatible]]` with an `id`) so Mistral, Grok, Qwen, Llama, etc.
  (via Ollama, OpenRouter, Together, or vendor APIs) each become
  independently addressable panelists, e.g.
  `openai-compatible:mistral/mistral-large`.
- Extend `domain.ParsePanel` and the adapter registry (constructed in
  `app/service.go`) to fan out to N configured instances instead of one.
- Extend `config.normalize` to validate each entry the way the single entry
  is validated today.

Lowest risk, ships independently, and already gets "seat as many agents as
are configured" against the existing review pipeline.

### Phase 1 — A real gather/scour phase

The actual gap: Tribunal reviews a frozen document; it doesn't yet have
agents independently searching for information. The only existing hook,
`Workers.WebSearchURL` plus `search:` evidence references
(`app/verification.go`), fires **after** review, to verify a citation a model
already wrote — not to let each agent go look for things.

- **1b (do first):** move evidence gathering before the review pass. Add a
  `RoleScout` adapter role alongside `RoleReviewer/Voter/Editor/Arbiter`
  (`adapters/adapter.go`). Each panelist proposes typed queries; the **host**
  (not the model) executes them through `WorkerService`, preserving the
  existing trust boundary ("model proposes, host validates and applies").
  Results become `EvidenceItem`s attached to the run before reviewing starts,
  so panelists still review blind to each other but against a shared, hashed
  evidence set.
- **1a (do after 1b is proven):** a `research "<question>"` mode with no
  input document — the host synthesizes the packet from a question instead
  of freezing a file. This is the open-ended "scour the web" case and is
  architecturally new.

Fetched web content must go through the same redaction/quarantine path
reviewed documents already use — "reviewed documents are untrusted input" is
not specific to files.

Open question: does the fetch allowlist stay global config
(`Workers.AllowedDomains`), or become per-run/per-topic? Global is what
exists; per-run is safer for open research but is new plumbing.

### Phase 2 — Surface the tension by default

Nothing missing in `domain` — `ArbitrationDispute.ForArgument`/`Against`
already capture competing positions. What's missing: this only surfaces via
`tribunal arbitrate` today. `report.go`'s `report.md`/`report.html` should
render a "conflicting evidence" section by default, listing disputes and
both arguments inline, rather than only on demand.

### Phase 3 — CLI/TUI wiring

New `research` command following the `newReviewCommand` pattern
(`internal/cli/review_commands.go`), or a `--mode research` flag on `review`.
`internal/tui` renders only `storage.Snapshot`, so new phases/evidence need
to land in that snapshot type.

### Process constraints (apply to every phase)

- `review.go` is already near the 800-line non-test file cap
  (`AGENTS.md`); scout logic goes in a new file, not grown into it.
- Schema, prompt, adapter, and persistence changes each require focused
  behavior tests per `AGENTS.md`.
- Stays document-only / state-root-only per `CLAUDE.md` — no Git writes, no
  change to "host alone validates and applies edits."
- `scripts/check.sh` at each phase boundary.

### Suggested order

1. Phase 0 — adapter breadth.
2. Phase 1b — pre-review evidence gather.
3. Phase 2 — report surfacing.
4. Phase 1a — open-ended `research` command.

## Migration: separate fork and repo

The user intends to develop this as a separate fork/repo rather than as
changes merged into `e3742526/tribunal`. This repo is MIT-licensed
(`LICENSE`), so forking and renaming is unrestricted; keep the original
copyright notice.

Treat this as **Phase 0a**, ahead of Phase 0 above, so every subsequent
commit already lands on the renamed module path in the new repo instead of
requiring a later history rewrite.

1. **Repo creation — real fork, not a fresh squash.** Fork on GitHub (or
   clone this repo and add it back as an `upstream` remote, pushing to a new
   `origin`) rather than starting from clean history, so upstream
   bugfixes/security patches to the shared review/consensus/adapter core can
   still be pulled in later. A squashed fresh start forfeits that.

2. **Identity rename — module path, binary, state dirs.**
   - `go.mod`'s module line and every `github.com/e3742526/tribunal/...`
     import; verify with `go build ./...` and `scripts/check.sh` after.
   - CLI binary name and default state/config directory names (currently
     `tribunal`, defaulting to `~/.config/tribunal/config.toml` and
     `~/.local/state/tribunal/<workspace-id>/` per `config/config.go`) —
     rename both if upstream and fork might ever run on the same machine, to
     avoid silently sharing config/state.
   - `README.md`'s `go install github.com/e3742526/tribunal@v...` line and
     other self-referential URLs.

3. **Adapter files.** `CLAUDE.md`/`AGENTS.md` become the fork's own contract.
   Decide explicitly which upstream constraints to keep vs. relax rather than
   drifting silently. Most likely candidate: `CLAUDE.md`'s "Git-independent,
   document-only... do not add repository-writing review behavior" — not
   needed by the phased plan above, but worth flagging as a constraint that
   may be revisited later rather than inherited permanently.

4. **Upstream tracking cadence.** Keep an `upstream` remote and a
   `sync/upstream` branch merged periodically, then fast-forwarded/PR'd into
   the fork's `main`, so the shared core keeps getting upstream fixes.
   Feature branches (Phase 0-3) build on the fork's `main`, not directly on
   `sync/upstream`.

5. **CI/release.** The fork needs its own workflow files — check
   `.github/workflows/` for anything referencing the `e3742526/tribunal`
   repo/org name explicitly and repoint it.

## Open decisions

- Phase 1 day-one scope: 1b only, or sketch 1a in parallel?
- Evidence allowlist: stay global config, or become per-run?
- Fork identity: new binary/module name, and whether to relax the
  Git-independence constraint for a future repo-writing capability.
