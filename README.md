# Tribunal

Tribunal is a Git-independent CLI for independent, evidence-aware review of documents. It freezes a deterministic packet, asks a panel for independent findings, persists every first-pass result before sharing anonymous findings, collects blind votes, computes consensus, and records dissent and arbitration.

Tribunal never edits during `review`. Editing is a separate, explicit operation: models may propose typed replacement hunks, but the host validates accepted finding IDs, source hashes, canonical paths, UTF-8 boundaries, and scope before an atomic write. Run state lives outside the reviewed documents.

Current version: `v0.1.0`.

## Install

Requirements are Go 1.23 or newer and at least two configured review adapters. PDF review additionally requires Poppler's `pdftotext`.

From a checkout:

```bash
go build -o ./bin/tribunal .
export PATH="$PWD/bin:$PATH"
tribunal doctor
```

After a release is published, `go install github.com/e3742526/tribunal@v0.1.0`
installs the same command.

Provider CLIs are detected from `PATH`:

- `codex` for Codex models;
- `claude` for Claude models;
- `agy` for Gemini models;
- `openai-compatible` for an HTTP endpoint configured by the user.

## Quickstart

Review one Markdown, text, DOCX, or PDF document:

```bash
tribunal review proposal.md
```

Review a folder. Supported documents are walked lexically; hidden paths are skipped:

```bash
tribunal review ./policy-packet --kind governance --split
```

Use a different built-in rubric:

```bash
tribunal recommend manuscript.md --rubric manuscript
```

Inspect the latest run:

```bash
tribunal status manuscript.md
tribunal transcript manuscript.md
tribunal findings list manuscript.md
```

Machine-readable output is available on every command:

```bash
tribunal review proposal.md --json
```

## Panel grammar

The exact grammar is `adapter/model[@persona]`, comma-separated. Model names may contain slashes, colons, spaces, or parentheses.

```bash
tribunal review proposal.md \
  --panel 'claude/claude-opus-4-8,codex/gpt-5.6-sol,agy/Gemini 3.5 Flash (Medium)'
```

The default panel uses those three adapter families with weight `1.0` and the `plain` persona. A panel must retain a majority quorum with at least two valid reviewers. Missing or malformed reviewers are reported as degraded; they are never silently replaced.

## Arbitration

On a terminal, `tribunal arbitrate` prompts for each pending dispute. In automation, supply a schema-versioned file:

```json
{
  "schema_version": 1,
  "run_id": "01J...",
  "rulings": [
    {
      "id": "A-1234abcd",
      "outcome": "accepted",
      "reason": "The cited policy controls this case.",
      "operator": "eric"
    }
  ]
}
```

```bash
tribunal arbitrate proposal.md --decisions decisions.json
tribunal arbitrate proposal.md --accept-majority --except A-1234abcd --operator eric
```

Rulings are appended to decision memory using the exact document item and finding fingerprint. A prior ruling can inform a later default, but does not silently decide a new dispute.

## Editing and revert

Generate and validate an edit proposal without changing a document:

```bash
tribunal edit proposal.md
```

Apply a validated proposal:

```bash
tribunal edit proposal.md --proposal edit-proposal.json --apply
```

Only plaintext and Markdown items are editable. Local scope is limited to 256 bytes around accepted anchors; section scope is limited to the containing Markdown section; document scope requires `--confirm-document-scope`. Backups and edit records stay in the external run directory.

```bash
tribunal revert proposal.md
```

Revert first verifies the live document still matches Tribunal's edited hash. It refuses to overwrite subsequent user changes.

## Configuration

Trusted user configuration is `~/.config/tribunal/config.toml`. Workspace `.tribunal.toml` is ignored unless `--trust-workspace-config` is explicit. Workspace `.env` files are always ignored.

```toml
schema_version = 1
panel = "claude/claude-opus-4-8,codex/gpt-5.6-sol,agy/Gemini 3.5 Flash (Medium)"
kind = "generic"

[limits]
passes = 2
max_findings = 25
max_output_bytes = 1048576
call_timeout = "15m"
run_timeout = "60m"
token_budget = 500000
max_verification = 10
max_arbitration = 10
max_context_tokens = 131072
reserved_output_tokens = 16384

[openai_compatible]
base_url = "http://127.0.0.1:11434/v1"
model = "gemma4:latest"
api_key_env = ""

[workers]
allowed_domains = ["api.crossref.org", "pubmed.ncbi.nlm.nih.gov", "export.arxiv.org"]
```

Recognized environment variables use only the `TRIBUNAL_` prefix: `TRIBUNAL_STATE_ROOT`, `TRIBUNAL_PANEL`, `TRIBUNAL_PASSES`, `TRIBUNAL_MAX_OUTPUT_BYTES`, `TRIBUNAL_MAX_WALL_TIME`, and `TRIBUNAL_TOKEN_BUDGET`.

Precedence is flags, shell environment, explicitly trusted workspace config, user config, then built-in defaults.

## State and privacy

Default state root:

```text
~/.local/state/tribunal/<workspace-id>/
├── active.json
├── latest.json
├── aliases.json
├── findings.json
├── decisions.jsonl
└── runs/<ulid>/
    ├── packet.json
    ├── packet-manifest.json
    ├── redacted-snapshot/
    ├── meta.json
    ├── calls/<reviewer>/<phase>/
    ├── worker-findings.json
    ├── clusters.json
    ├── votes.json
    ├── arbitration.json
    ├── report.md
    ├── report.html
    ├── events.jsonl
    ├── state.json
    └── final.json
```

The state root must be outside the document tree. Selected symlinks, special files, path-boundary changes, stale edit hashes, and unknown schema versions fail closed. Redaction is length-preserving so anchors and edit offsets remain stable. Use `--fail-on-secret` to reject instead.

Tribunal does not read or migrate state from any predecessor product.

## Commands

```text
review, recommend
arbitrate, edit, revert, resume, replay, explain
findings list, findings defer, decisions export
status, transcript, tui
persona list, persona new, persona lint
bench, doctor, adopt
version, verify-install
```

`resume` restarts an incomplete run from its already frozen packet. `replay` creates a new ULID while preserving the recorded packet hash and panel. `adopt` creates external identity metadata without writing into the folder.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Clean review or accepted recommendations below major severity |
| 1 | Accepted major or blocker finding |
| 2 | Arbitration pending |
| 3 | Degraded panel |
| 4 | Invalid arguments or schema input |
| 5 | Preflight or persistence failure |
| 6 | Aborted run or refused unsafe mutation |

## Development

```bash
scripts/check.sh
go test -race ./...
```

Architecture, contracts, ADRs, requirements, and test evidence are indexed in [docs/INDEX.md](docs/INDEX.md). Security reports should use GitHub private vulnerability reporting.
