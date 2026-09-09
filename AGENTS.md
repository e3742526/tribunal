# AGENTS

## Normative Core

`AGENTS.md` (uppercase, plural) is the single canonical repo-wide agent
contract for **tribunal**. Tool-specific adapter files (`CLAUDE.md`,
`CODEX.md`, `GEMINI.md`, `.claude/AGENTS.md`, `.codex/AGENTS.md`) may add
preferences but must not redefine or weaken this file; on conflict, this file
wins. Rationale and non-binding working practices live in
`docs/agent/working-practices.md`.

### Binding rules

- Inspect before modifying. Do not invent APIs, files, config keys, commands,
  or paths without verifying they fit the repo.
- Preserve existing naming, formatting, architecture, and test conventions
  unless the task explicitly changes them.
- One coherent task per run. Make the smallest coherent change, stay in scope,
  and disclose adjacent edits.
- No fake success: never present a stub, placeholder, or canned response as
  working, and do not suppress errors to make tests or logs look clean.
- Validate before completion. State exactly what passed, failed, or was not
  run; say `partially validated` when that is the truth.
- Do not delete or overwrite user work; never revert existing changes without
  explicit instruction.
- Uncommitted files are repository state, not a failure, unless the task
  requires a clean tree.
- Pushback on unclear architecture or unvalidated completion is advisory
  unless the task explicitly asks for a hard stop. A documented exception in
  `.architecture/exceptions.json` is reported as covered, not rediscovered.

### Read order for non-trivial changes

1. `AGENTS.md`
2. the active tool's adapter file, if present
3. `README.md`
4. `docs/INDEX.md`
5. task-relevant docs, tests, and nearby implementation files

## Private agent-skills catalog

For a task that may match a reusable workflow — audit, repair, planning,
governance, research, writing, infrastructure, acquisition, or graphics —
search the private agent-skills catalog before improvising, including this
repository root for repository work. If the catalog is unavailable, report that
fact and continue within the task's normal constraints.

## Tribunal Architecture

Dependencies point inward: `main` → `internal/cli` → `internal/tribunal/app`.
The application layer coordinates pure `domain` rules, `documents`, `storage`,
`config`, and `adapters`; `internal/tui` renders only `storage.Snapshot`.

Trust boundaries:

- Reviewed documents are untrusted input.
- Review code must not execute Git or write into the document workspace.
- Model editors propose hunks; the host alone validates and applies them.

The dependency graph is declared in `.architecture/intent.json` and enforced
by `scripts/check-architecture.sh` (registry completeness, allowed production
dependencies, exception expiry, baseline accounting and health score) against
`.architecture/invariants.json`, `.architecture/exceptions.json`, and
`.architecture/baseline.json`.

## Validation gate

Run `scripts/check.sh` before completion. It runs, in order: `gofmt -l`,
`scripts/check-go-file-lines.sh`, `scripts/check-architecture.sh`,
`go test ./...`, `go vet ./...`, `go build ./...`, `go mod verify`,
`go mod tidy -diff`, and `govulncheck ./...` (required in CI, reported as
skipped when absent locally).

No non-test Go file may exceed **800 lines**.

## Test ledger

`docs/TEST_LEDGER.md` records, per area, the runtime evidence and the explicit
remaining gap. Changes to schemas, prompts, adapters, persistence, or edit
validation require focused behavior tests, and any change that alters what is
proven — or which gap remains open — must update the matching ledger row in the
same change set.

## Compact checklist

1. Read `AGENTS.md`, then the adapter file, `README.md`, and `docs/INDEX.md`.
2. Search the private agent-skills catalog if the task matches a known
   workflow.
3. Read the existing code and nearby tests before editing anything.
4. Do one coherent task; keep the diff the smallest that works.
5. Keep dependencies pointing inward and never let review code run Git or
   write into the document workspace.
6. Add focused behavior tests for schemas, prompts, adapters, persistence, or
   edit validation.
7. Update the matching row in `docs/TEST_LEDGER.md`.
8. Run `scripts/check.sh` and report what passed, failed, or was skipped.
