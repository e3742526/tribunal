# Tribunal Hardened Specification v3.1

Status: implementation contract. This repository is the clean-break Tribunal
fork founded from Tagteam `9dc1982` on 2026-07-21.

## Mission

Tribunal is a Git-independent CLI for high-stakes document review. It freezes a
canonical document packet, sends the same effective input to independent model
reviewers, validates and clusters their findings, records blind votes and
dissent, and publishes recommendations or a bounded user-arbitration packet.
Review is read-only by default. Editing is a separate, explicitly accepted,
hash-checked operation.

## Binding invariants

1. Documents only: Markdown, plaintext, DOCX, and PDF. Code review, diffs,
   repositories, test execution, coding agents, and Git are outside the runtime.
2. Pass-1 independence is host-enforced. No reviewer sees peer output before
   every pass-1 result is persisted or classified failed.
3. Every persisted JSON object carries `schema_version`; missing and unknown
   versions fail closed. There is no field-presence compatibility inference.
4. Packet content, persona text, fetched evidence, workspace configuration,
   paths, and symlinks are untrusted input.
5. A run is identified by a ULID and a packet by SHA-256 over a canonical
   manifest. Workspace identity is the first 24 hex characters of SHA-256 over
   the canonical artifact-directory path.
6. Runtime state lives under
   `~/.local/state/tribunal/<workspace-id>/`; review never writes beside the
   documents.
7. Every sensitive read/write revalidates its canonical root. State roots inside
   document roots, symlinked document entries, and escaping paths are rejected.
8. Model output never directly controls host capabilities. It is parsed through
   bounded JSON recovery, JSON Schema, and semantic invariants.
9. Reviewers review and vote; workers fetch/check; editors only propose edits
   for accepted findings; arbiters only rank. Permissions are enforced in host
   argv, environment, working directory, and typed inputs.
10. Findings preserve provenance, anchors, evidence status, votes, dissent, and
    lifecycle. Chain-of-thought is never requested or stored.
11. Quorum or silence: below majority quorum (minimum two), independent findings
    are published but no consensus is claimed.
12. All loops, calls, outputs, findings, verification tasks, arbitration items,
    time, and token use are capped.

## Roles and panels

Panel entries use `adapter/model[@persona]`. The adapter ends at the first `/`;
the optional persona is a trailing slug matching `[a-z0-9-]{1,64}`; the model
between them is preserved verbatim. Weighted panels use a TOML panel file and
weights are clamped to 0.5–2.0. Default panel: Claude Opus 4.8, Codex GPT-5.6
Sol, and Agy Gemini 3.5 Flash Medium, each weight 1.0.

Reviewers receive a fenced role prompt, rubric, persona, and packet only. Worker
evidence gathered before review is part of the packet. Post-review verification
evidence has a separate hash and is included in every vote packet. Workers never
vote and cannot originate blocker/major findings without reviewer adoption.

## Packet and delivery

Plaintext inputs preserve bytes and require UTF-8. Folder walks are lexical and
support `.md`, `.markdown`, `.txt`, `.docx`, and `.pdf`. Hidden metadata is
skipped; selected symlinks and special files fail preflight. DOCX is extracted
with ZIP/XML; PDF uses bounded `pdftotext` and is review-only.

The packet contains a redacted/extracted snapshot, manifest, rubric, pre-review
evidence, original source hashes, packet item hashes, and a top-level packet
hash. Secret scanning combines known environment-value redaction with common
credential/PII patterns and entropy checks. Redaction is length-preserving and
recorded without the secret value. `--fail-on-secret` aborts before packet
content is persisted.

Context preflight uses the smallest configured reviewer budget. Oversize input
fails unless `--split`; splitting is deterministic at UTF-8 paragraph/section
boundaries and every reviewer receives all chunks in identical order. Every
call records delivered item/chunk IDs and truncation status.

## Findings and consensus

Finding schema version 2 includes reviewer, persona, origin, severity, category,
quote/section anchor, issue, recommendation, evidence IDs, evidence status, and
ordinal confidence. Anchors resolve exactly, then by bounded prefix/suffix fuzzy
matching. Unresolved findings are quarantined and excluded from voting.
Unevidenced findings are capped at minor unless evidence attaches.

Default clustering merges only matching categories with overlapping anchors.
Optional model clustering may group but never delete/rewrite member findings.
Voting is identity-blind and deterministically shuffled. Acceptance requires
more than half of non-abstain votes and at least two such votes; ties and
insufficient votes require arbitration. Severity is the median ordinal vote.
Rejecting an accepted finding or voting two severity levels from the median is
recorded dissent. Confidence never affects acceptance.

Security, data-loss, and citation-integrity require unanimity of the full
configured panel. Factual claims require majority plus anchored or worker-
verified evidence; otherwise they publish as unverified recommendations. An
incomplete panel cannot claim unanimity.

Arbitration is capped at ten disputes ranked by severity and vote closeness.
The user remains the final authority. Decision memory records rulings by finding
fingerprint and later panels must acknowledge matching rulings.

## Lifecycle, persistence, and limits

Lifecycle:

`INIT -> PACKET_BUILT -> REVIEWING -> REVIEWED -> [VERIFYING] -> CLUSTERED ->
VOTING -> CONSENSUS | ARBITRATION_PENDING | DEGRADED -> RECOMMENDED ->
[EDIT_PENDING -> EDITED -> REREVIEWING] -> FINAL`.

Every transition appends and fsyncs `events.jsonl` before atomically replacing
`state.json`. Terminal success requires durable terminal state and `final.json`.
Resume reacquires locks, validates schema/path/hash checkpoints, and continues
the first incomplete idempotent step. Run/provider locks use kernel `flock` on
macOS/Linux and fail closed.

Defaults: 2 passes (max 3), 25 findings/reviewer, 1 MiB/call, 15 minutes/call,
60 minutes/run, 500k tokens/run, 10 verification checks, and 10 arbitration
items. Provider invocation failures receive one fresh panelist invocation;
contract failures receive one validation-error retry. All fallback use is
recorded in `reason_codes`.

Exit codes: 0 clean recommendations, 1 accepted blocker/major, 2 arbitration
pending, 3 degraded, 4 invalid arguments, 5 preflight, 6 aborted.

## Editing

Editing is a separate command over accepted findings. The editor sees accepted
findings, anchored regions, minimal context, and explicit local/section/document
scopes. It emits typed replacement hunks. The host rejects hunks outside the
allowlist, revalidates paths, and compares the live source hash to the packet
source hash. Apply is dry-run first, then durable atomic replacement with the
original preserved. Document-scope edits require per-hunk confirmation or a
mandatory rereview. Revert refuses if the edited file changed afterward.

## Configuration and commands

Precedence is defaults < user config < explicitly trusted workspace config <
shell `TRIBUNAL_*` < flags. Workspace config is ignored unless trusted and a
workspace `.env` is never loaded. Authority-bearing workspace values are shown
before invocation and hashed into metadata. Secrets enter only through shell
variables selected by trusted configuration.

Commands: `review`, `recommend`, `arbitrate`, `edit`, `revert`, `resume`,
`replay`, `explain`, `findings list|defer`, `decisions export`, `status`,
`transcript`, `persona list|new|lint`, `bench`, `doctor`, `adopt`, `tui`,
`version`, and `verify-install`. Every command supports `--json`.

## Deferred and rejected

No Git compatibility, Tagteam state migration, code kind, autonomous authoring,
automatic edit merge, open-by-default web access, URL persona imports, MCP/control
transport, Windows release, or arbiter model that overrules the user.

