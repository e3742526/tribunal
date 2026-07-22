# Tribunal cross-lens audit report

Audit date: 2026-07-21  
Repository: `/Users/eric/Work/vscode/tribunal`  
Revision: `4a8003c790857fbc3d7c5d7adacebda6fe030f32`  
Authority: read-only source audit; report artifacts only  
Catalog skills: `audit-architecture-drift`, `audit-failsafe-readiness`, `audit-security`, `audit-security-llm`

## Executive summary

Tribunal has a strong defensive foundation—external state, atomic file replacement, kernel locks, bounded subprocess output, process-group termination, exact-domain evidence-worker allowlists, private-address rejection, model-output schemas, anchor checks, and host-enforced edit scopes. The primary review and recovery protocols nevertheless contain material correctness and integrity gaps.

This audit records **11 findings: 5 High, 6 Medium**. Nine are source-confirmed implementation properties; two are Likely because either the architecture intent is inferred or a crash/runtime manifestation was not reproduced. The highest-priority defects are:

1. Voters receive anonymous findings but not the frozen document, rubric, or post-review evidence they are instructed to evaluate.
2. A successful HTTP fetch is treated as semantic verification of a finding without checking whether the source supports the claim.
3. `resume` restarts the full review pipeline and overwrites same-path call artifacts instead of continuing the first incomplete idempotent step.
4. Persisted packet content is trusted on resume/replay/edit without recomputing its canonical hash.
5. Applied edits can remain on disk when the subsequent lifecycle/publication step fails, and crash windows exist before a durable edit record.

Overall architecture-health score: **not computed**. The architecture audit requires a scope-bearing invariant registry to normalize violation mass; no `.architecture/` registry or baseline exists. Treating absent checks as 100 or assigning an arbitrary denominator would be false precision. Risk is **High**, driven by decision-integrity, recovery, and document-mutation paths. Trend and new-vs-known status are unavailable on this first bootstrap audit.

## Scope and method

The audit statically traced the CLI-to-application review path, model and evidence adapters, document packet creation, storage and lifecycle transitions, resume/replay, consensus, edit/revert, release workflow, architectural declarations, and nearby tests. It used the checked-in specification and ADRs as declared intent. Generated/vendor trees were excluded from structural judgments.

No target program, provider, test suite, network request, fault injection, or hostile payload was executed because the selected catalog skills are read-only. Existing tests were inspected as source evidence but were not rerun. Runtime consequences of crashes, process sandbox behavior, and external-provider semantics remain explicitly bounded by those validation limits.

## Findings overview

| Priority | ID | Severity | Confidence | Inventory | Finding |
|---:|---|---|---|---|---|
| 1 | ARC-TRIBUNAL-002 | High | Confirmed | AID-001, AID-009, LLM-014 | Vote packets omit the frozen document, rubric, and verification evidence |
| 2 | LLM-TRIBUNAL-001 | High | Confirmed | LLM-014, SEC-005 | Transport success is promoted to semantic evidence verification |
| 3 | FSR-TRIBUNAL-001 | High | Confirmed | FSR-009, FSR-014, SC-INT/SC-COR | Resume reruns and overwrites instead of continuing a checkpoint |
| 4 | SEC-TRIBUNAL-001 | High | Confirmed | SEC-005, SEC-008 | Persisted packet identity is not cryptographically revalidated |
| 5 | FSR-TRIBUNAL-002 | High | Likely | FSR-005, FSR-014, FSR-015, SC-INT | Edit/revert mutations are not transactionally coupled to lifecycle records |
| 6 | LLM-TRIBUNAL-002 | Medium | Confirmed | LLM-012, FSR-007 | Token budget is an estimate, not a runtime circuit breaker |
| 7 | ARC-TRIBUNAL-003 | Medium | Confirmed | AID-009 | Unevidenced major/blocker findings are capped only for one category |
| 8 | ARC-TRIBUNAL-004 | Medium | Confirmed | AID-009, AID-011 | Persisted readers validate outer versions but accept unknown nested versions |
| 9 | FSR-TRIBUNAL-003 | Medium | Confirmed | FSR-009, FSR-012, SC-INT | Workspace ledger is updated before terminal publication succeeds |
| 10 | SEC-TRIBUNAL-002 | Medium | Confirmed | SEC-009 | Release archive smoke occurs after the release is published |
| 11 | ARC-TRIBUNAL-001 | Medium | Likely | AID-012, AID-014 | No machine-evaluable intent, invariant, exception, or baseline registry |

## Detailed findings

### ARC-TRIBUNAL-002 — Vote packets omit the frozen document, rubric, and verification evidence

Severity: High  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Trace basis: declared specification

Evidence:

- `docs/SPEC.md:53-56` requires reviewers to receive the packet and requires post-review evidence in every vote packet.
- `internal/tribunal/app/review.go:105-115` collects `verificationEvidence`, but `internal/tribunal/app/review.go:134` calls `votePass` with only `packet` and clustered findings.
- `internal/tribunal/app/prompts.go:52-55` serializes only packet hash, voter ID, and anonymous findings.
- `internal/tribunal/app/review.go:551-560` records finding IDs and a blind finding packet, not document/chunk/evidence delivery.

Observed: voters are told to evaluate findings against the “same frozen packet,” but the vote prompt contains no document content, rubric, chunk map, pre-review evidence, or post-review verification evidence. A finding's own model-authored quote and prose are the only substantive context.

Expected boundary: every voter must receive the identical frozen review packet plus the separately hashed verification evidence, while identity-blinded findings remain the only peer-derived material.

