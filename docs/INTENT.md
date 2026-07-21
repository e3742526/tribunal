# Tribunal Intent Charter

## Mission and audience

Tribunal helps authors, researchers, strategists, and governance teams inspect
high-stakes documents through independent AI review without surrendering the
decision or modification boundary to a model.

## Primary workflow

Given a document or document folder, Tribunal freezes and hashes a canonical
packet, obtains independent structured reviews, resolves anchors, gathers blind
votes, and writes an inspectable recommendation or arbitration packet. A user
may later accept findings and invoke a separate guarded edit operation.

## Success

- The same effective packet is evidenced for every reviewer.
- Findings, evidence, votes, dissent, decisions, and degraded behavior are
  schema-versioned and attributable.
- Review works without Git and leaves the reviewed workspace unchanged.
- Crashes, malformed model output, missing reviewers, stale files, hostile
  paths, and failed persistence never become silent success.
- A cold contributor can operate and extend the project from repository docs.

## Binding non-goals

Code review, diff capture, test execution, autonomous authorship, automatic
edit merging, chain-of-thought storage, hidden telemetry, open network access,
Tagteam compatibility, and an arbiter that replaces user judgment.

## Scope-pressure order

Preserve packet integrity, read-only review, consensus correctness, persistence,
and edit safety. If scope must contract, cut TUI polish, HTML styling, optional
worker providers, imported personas, and additional document extraction before
weakening those primary guarantees. Every cut requires a plan-change record.

This charter changes only through `docs/build/plan-changes.md`.

