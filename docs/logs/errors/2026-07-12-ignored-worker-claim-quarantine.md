# 2026-07-12: Ignored worker claim quarantined a valid partial patch

## Observed failure

Dory consumer run `2026-07-12T132406.074298000Z` completed a 19-file
implementation, but the worker included a repo-required, gitignored session log
in `files_changed`. Tagteam correctly excluded that file from its Git-derived
review artifact, rejected the inconsistent worker contract, and quarantined the
partial patch after recovery selected quarantine.

The run also used `uv run pytest -q` in Tagteam's isolated test environment.
Dependency resolution failed because Dory's unpublished optional
`control-hooks` dependency was unavailable. This is a repository test-command
selection issue rather than an adapter failure; continuation should use the
repository's already-provisioned virtual environment.

## Preservation

The partial patch and exact contract diagnostics remain under:

`~/.local/state/tagteam/ece01021b48cd2ae4996ca4b/runs/2026-07-12T132406.074298000Z/`

The Dory feature worktree retains the same edits. No ignored file content was
copied into Tagteam review artifacts.

## Repair

Worker prompts now state that `files_changed` must match Git-visible changes,
that ignored local-only outputs belong in `remaining_risks`, and that an ignored
artifact intended for review must be explicitly staged with `git add -f`.

A future contract-only repair path is tracked in `docs/TODO.md`; it must allow
metadata correction without reopening repository editing after a partial patch.