Failure mechanism: the orchestration drops the packet/evidence at the review-to-vote boundary. A plausible-sounding but incorrect finding can therefore collect votes without the voter independently checking the source text or evidence.

Impact: consensus, severity median, dissent, and arbitration inputs are not grounded in the artifact they purport to adjudicate. This undermines Tribunal's primary decision product.

Recommended repair: define a schema-versioned vote packet that contains packet identity, canonical rubric, delivered item/chunk IDs and content, pre-review evidence, post-review evidence plus verification hash, and blinded findings. Record this full delivery and assert byte/hash equality across voters.

Validation: a vote-adapter test must fail unless the prompt/delivery includes identical packet items, chunks, rubric hash, verification hash, and evidence IDs for every valid voter; a poisoned finding with a quote contradicted by the packet must be rejectable from the provided context.

Implementation assessment: workflow protocol, cost M, nominal agent `claude`; touches prompt contracts, artifacts, orchestration, schemas, and end-to-end tests.

### LLM-TRIBUNAL-001 — Transport success is promoted to semantic evidence verification

Severity: High  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Inventories: LLM-014 manipulated-content/output integrity; SEC-005 trust-boundary confusion

Evidence:

- `internal/tribunal/app/verification.go:24-60` sets `verified = true` for any successfully fetched reference and then writes `EvidenceWorkerVerified` to the finding.
- `internal/tribunal/adapters/workers.go:74-149` validates URL policy, status, byte bounds, and content hash, but performs no claim-to-source support check.
- `internal/tribunal/domain/consensus.go:123-130` lets a worker-verified factual claim use the normal accepted path.
- `docs/SPEC.md:93-96` requires factual claims to have anchored or worker-verified evidence.

Observed: HTTP availability and provenance are conflated with semantic support. An unrelated allowlisted 200 response—or a real DOI that contradicts the finding—upgrades the model output to `worker-verified`.

Expected boundary: “retrieved” and “supports the claim” must be distinct states. Only bounded, provenance-recorded verification that evaluates the cited claim against the fetched source may set `worker-verified`; contradictory or indeterminate evidence must remain unverified and be visible to voters.

Failure mechanism: lower-trust model output selects the citation; the worker confirms only transport, then the host raises its authority. This is evidence laundering across a trust boundary.

Impact: fabricated or irrelevant citations can bypass the factual-claim evidence gate and influence accepted recommendations. The result is authoritative-looking but unsupported output.

Recommended repair: persist retrieval status separately, build a typed verification task binding finding ID, normalized claim, evidence ID, excerpt/hash, and verdict (`supports`, `contradicts`, `indeterminate`, `unavailable`), and require deterministic/independent validation before promotion. Include the complete result in every vote packet.

Validation: fixtures for unrelated 200 content, contradictory content, a correct source, search result pages, DOI landing pages, and fetched-content prompt injection; only the supporting fixture may become worker-verified.

Implementation assessment: external-service semantics, cost M, nominal agent `multi-agent`; the change spans worker contracts, evidence state, voting, and adversarial fixtures.

### FSR-TRIBUNAL-001 — Resume reruns and overwrites instead of continuing a checkpoint

Severity: High  
Confidence: Confirmed for the source property; no crash manifestation was executed  
Evidence basis: source-evidenced  
Scenario: SC-INT/SC-COR  
Observed class: unsafe-continue  
Required safe states: `fail_resumable`, `fail_idempotent`

Evidence:

- `docs/SPEC.md:110-114` requires checkpoint validation and continuation from the first incomplete idempotent step.
- `internal/tribunal/app/operations.go:141-160` reads packet/meta and calls `s.Review` again with the same run ID and directory.
- `internal/tribunal/app/review.go:64-83` persists start artifacts and begins pass 1 again.
- `internal/tribunal/app/review.go:321-337` replaces packet/meta/snapshot artifacts; invocation artifacts use stable `calls/<reviewer>/review` paths.
- `internal/tribunal/app/review_test.go:127-133` removes only `final.json` and verifies that the rerun finishes; it does not assert checkpoint continuation, provider-call counts, or artifact preservation.

Observed: an incomplete run is not interpreted as a state machine checkpoint. Resume re-enters the complete review use case, repeats paid/non-deterministic provider calls, and replaces same-path evidence.

Expected boundary: after validating journal, snapshot, packet hash, artifacts, and delivery records, resume should continue exactly the first incomplete step; completed calls must remain immutable and must not be invoked again.

Failure mechanism: recovery is implemented as a recursive full workflow invocation rather than phase-specific idempotent handlers.

Impact: duplicate cost, changed model outcomes under one run identity, loss of forensic evidence, and a final artifact that can no longer be reconstructed from the original persisted pass.

Recommended repair: introduce a checkpoint reducer over journal/state plus artifact validators, immutable attempt IDs, and idempotent phase handlers. Quarantine mismatched/corrupt checkpoints instead of overwriting them.

Validation: kill/restart at each phase; assert unchanged hashes/mtimes for completed artifacts, zero duplicate adapter invocations, same run/packet identity, and exact continuation from the first absent artifact.

Resilience/FMECA: recover → reconstitute/understand; local effect is artifact replacement, workflow effect is repeated deliberation, operator effect is a misleading “resumed” run. Detection is delayed and mostly forensic. Criticality is plausible/inferred; the paid workflow is the blast radius.

Implementation assessment: persistence recovery, cost L, nominal agent `multi-agent`; requires a phase protocol and systematic fault tests.

### SEC-TRIBUNAL-001 — Persisted packet identity is not cryptographically revalidated

Severity: High  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Inventories: SEC-005 trust-boundary confusion; SEC-008 unsafe path/file access

