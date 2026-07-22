# I/O Contract

## Inputs

`review` and `recommend` accept one existing regular file or directory. Native
text formats are `.md`, `.markdown`, and `.txt`; `.docx` and `.pdf` are
extracted for review. Direct binary editing is rejected. Folder logical paths
are slash-separated relative paths sorted lexically.

Panel strings use `adapter/model[@persona]` and enter via flag, environment,
or config key. Personas are schema-versioned TOML; rubrics are built-in
(generic, manuscript, strategy, governance). Model responses are JSON and
validated against the exact role schema.

## State location

Default:

`~/.local/state/tribunal/<workspace-id>/runs/<run-ulid>/`

Workspace files are never used as runtime pointers. Workspace-level
`findings.json`, `decisions.jsonl`, aliases, active, and latest projections live
under the workspace state directory. Directories are mode 0700 and regular
artifacts 0600 where supported.

## Run artifacts

Core files are `packet.json`, `meta.json`, `state.json`, `events.jsonl`,
`findings.json`, `votes.json`, `arbitration.json`, `report.md`, optional
`report.html`, and `final.json`. Model call directories preserve prompt,
delivery, raw output, parsed output, and validation error/retry artifacts.

All JSON carries `schema_version`. Finding objects use version 2; other initial
Tribunal schemas use version 1. Unknown/missing versions fail with a preflight
error. Readers ignore unknown fields within a supported version.

## Atomicity and overwrite

Canonical JSON snapshots use temp file, file sync, rename, and directory sync.
Journal and decision records append complete JSON lines and sync; a torn
trailing line left by a crash is quarantined to a `.corrupt` sidecar and the
journal truncated to its last complete record on the next append, while
readers skip the torn fragment. Review never writes to the artifact root.
Edit refuses stale sources; a leftover recovery backup with identical content
is reused on retry and a mismatched one is archived, never destroyed. Revert
refuses a live file changed since the Tribunal edit.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | recommendations complete without accepted blocker/major |
| 1 | accepted blocker/major exists |
| 2 | arbitration pending |
| 3 | degraded; inspect `degraded_reason` and `panel_status` |
| 4 | invalid arguments |
| 5 | preflight failure |
| 6 | aborted by budget, timeout, or user |
| 7 | internal runtime fault |

In `--json` mode a failed command emits a schema-versioned error envelope on
stdout — `{"schema_version": 1, "error": "...", "exit_code": N}` — whenever no
result object was produced; a populated result (for example a degraded final)
still renders alongside its non-zero exit code. Flag and argument parse errors
print usage to stderr and exit 4 without an envelope.

