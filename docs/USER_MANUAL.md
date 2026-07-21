# User manual

## Review lifecycle

`tribunal review <input>` canonicalizes the selected file or folder, rejects
unsafe file types and path changes, extracts supported content, redacts known
secret/PII patterns, and hashes the frozen packet. All reviewers receive the
same content or the same deterministic chunk sequence.

Every first-pass result is persisted or classified failed before anonymous
findings are constructed. Tribunal then resolves anchors, runs deterministic
spelling/reference checks, clusters overlapping findings by category, shuffles
the blind packet deterministically, collects votes, computes consensus, and
writes Markdown/escaped HTML reports.

Review never writes into the input folder. `--fail-on-secret` rejects instead
of redacting. `--split` is required when the packet exceeds the smallest panel
context budget.

## Interrupted and degraded runs

Use `status` and `transcript` with `--run <ULID>` or omit it for the latest run.
A run aborted by cancellation or its wall-time cap preserves all available
results and terminal state. `resume` reuses its frozen packet and recorded
panel. `replay` creates a new run from that exact packet.

Degraded output means fewer than a majority, or fewer than two, reviewers
produced valid first-pass results. Independent findings remain visible but are
not misrepresented as consensus.

## Arbitration

Interactive arbitration is available on a TTY. Automation must use a versioned
decisions file or `--accept-majority`; `--except` leaves selected disputes
pending. Every applied ruling needs an operator and reason and is appended to
workspace decision memory.

## Findings ledger

`findings list` shows stable fingerprint records. Major/blocker records remain
open until explicitly handled. `findings defer` requires both `--reason` and
`--operator`; omission is an argument error.

## Personas

User personas live below `~/.config/tribunal/personas`. Workspace personas live
in `.tribunal/personas` and are structured-only. `persona lint --workspace`
rejects freeform lenses and all personas reject voting, role, tool, schema, or
permission directives.

## Edit and revert

`edit` defaults to a proposal-only dry run. `--apply` is the explicit mutation
boundary. Each hunk must name accepted findings, the packet item, the original
source hash, a scope, and byte offsets. PDF and DOCX are review-only.

Backups live inside the external run state. `revert` checks both the backup hash
and the live edited hash before restoration. If the document changed after the
edit, resolve it manually; Tribunal will not overwrite it.

## Operational commands

`doctor` detects provider CLIs and `pdftotext`. `bench` runs a planted
statistics/citation/instruction-injection fixture unless a fixture document is
supplied. `adopt` initializes external identity metadata without a workspace
write. `verify-install` validates build metadata and the adjacent SHA-256
manifest for release binaries.