Evidence:

- `internal/tribunal/app/review.go:751-759` reads `packet.json` and checks only top-level schema version and a non-empty hash.
- `internal/tribunal/app/operations.go:152-176` compares `meta.PacketHash` to the packet's stored hash field for resume/replay.
- `internal/tribunal/app/edit.go:75-90` consumes the same unverified persisted packet before proposal and edit-scope validation.
- `docs/SPEC.md:23-34` declares packet content and paths untrusted and binds packet identity to canonical SHA-256; `docs/SPEC.md:112-113` requires hash checkpoint validation.

Observed: changing persisted packet content while retaining its old `packet_hash` passes the reader and meta comparison. Item hashes, rubric hash, evidence hash, canonical manifest, source paths, and workspace identity are not recomputed at this trust re-entry.

Expected boundary: every persisted packet read must strictly validate versions and semantic invariants, recompute item/rubric/evidence/canonical packet hashes, and revalidate canonical paths before the packet can drive resume, replay, explanation, or edit.

Failure mechanism: stored identity fields are treated as proof of the content they describe.

Impact: local corruption or same-user tampering can produce reviews, replays, or edit proposals under false provenance. The document hash checks reduce direct arbitrary edit risk, but the deliberation and audit identity remain compromised.

Recommended repair: centralize `ValidatePersistedPacket`, recompute the canonical hash from decoded content, require equality across packet/meta/manifest/state, validate every nested schema, and quarantine mismatch with a reason code.

Validation: mutate each packet field while preserving the old top-level hash; resume, replay, and edit must refuse without any provider call or source mutation.

Implementation assessment: local guardrail evolving into persistence recovery, cost M, nominal agent `codex`; central validator plus corruption fixtures.

### FSR-TRIBUNAL-002 — Edit/revert mutations are not transactionally coupled to lifecycle records

Severity: High  
Confidence: Likely (missing guards are source-evidenced; crash manifestation not reproduced)  
Evidence basis: source-evidenced  
Scenario: SC-INT  
Observed class: unsafe-continue; crash window may become data-loss  
Required safe states: `fail_rollback`, `fail_manual_hold`

Evidence:

- `internal/tribunal/app/edit.go:98-114` applies edits before lifecycle transition and publication; either later error returns exit 6 without rolling back the applied document.
- `internal/tribunal/app/edit.go:388-423` writes backups, then source files, then the edit record; a process death between those steps has no recovery marker.
- `internal/tribunal/app/edit.go:170-195` restores originals before persisting `RevertedAt` and the new final state.
- `docs/SPEC.md:127-133` requires durable atomic replacement with the original preserved.

Observed: per-file writes are atomic and in-process write errors attempt rollback, but workflow-level commit is not journaled. A deterministic transition/publication failure leaves edited content with an aborted command. Crash points can leave partial multi-file changes or an absent/stale edit record. Revert has the symmetric state-after-mutation window.

Expected boundary: prepare and fsync a transaction record before mutation; apply all files with explicit progress; commit lifecycle/final projections; on recovery either finish the exact transaction or roll it back. Ambiguous state must stop on manual hold.

Failure mechanism: atomic file replacement is mistaken for an atomic multi-artifact workflow transaction.

Impact: user documents may differ from Tribunal's durable status, revert may be unavailable or repeatable incorrectly, and operators cannot safely infer whether retrying will duplicate or overwrite work.

Recommended repair: journal an edit transaction with before/after hashes and phase (`prepared`, per-file applied, `committed`, `reverted`); make recovery reconcile every file before allowing edit/revert/resume; roll back if transition or publication fails before commit.

Validation: fault-inject after each backup, each source replacement, edit-record write, lifecycle transition, final write, and revert write; assert all-old or all-new state with an accurate recoverable record.

Implementation assessment: persistence recovery, cost L, nominal agent `multi-agent`; multi-file crash consistency and lifecycle coupling require systematic fault coverage.

### LLM-TRIBUNAL-002 — Token budget is an estimate, not a runtime circuit breaker

Severity: Medium  
Confidence: Confirmed for missing enforcement; overspend manifestation not observed  
Evidence basis: source-evidenced  
Inventory: LLM-012 unbounded consumption

Evidence:

- `docs/SPEC.md:42-43` says all token use is capped and `docs/SPEC.md:116-120` sets a 500k default.
- `internal/tribunal/app/review.go:49-50,285-297` performs one byte-based preflight estimate.
- `internal/tribunal/adapters/adapter.go:34-40` defines token/cost response fields, and `internal/tribunal/adapters/openai.go:94-109` populates token usage.
- The review result/orchestration path does not accumulate those fields or stop subsequent calls when the configured budget is reached.

Observed: the estimate excludes prompt framing, rubric/persona overhead, vote prompts, verification, output tokens, and retries. Actual provider usage is discarded.

Expected boundary: enforce per-call and per-run input/output token and optional cost ceilings before every invocation, account returned usage, reserve the next call's maximum, and persist a budget-exhausted terminal reason.

Failure mechanism: a preflight heuristic is labeled as a runtime cap, while reachable retry and vote paths are not circuit-broken.

Impact: crafted large documents or repeated contract failures can exceed the operator's declared cost ceiling while the run continues normally.

Recommended repair: add a durable run budget ledger and reservation/reconciliation API at the central provider invocation boundary; fail visible/manual hold before the next call if usage metadata is absent or the reservation would exceed the ceiling.

Validation: adapters returning explicit usage, missing usage, retry usage, and near-limit values; assert no call starts once reserved plus consumed tokens would exceed the limit.

