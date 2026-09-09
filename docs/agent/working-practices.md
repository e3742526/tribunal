# Working practices (tribunal)

Non-binding rationale and posture moved out of `AGENTS.md` so the canonical
contract stays short. `AGENTS.md` remains authoritative; nothing here overrides
it.

## Why the contract is short

Capable models already inspect before editing, prefer minimal diffs, avoid
presenting stubs as finished work, and report validation honestly. The
`AGENTS.md` Normative Core keeps those rules in one compact block because they
are the ones this repo enforces; the repeated coaching that surrounded them was
removed rather than restated.

## Working posture (advisory)

- Prefer inspection before modification and minimal diffs over rewrites.
- Prefer extending existing patterns over inventing parallel ones.
- Prefer real wiring over mock structure, and validation evidence over claims.
- Surface conflicting patterns instead of averaging them: record the conflict
  and make the smallest reversible local choice.
- Stop at destructive actions, unresolved ambiguity, or conflicting repo state
  and report the next safe step.

Push back when implementation is requested before the relevant architecture or
workflow is understood; when UI or CLI behavior is added before the execution
path is clear; when contracts, persistence, or external integration behavior is
unclear; or when work is declared complete without end-to-end validation for
the changed path. Pushback is advisory unless the task asks for a hard stop.

## Sequence guidance for large changes

1. Identify the entry points, contracts, and main execution path.
2. Inspect relevant files and nearby tests.
3. State assumptions, dependencies, and likely failure modes.
4. Implement only after the dependency chain is clear enough to act safely.
5. Validate the result against the changed path, not just static structure.

## What counts as an incomplete implementation

Treat as incomplete unless explicitly requested: buttons or commands with no
real handler path; routes or commands returning canned success without doing
the work; services that only wrap TODOs; mock or placeholder data presented as
real output; "security" that hides fields in the UI without backend
enforcement; workflow support that does not execute end to end; and tests that
only assert trivial truth or import success. Call these out when they exist in
touched scope.

## Testing guidance

Add tests when the change introduces reusable logic, non-trivial branching,
cross-module interactions, data transformations, or a bug fix that should not
regress. Tests are optional for exploratory code, trivial behavior fully
validated by direct execution, and purely presentational scaffolding. A test
should validate behavior, stay focused, and fail if the core behavior breaks.

## Completion language

Prefer `implemented`, `partially validated`, `statically consistent`,
`runtime-verified for the tested path`, `residual risks remain`. Avoid `done`
for partial validation, `fully working` after static checks only, `wired` for
references alone, `secure` without boundary review, and `production-ready`
without operational evidence.