Implementation assessment: workflow protocol, cost M, nominal agent `codex`.

### ARC-TRIBUNAL-003 — Unevidenced severity cap is applied only to factual claims

Severity: Medium  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Inventory: AID-009 declared-design contradiction

Evidence:

- `docs/SPEC.md:79-84` states: “Unevidenced findings are capped at minor unless evidence attaches.”
- `internal/tribunal/domain/consensus.go:123-130` applies the cap only when category is `factual-claim` and evidence status is `unevidenced`.

Observed: an unevidenced correctness, evidence, integrity, scope, structure, style, security, or data-loss finding may retain a model-voted major/blocker severity and produce exit code 1.

Expected boundary: the global evidence cap must apply to every unevidenced finding; stricter category rules should overlay that baseline, not replace it.

Failure mechanism: the implementation merged the special factual-claim outcome with the general severity rule.

Impact: unsupported findings can be presented as blocking/major recommendations and alter command exit behavior.

Recommended repair: normalize evidence severity before category-specific outcome logic; table-test every category × evidence status × severity combination.

Validation: property/table tests showing every unevidenced category caps at minor, while anchored/worker-verified findings retain the voted median subject to strict-category rules.

Implementation assessment: local guardrail, cost S, nominal agent `codex`.

### ARC-TRIBUNAL-004 — Persisted readers accept unknown nested schema versions

Severity: Medium  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Inventories: AID-009, AID-011

Evidence:

- `docs/SPEC.md:21-22` requires missing and unknown versions to fail closed.
- `internal/tribunal/storage/store.go:136-145` uses permissive `json.Unmarshal` with no unknown-field or version dispatch.
- `internal/tribunal/app/operations.go:403-411` validates only the outer final version/identity.
- `internal/tribunal/storage/state.go:74-86` validates only the outer ledger version.
- `internal/tribunal/domain/types.go:86-103,153-204,260-279` persists versioned findings/votes/clusters/decisions/disputes inside those outer artifacts.

Observed: a `final.json` or `findings.json` with a valid outer version but missing/unknown nested finding, evidence, cluster, vote, or decision versions is accepted by read paths. Some persisted nested object types have no version field at all.

Expected boundary: each artifact reader must dispatch an explicit schema for the complete object graph and reject unknown fields, missing versions, unknown versions, and semantic identity mismatches.

Failure mechanism: write-time typed structs and provider JSON Schema are assumed to make durable read-time data trustworthy.

Impact: corrupt or future-format state can silently acquire current semantics, weakening replay, explain, edit, and ledger integrity.

Recommended repair: implement strict per-artifact readers with schema-version dispatch and nested semantic validation; use the JSON Schema dependency for persisted artifacts or equivalent strict typed validation.

Validation: golden reader tests mutate every nested `schema_version` to zero/unknown and add unknown fields; all must fail closed with artifact and path context.

Implementation assessment: workflow protocol, cost M, nominal agent `gpt`.

### FSR-TRIBUNAL-003 — Ledger publication precedes terminal publication

Severity: Medium  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Scenario: SC-INT  
Observed class: degraded-lying / unsafe-continue  
Required safe state: `fail_resumable`

Evidence:

- `internal/tribunal/app/review.go:650-669` calls `UpdateLedger` before reports, terminal state/final, latest, and active projections.
- Any later error is returned, but the ledger update is not reverted or marked provisional.
- `docs/SPEC.md:110-114` says terminal success requires durable terminal state and `final.json`.

Observed: a report, terminal, latest, or active write failure can leave workspace findings updated from a run that has no successful terminal publication.

Expected boundary: authoritative workspace projections should advance only from a durably committed terminal record, or carry a provisional transaction marker that recovery can finish/revert idempotently.

Failure mechanism: publication is a sequence of independently atomic files without a transaction/commit point; the most authoritative cross-run projection is written first.

Impact: `findings list` can expose decisions from a failed publication while status/resume sees an incomplete run. Retry may rewrite projections without an auditable commit boundary.

Recommended repair: persist and fsync terminal run artifacts first, then update workspace projections from that immutable source under a publication transaction marker; make projection repair idempotent.

Validation: fault-inject each publication write and assert either no workspace projection changes or an explicit recoverable pending-publication state whose replay is duplicate-free.

Implementation assessment: persistence recovery, cost M, nominal agent `codex`.

### SEC-TRIBUNAL-002 — Release archive smoke occurs after publication

Severity: Medium  
Confidence: Confirmed  
Evidence basis: source-evidenced  
Inventory: SEC-009 unsafe deployment default

Evidence:

- `.github/workflows/release.yml:33-67` publishes the release after only the repository verification job.
- `.github/workflows/release.yml:81-116` declares `archive-smoke` with `needs: publish` and downloads the already-published archive.

Observed: a broken archive, wrong packaging layout, or failing `verify-install` is already public by the time the archive smoke can reject it.

Expected boundary: build archives without publishing, smoke the exact bytes on the release target matrix, then publish and attest only those validated immutable artifacts.

Failure mechanism: artifact production and release publication are combined in one job, forcing acceptance tests to run after the irreversible external side effect.

Impact: users can download a known-bad release before CI turns red; cleanup is manual and attestations may refer to artifacts that failed installation verification.

Recommended repair: split GoReleaser into snapshot/build and publish phases, upload pre-release artifacts between jobs, smoke by checksum on all targets, and publish only if the full matrix passes.

Validation: a deliberately malformed archive must fail the workflow before any GitHub Release asset or attestation exists.

Implementation assessment: workflow protocol, cost M, nominal agent `codex`.

### ARC-TRIBUNAL-001 — No machine-evaluable architecture intent or invariant registry

Severity: Medium  
Confidence: Likely (bootstrap/inferred-intent ceiling)  
Evidence basis: source-evidenced  
Inventories: AID-012, AID-014

Evidence:

- A repository search for `.architecture/` produced no path.
- `docs/ARCHITECTURE.md:3-64`, `docs/SPEC.md:15-43`, and the ADRs declare important boundaries, but none is registered with owners, scopes, assertions, exceptions, or baseline evidence.

Observed: architecture conformance depends on prose review. This audit found direct contradictions that no deterministic repository gate catches.

Expected boundary: a versioned intent map and invariant registry should identify owners, nodes, dependency rules, schema/hash rules, vote-packet requirements, lifecycle rules, exception expiry, and evidence-producing checks.

Failure mechanism: architectural intent exists, but its enforcement metadata does not. Documentation and implementation can diverge without a stable identity or trend baseline.

Impact: recurrent drift, ambiguous ownership, no new-vs-known CI gate, and no defensible architecture health/trend score.

Recommended repair: use `plan-architecture-invariants` to create the registry and baseline; seed it from accepted SPEC/ADR constraints and the regression tests recommended here. This audit does not define or write that target architecture.

Validation: registry validation passes; one deterministic invariant is hand-recomputed; CI rejects a fixture that violates a packet/vote/lifecycle dependency rule; exceptions require owner and expiry.

Implementation assessment: governance decision, cost M, nominal agent `human-owner` with implementation support from `gpt`.

## LLM trust-boundary map

| Ingress | Trust | Context form | Deterministic host control | Reachable sink/state | Verdict |
|---|---|---|---|---|---|
| System/reviewer/voter prompt templates | trusted source | authoritative prompt | source-controlled constants; output schema | provider behavior | Checked; prompt is not counted as a security boundary |
| Reviewed Markdown/plaintext/DOCX/PDF | untrusted | explicit delimiters and untrusted notice (`prompts.go:13,37-48`) | output schema, finding semantic validation, anchor resolution | findings, then votes/report/edit proposal | LLM-001 Not Confirmed for tool/action injection; no direct tool sink shown |
| Persona lens | untrusted | labeled as persona lens | structured/freeform fencing and output schema | reviewer findings | Checked; no host capability sink shown |
| Pre-review evidence | untrusted | delimited/labeled evidence | allowlisted fetch and packet hash | reviewer findings | Checked; no action sink, but output integrity still depends on voting |
| Post-review evidence | untrusted external content | not delivered to voters | fetch policy only | evidence status/final report | Finding LLM-TRIBUNAL-001 and ARC-TRIBUNAL-002 |
| Model review output | untrusted | JSON contract | real JSON Schema + semantic checks + anchor quarantine | clusters/vote packet | Strong boundary; consensus defects noted separately |
| Model vote output | untrusted | JSON contract | schema, voter identity, blind-ID map | decisions/final | Strong parsing; missing adjudication context is a protocol defect |
| Model edit output | untrusted | JSON contract | accepted-finding binding, byte scopes, hashes, host-only writes | source documents | Host enforcement is strong; workflow durability finding remains |
| Provider/tool metadata | local config/CLI version | provider CLI surface | fixed adapter registry/argv | external provider process | No MCP/dynamic tool catalog; runtime provider drift is a validation gap |

### Agency, side-channel, poisoning, and supply-chain results

The model has no Tribunal-registered send/pay/delete/admin tool. Claude is invoked with an empty allowed-tool set; Agy is invoked in sandbox/plan mode; Codex is invoked read-only in the run directory (`internal/tribunal/adapters/subprocess.go:103-136`). Host edits require a separate command and deterministic scope validation. Therefore LLM-004/005 excessive agency and confused-deputy escalation are **Not Confirmed** on the traced Tribunal host path. Codex's exact read sandbox outside the run directory was not runtime-verified, so this is not a broad confidentiality clearance.

| Role/capability | Provenance | Reach and risk | Argument/destination validation | Independent approval | Credential/identity | Confused-deputy result |
|---|---|---|---|---|---|---|
| reviewer/voter provider subprocess | fixed host registry and adapter argv | provider CLI process; intended read-only, no Tribunal write/send tool | typed request; bounded output; JSON Schema and semantic checks | N/A—no consequential host action | current local user plus explicitly allowlisted provider secret | Not Confirmed; exact provider sandbox requires runtime conformance |
| evidence worker fetch | host-built typed URL task | outbound HTTPS to exact allowlisted public domain | URL/provider resolver, destination allowlist, DNS/IP/redirect/byte/time caps | trusted config establishes allowlist | local process; optional named auth env | SSRF/deputy path Not Confirmed; semantic authority defect is LLM-TRIBUNAL-001 |
| editor proposal provider | fixed host registry; Claude disabled for editing | produces JSON only; host owns file writes | accepted finding IDs, canonical path/hash, byte scope, stale-content checks | separate user `edit --apply`; document scope confirmation | current local user/provider credential | Model cannot directly write; transaction durability is FSR-TRIBUNAL-002 |

Agent influence-cycle overlay:

| Stage | Influence | Enforced control | Next state |
|---|---|---|---|
| input ingestion | user-selected document, untrusted document bytes | canonical path/type/UTF-8/secret handling | frozen packet |
| context assembly | rubric, persona, packet, evidence | explicit trust labels/delimiters; packet hash | provider decision |
| capability selection | fixed adapter role; no dynamic MCP/tool list | host registry and role-specific argv | provider invocation |
| pre-execution | model JSON | schema plus semantic/anchor/scope validation | finding, vote, or edit proposal |
| execution/result | provider process or bounded evidence HTTP | timeout/output/process/network controls | durable raw/parsed artifacts |
| memory/write-back | human arbitration decision memory | typed fingerprint record; no open-ended semantic memory | later default recommendation |
| downstream propagation | report, ledger, optional host edit | consensus and explicit edit command | user-visible state/document |

Workload identity is the local OS principal. Provider credentials are taken only from configured named environment variables and passed through the restricted environment (`subprocess.go:142-156`); there is no tenant/delegated identity model. That is appropriate for the declared local single-user CLI but must not be generalized to a service deployment without a new authorization review.

No Markdown renderer, link unfurl, or auto-fetch of model-emitted URLs was found. Evidence fetches accept only typed evidence strings, exact allowlisted domains, public DNS/IPs, HTTPS, bounded redirects, bytes, and time (`internal/tribunal/adapters/workers.go:74-201`). LLM-006 side-channel exfiltration is **Not Confirmed** for the inspected sinks.

There is no RAG/vector index, training/fine-tune ingestion, semantic memory write-back, or cross-tenant service in the repository. LLM-008 through LLM-011 are **N/A — role absent**. Decision memory stores human arbitration records keyed by finding fingerprint; it is not injected as open-ended model context on the traced review path.

Consumption bounds exist for wall time, output bytes, findings, verification tasks, arbitration items, redirects, and extraction. Runtime token/cost accounting is missing (LLM-TRIBUNAL-002). Provider/model identities are configured but no runtime approved-baseline/digest mechanism exists; because remote provider semantics and CLI update provenance were not observed, LLM-013 is a **Plausible governance gap**, not a separate finding.

LLM inventory dispositions: LLM-001 non-finding for action injection; LLM-002 non-finding for protected host capability; LLM-003 non-finding (schema + semantic validation before classic sinks); LLM-004/005 non-findings; LLM-006/007 non-findings within inspected local single-user scope; LLM-008/009/010/011 N/A; LLM-012 finding; LLM-013 Plausible/not escalated; LLM-014 finding.

## Classic security boundary map and inventory

| Surface | Authority/data | Enforcement | Result |
|---|---|---|---|
| Local CLI and external state | current OS user | state/workspace path separation, 0700 dirs, 0600 files, canonical revalidation, kernel locks | Strong; packet revalidation defect remains |
| Provider subprocess | local provider credential | explicit env allowlist, secret redaction, bounded output/time, process-group kill | SEC-012/015 Not Confirmed on inspected path |
| OpenAI-compatible HTTP | configured endpoint/API key | same-origin redirects, bounded response, explicit secret env | No SSRF path from reviewed document found |
| Evidence retrieval | model-provided reference | typed resolver, exact domain allowlist, HTTPS, public IP dial, redirect/byte/time caps | SSRF Not Confirmed; semantic trust finding remains |
| Document edit/revert | accepted finding + user command | canonical path/hash, regular-file, UTF-8 byte/scope checks, atomic write/backups | Path injection Not Confirmed; transaction finding remains |
| Release | GitHub Actions tag principal | pinned actions, checksums/SBOM/attestation | Unsafe order finding SEC-TRIBUNAL-002 |

SEC-001/002/003/010/013/014 are N/A for this local, single-user CLI (no remote routes, tenants, or UI security boundary). SEC-004 is routed to the LLM agency review and not confirmed. SEC-005 has findings LLM-TRIBUNAL-001 and SEC-TRIBUNAL-001. SEC-006 injection is not confirmed: subprocess arguments are constructed as argv, PDF uses `--`, model output is schema/semantically validated, and edit paths are host-selected. SEC-007 secret exposure is not confirmed in the inspected path; known values are redacted and the workspace `.env` is not loaded. SEC-008 has SEC-TRIBUNAL-001; direct traversal/symlink edit bypass was not confirmed. SEC-009 has SEC-TRIBUNAL-002. SEC-011/012/015 are not confirmed given explicit environments, bounded provider roles, and process supervision; the Codex sandbox runtime gap is noted above.

## Failsafe inventory and scenario records

| Workflow | Trigger/family | Target safe state | Static classification | Key evidence |
|---|---|---|---|---|
| Review preflight | bad config, missing provider, oversized context; SC-CFG/DEP/USR | fail_closed | safe-stop/degraded-honest | strict config, context preflight, provider quorum status |
| Provider invocation | timeout, malformed JSON, unavailable panelist; SC-NET/STL/DEG | fail_visible or fail_degraded | safe-stop/degraded-honest | call timeout, bounded output, one contract retry, quorum publication |
| Evidence worker | DNS/redirect/private target/oversize; SC-NET/USR | fail_visible/degraded | safe-stop | exact allowlist, public dial, bounds; semantic verification is unsafe-continue |
| Run recovery | missing/corrupt terminal artifact; SC-INT/COR | fail_resumable/idempotent | unsafe-continue | FSR-TRIBUNAL-001 |
| Durable publication | failure after ledger update; SC-INT | fail_resumable | unsafe-continue/degraded-lying | FSR-TRIBUNAL-003 |
| Edit/revert | interruption after document mutation; SC-INT | fail_rollback/manual_hold | unsafe-continue; possible data-loss | FSR-TRIBUNAL-002 |
| PDF extraction | missing/hung/malformed `pdftotext`; SC-DEP/STL/USR | fail_closed/visible | safe-stop | lookpath, timeout, UTF-8 and output cap |
| Release | archive smoke fails; SC-INT/DEP | fail_closed before external publication | unsafe-continue | SEC-TRIBUNAL-002 |

FSR inventory dispositions: FSR-001 non-finding (dependency detection); FSR-002/003/004 non-findings (strict config/user preflight/startup refusal); FSR-005 finding FSR-TRIBUNAL-002; FSR-006 non-finding (process-group cleanup); FSR-007 finding LLM-TRIBUNAL-002, otherwise bounded waits; FSR-008 conditional non-finding (bounded failures, no general backoff); FSR-009 findings FSR-TRIBUNAL-001/003; FSR-010 non-finding for provider contracts and document formats; FSR-011 conditional—bounded single contract retry, no retry storm path found; FSR-012 finding FSR-TRIBUNAL-003, quorum degradation itself is honest; FSR-013 conditional—CLI errors/reason codes exist, no alerting expected for a one-shot CLI; FSR-014 findings FSR-TRIBUNAL-001/002; FSR-015 finding FSR-TRIBUNAL-002, with revert controls otherwise strong; FSR-016 non-finding for malformed/provider failures, with token accounting exception.

### Readiness scorecard

All scores are static and therefore capped at 2. Grades use the worst scenario, not an average.

| Subsystem | Families traced | Worst class | Det | Con | Rec | Sig | Grade | Driver |
|---|---|---:|---:|---:|---:|---:|---|---|
| Review/provider pipeline | CFG DEP USR NET STL DEG INT | unsafe-continue | 2 | 1 | 0 | 2 | conditional | resume repeats calls; bounded and reversible at state level, but not acceptable for paid/forensic evidence until fixed |
| Evidence workers | DEP NET USR DEG | unsafe-continue | 2 | 1 | 1 | 2 | conditional | transport-only “verification” raises content authority |
| Storage/publication | INT COR DEG | degraded-lying | 2 | 1 | 1 | 1 | conditional | ledger can advance before terminal commit |
| Edit/revert | USR INT COR | possible data-loss | 2 | 1 | 1 | 2 | not-ready | irreversible user-visible document side effect can outlive failed lifecycle commit |
| Release | DEP INT DEG | unsafe-continue | 2 | 0 | 1 | 2 | not-ready | external publication precedes archive acceptance |

## Architecture inventory and category evaluation

AID dispositions: AID-001 finding ARC-TRIBUNAL-002; AID-002 non-finding (one review implementation); AID-003 non-finding for sampled declared modules; AID-004 non-finding within sampled source; AID-005/006 N/A—no service/API topology; AID-007 non-finding (adapter registry has multiple real implementations); AID-008 non-finding in sampled paths; AID-009 findings ARC-TRIBUNAL-002/003/004; AID-010 finding ARC-TRIBUNAL-002 also contradicts the review-sequence text; AID-011 findings include missing vote-packet, checkpoint, nested-schema, and crash-transaction regressions; AID-012 finding ARC-TRIBUNAL-001; AID-013 N/A—no baseline; AID-014 not evaluable—no registry, recorded as ARC-TRIBUNAL-001.

| Category | Score | Mapped checks | Violation mass | Result |
|---|---:|---:|---|---|
| architecture | not normalized | 14 AID codes | 1H/3M | material declared-design contradictions; no registry denominator |
| design | not normalized | 5 workflows | 3H/3M | review/recovery protocol gaps |
| security | not normalized | 15 SEC + 14 LLM codes | 2H/2M | integrity findings; many classic remote-role items N/A |
| performance | not assessed | 1 partial | 1M | consumption bounds only |
| scalability | not assessed | 0 | — | local single-run CLI; provider serialization sampled only |
| maintainability | not normalized | 3 | 1H/1M | centralized services, but full-workflow resume duplicates behavior |
| documentation | not normalized | 4 | 1H/3M | prose is strong; implementation drift present |
| testing | not normalized | 4 | 1H/3M | happy path covered; adversarial checkpoint/vote/evidence cases missing |
| observability | not normalized | 3 | 1M | static CLI/artifact signals only; no service alerting role |
| deployment | not normalized | 1 | 1M | unsafe release acceptance order |
| developer experience | not assessed | 0 | — | outside requested audit depth |
| consistency | not normalized | 3 | 1H/2M | schema/lifecycle consistency findings |
| naming | not normalized | 1 sample | 0 | no material finding in sampled source |
| ownership | not normalized | 1 | 1M | no machine registry/owners |
| technical debt | not normalized | 11 findings | 5H/6M | no separate denominator without registry scope |
| dependency health | not assessed | 2 partial | 0 | dependency absence paths sampled; vulnerability freshness not queried |
| agent coordination | not normalized | 4 | 2H | pass-1 barrier present; vote/evidence handoff defective |
| memory usage | not normalized | 1 partial | residual | raw source read precedes extraction caps; runtime effect untested |
| state management | not normalized | 5 | 3H/2M | recovery, packet, publication, and edit findings |
| error handling | not normalized | 4 | 2H/1M | explicit errors generally; transactional gaps remain |
| configuration | not normalized | 3 | 0 | strict trusted config is a non-finding |
| extensibility | not normalized | 2 samples | 0 | multiple real adapter implementations; no drift finding |

### Architectural opportunities

- Introduce the documented application snapshot port between TUI and storage. `docs/ARCHITECTURE.md:55` says TUI depends on an app snapshot port, while `internal/tui/tui.go:8-11` imports `storage.Snapshot` directly. This is a low-risk seam cleanup, not a material finding in this audit.
- Move packet-dependent spell/reference worker inputs behind a typed worker-task port. `internal/tribunal/adapters/workers.go:20,204` imports the concrete documents packet even though the module contract describes adapters in terms of domain/app ports. This would make adapter tests and future evidence workers less coupled.

### Waivers, trend, and architecture escalation

No exception registry exists, so there are no current or expired waivers to evaluate. AID-013 coupling trend is N/A because no baseline exists. Health, violation-mass trend, and new-vs-known classification are intentionally absent; the emitted metrics line records null values rather than a fabricated baseline.

| Concern | Owning follow-on skill | Handoff |
|---|---|---|
| intent/invariant definitions and baseline | `plan-architecture-invariants` | consume ARC findings and drift sidecar; maintainers decide rules/owners |
| interruption-point and rerun semantics | `audit-recovery-idempotency` | deepen FSR-TRIBUNAL-001/002 before repair |
| packet/artifact identity flow | `audit-dataflow-input-output` | validate source→packet→manifest→resume/edit hash binding |
| bounded fixes after acceptance | appropriate `repair-*` workflow | use ranked findings JSON; no repair was authorized here |

## Break-it review and non-findings

Static attack traces that survived:

- Selected symlink/special-file/traversal replacement is rejected during packet build and edit revalidation.
- Evidence-worker SSRF attempts are constrained by exact domains, HTTPS, public DNS/IP checks, redirect revalidation, and a pinned public dial path.
- Model JSON containing unknown finding IDs, duplicate votes, invalid categories/severities, or out-of-scope edit hunks is rejected by schema and semantic validation.
- Provider hangs and output floods are bounded; Unix child processes are terminated as a group.
- Review does not write beside source documents; state remains under the external root.
- Revert refuses when live content no longer matches Tribunal's edited hash.
- Release actions are commit-pinned and archives are checksummed/SBOM-attested, although smoke ordering is wrong.

Static attacks that failed and produced findings:

- Give a voter a plausible anonymous finding: it cannot consult the document/evidence because those are absent.
- Cite any successful allowlisted response: the finding is marked worker-verified without a support verdict.
- Remove an incomplete run's final artifact: resume enters the complete review again.
- Mutate persisted packet content but retain its stored hash: packet/meta equality still passes.
- Fail lifecycle/publication after edit: the command reports aborted while document changes remain.
- Fail publication after ledger update: cross-run findings can become authoritative without terminal commit.
- Fail archive install smoke: release assets already exist.

## Planner handoff and prioritized remediation proposal

1. Restore decision integrity: build complete frozen vote packets and implement claim-to-evidence verdicts together; they share schemas and adversarial fixtures.
2. Restore recovery integrity: strict packet/artifact validators, phase-specific resume, immutable attempts, and journal-driven publication recovery.
3. Make edit/revert a recoverable transaction before expanding edit functionality.
4. Add runtime token/cost reservations at the central invocation boundary.
5. Correct the global evidence cap and strict persisted-schema readers.
6. Move archive smoke ahead of publication.
7. Create the planner-owned intent/invariant registry and convert each fixed defect into a deterministic invariant/regression gate.

## Regression and guardrail test set

- Identical vote packet and verification hash for every voter; no identity leakage.
- Irrelevant/contradictory/failed evidence never promotes a finding; fetched injection remains inert data.
- Kill/restart at every lifecycle transition with adapter invocation counters and immutable artifact hashes.
- Persisted packet/meta/manifest/state mutation matrix; every mismatch fails before side effects.
- Nested unknown/missing schema version matrix for every durable artifact reader.
- Token/cost reservation tests across retries, votes, missing usage, and provider fallback.
- Consensus property tests across every category/evidence/severity combination.
- Edit/revert fault injection after every durable write; all-old/all-new or explicit manual hold.
- Publication fault matrix proving ledger/latest/active projections are derived only from committed terminal runs.
- Release fixture proving failed archive smoke creates no external release.
- Provider sandbox test with synthetic secrets/files verifying each CLI cannot read beyond the intended packet/run directory or invoke tools.

## Residual risk register

| Risk | Current control | Evidence gap | Owner decision |
|---|---|---|---|
| Provider CLI read sandbox differs by installed version | fixed argv, read-only/sandbox modes, restricted env | no authorized runtime sandbox drill | define supported provider versions and a conformance smoke |
| Remote model/endpoint semantics drift | configured model identity and adapter contracts | no approved runtime baseline/digest | decide provenance/upgrade policy |
| Large source files are read before extraction limits (`packet.go:153-164`) | extracted PDF/DOCX/output/context caps | no input-file byte preflight or memory stress evidence | define maximum source bytes per format |
| No general network retry/backoff | bounded calls and honest failure | rate-limit recovery behavior not drilled | decide whether fail-visible is preferred to retry for each worker/provider |
| Architecture trend unavailable | prose SPEC/ADRs | no registry or baseline | establish planner-owned baseline after fixes |

## Validation limits

- Static source review only; no tests or binaries were run under the read-only audit authority.
- No provider credentials, live model calls, network endpoints, signals, crashes, or filesystem fault injector were used.
- No dependency vulnerability database query or CVE claim was made.
- Codex/Claude/Agy sandbox behavior depends on installed versions and was not runtime-observed.
- Absence-based architecture conclusions are capped where dynamic/external consumers could exist.
- The report was generated against revision `4a8003c790857fbc3d7c5d7adacebda6fe030f32`; later changes are not covered.

Machine-readable findings are in `tribunal-findings-2026-07-21.json`; architecture drift identities are in `tribunal-findings-drift-2026-07-21.json`.

Final confidence: **High** for the reported static implementation properties and their declared-contract contradictions; **Medium** for operational severity where crash, provider, filesystem, or release runtime behavior was not reproduced. No finding is presented as runtime-observed.
