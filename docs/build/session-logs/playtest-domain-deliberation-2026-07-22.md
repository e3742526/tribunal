# Domain deliberation playtest — 2026-07-22

Skill: `audit-playtest-app` (CLI-adapted). Target: how Tribunal **solves,
argues, and agrees** when reviewing substantive issue documents across
philosophy, ethics, science, and coding.

Reference: prior pass `playtest-consensus-2026-07-23.md` (51 consensus cards,
D-070 weighted-lean fix). This pass adds domain content, not vote-arithmetic
coverage.

## Scope and method

### Scenario library

- Existing cards in `docs/test_scenarios/{01,02}-*.md` left unchanged.
- **Added** `docs/test_scenarios/03-domain-deliberation.md` (100 domain cards
  + clustering + 5 disagree→consensus extras).
- Updated `docs/test_scenarios/README.md` with Domain pass shape.

### Corpus (live documents)

100 short flawed Markdown briefs under `/tmp/tribunal-playtest/corpus/`:

| Domain | IDs | Planted defect style |
|---|---|---|
| Philosophy | P01–P25 | Overstrong metaphysics, is–ought leaps, modal bridges |
| Ethics | E01–E25 | Absolute duties, dismissed counterarguments, missing safeguards |
| Science | S01–S25 | Stats misreads, causal leaps, underpowered claims |
| Coding | C01–C25 | Injection, secrets, races, fail-open auth, weak crypto |

Documents are **arguments/proposals to review**, not questions to answer.

### Harness path (100 scenarios — primary count)

`go test ./internal/tribunal/app/ -run 'TestDomainDeliberation'`:

| Suite | Count | Path |
|---|---:|---|
| `TestDomainDeliberationPlaytest` | **100** | `domain.ResolveVotes` on domain-flavored findings × vote patterns |
| `TestDomainDeliberation_CrossDomainClustering` | 4 | Same quote+category clusters despite different issue prose |
| `TestDomainDeliberation_DisagreementToConsensus` | 5 | Full `Service.Review` → `Arbitrate` (FuncAdapter) |

**Result: all green.** 25 per domain; patterns cycled
(`unanimous_accept`, `majority_accept`, `majority_reject`, `tie`,
`unanimous_reject`, `strict_split`, `unevidenced_accept`, `severity_dissent`,
`abstain_heavy`).

### Live path (qualitative sample — real adapters)

Default panel: `claude/claude-opus-4-8`, `codex/gpt-5.6-sol`,
`agy/Gemini 3.5 Flash (Medium)`. Wall time 15–20m per run.

| Run | Domain | Status | OK reviewers | Findings | Decisions | Arb | ~sec |
|---|---|---|---:|---:|---:|---:|---:|
| smoke (trolley Q) | ethics-ish | aborted | 0 | 0 | 0 | 0 | 480 |
| C01 SQL injection | coding | arbitration_pending | 2 | 7 | 3 | 1 | ~220 |
| P01 Ship of Theseus | philosophy | findings | 2 | 7 | 4 | 0 | 177 |
| P07 Utilitarian organs | philosophy | degraded | 1 | 4 | 0 | 0 | 135 |
| E04 Informed consent | ethics | findings | 2 | 7 | 3 | 0 | 245 |
| E20 Ventilator triage | ethics | findings | 2 | 7 | 3 | 0 | 199 |
| S01 RR vs AR | science | final | 2 | 6 | 0 | 0 | 214 |
| S11 Base rate | science | findings | 2 | 6 | 3 | 0 | 138 |
| C08 Hardcoded AWS key | coding | arbitration_pending | 2 | 8 | 4 | 1 | 220 |
| C22 Unsalted SHA-1 | coding | arbitration_pending | 2 | 7 | 3 | 3 | 115 |

Live full-corpus (all 100 with real models) was **not** completed: ~2–4
minutes per successful 2-of-3 panel, plus frequent Claude schema failures and
occasional degraded/abort runs, makes 100 live reviews multi-hour and
token-heavy. The harness covers the 100-scenario request deterministically;
live sample grounds qualitative claims.

## How Tribunal solves problems

Tribunal is a **document defect finder**, not a free-form problem solver.

1. Freeze a packet + rubric.
2. Each reviewer independently emits ranked findings with quote anchors.
3. Host validates schema, resolves anchors, may quarantine bad anchors.
4. Cluster overlapping findings; blind vote; compute consensus.
5. Strict categories (security, data-loss, citation-integrity) demand full-panel
   unanimity; otherwise majority (weighted) accept/reject, or arbitration.

**Implication for “solve this issue” prompts:** if the document is a bare
question (“is diverting the trolley required?”), models split:

- Some **answer the question** (role confusion; smoke R-003 first pass).
- Some **critique the prompt structure** (smoke R-001: deontic options overlap).
- Schema enforcement drops non-conforming answers → degraded/abort.

Framed as flawed briefs (this corpus), models consistently attack **argument
quality**: unsupported leaps, missing evidence, scope overreach, operational
risk — not “who wins the trolley debate.”

## How it argues (live)

### Shared argument pattern (all domains)

Independent reviewers (when valid) repeatedly raise the same **three layers**:

1. **Core claim defect** — the planted falsehood or unsafe proposal.
2. **Evidence / methods gap** — no sources, no numbers, no tests, no protocol.
3. **Process / integrity failure** — “claim established,” dissent-as-confusion,
   empty risk section, Friday production ship.

That stack is Tribunal’s default argument shape under the generic rubric.

### By domain (observed)

| Domain | What the panel argues about | Severity lean |
|---|---|---|
| **Philosophy** | Conceptual error (identity continuity), unsupported simplicity heuristics, dogmatic “claim established” close | major/minor; fewer blockers |
| **Ethics** | Rights/autonomy violations (omit consent; age-only triage), welfare claims without definition, dismissed fairness as “minority” | blocker + major |
| **Science** | Mathematical misuse (RR≠ARR; base-rate neglect), narrative “large” effects, missing methods/preregistration | blocker/major on the math error |
| **Coding** | Concrete security defects (SQL concat, AWS key in repo, unsalted SHA-1), placeholder implementation, manual-only tests, Friday ship | **blocker/security** + major process |

### Reviewer personality (adapters)

| Adapter | Live reliability this pass | Argument style |
|---|---|---|
| **Claude** (`claude-opus-4-8`) | **0/9 valid** on sample after smoke; consistent `invalid_output` (schema_version string, flat finding objects, missing top-level fields) | First-pass raw often excellent; **retry/repair path loses the contract** |
| **Codex** (`gpt-5.6-sol`) | Usually `ok`; slowest; sometimes timeout under short wall clocks | Structured, multi-finding, security-precise; often **quarantined** on packet-item naming |
| **Gemini** (`agy` Flash Medium) | Usually `ok`; fastest | Good core defect catch; shorter writeups; anchors more often **vote-eligible** |

## How it agrees

### Agreement mechanism (working)

1. **Independent rediscovery** — different models restate the same planted
   defect in different prose (SQL injection; RR/ARR; base rate; consent).
2. **Clustering** (harness CL-*) — same category + overlapping quote anchors
   merge members even when `issue` text differs.
3. **Majority accept** — with 2 valid reviewers, non-strict findings often
   `accepted/majority_accept` with A2/R0 (no formal dissent when both accept).
4. **Disagree→consensus** (harness 5 e2e) — vote ties go to arbitration;
   operator ruling clears the dispute and records decision memory.

### Where agreement fails or is understated (live)

1. **Claude out of panel** → configured=3, valid=2. Security findings then hit
   `category_requires_full_panel_unanimity` even when both valid reviewers
   accept (C01, C08). Looks like “disagreement” but is **incomplete panel +
   strict category**, not a substantive split.
2. **Codex findings quarantined** (`packet item "….md" not found`) while
   Gemini’s parallel findings vote. Independent agreement is **split across
   quarantined vs live IDs**, so the report undercounts true consensus.
3. **S01 (RR vs AR):** both remaining reviewers raised the correct math defect,
   but **all six findings were quarantined** → `status=final`, exit 0, **zero
   decisions**. Critical science error detected then discarded by host
   validation — agreement without a recorded outcome.
4. **C22:** several decisions are `arbitration/insufficient_non_abstain_votes`
   (A1) — clustering/voting only saw one non-abstain path for some findings
   after quarantine, so even clear SHA-1 defects pend human review.
5. **Template echo:** ethics docs share boilerplate (“minority views,” empty
   risk section). Reviewers correctly flag it, but findings look **copy-paste
   across E04/E20** — agreement on template seams as much as on the unique
   ethical claim.

### Harness agreement statistics (100 scenarios)

Vote-pattern outcomes behaved as designed:

| Pattern | Outcome class | Count (approx across 4 domains) |
|---|---|---|
| Unanimous / majority accept | accepted (+ dissent when 2–1) | ~40 |
| Majority / unanimous reject | rejected | ~16 |
| Tie / strict_split / abstain_heavy | arbitration | ~28 |
| Unevidenced factual-claim accept | unverified-claim | ~8 |
| Severity dissent | accepted with dissent | ~8 |

No product bugs in ResolveVotes for these patterns (post–D-070).

## Product findings (live-confirmed)

### Issue L-01 — Claude schema invalidation is systemic

**Severity:** High  
**Confirmation:** Confirmed (9/9 sampled live runs after smoke; smoke also failed Claude)  

**Where:** Reviewer R-001 `invalid_output` on every stratified live run.

**Expected:** Claude returns schema-valid review JSON after retry.  
**Actual:** Retry/repair yields `schema_version: "1.0"`, missing required top-level
fields, or flat finding objects rejected by `additionalProperties: false`.

**User impact:** Default 3-family panel **always** loses one family; security
findings inflate to arbitration; degraded runs more common.

**Suggested fix:** Adapter-side coercion (integer schema_version; wrap bare
findings); tighter retry prompt; or fail-soft normalization for known Claude
shapes before schema reject.

### Issue L-02 — Packet-item quarantine drops valid Codex findings

**Severity:** High  
**Confirmation:** Confirmed (C01/C08/C22/P01/S01 reports show `Quarantined:
packet item "….md" not found` on Codex rows)

**Expected:** Anchors using the packet artifact id resolve.  
**Actual:** Codex findings often quarantine; Gemini counterparts on the same
defect vote. S01 lost **all** findings this way.

**User impact:** Silent loss of multi-model agreement; false “final/clean”
science run; security consensus delayed to arbitration.

**Suggested fix:** Normalize packet_item aliases (`C01.md` ↔
`artifact:C01.md`); reject-or-repair anchors before quarantine; surface
quarantine rate in the CLI summary.

### Issue L-03 — Incomplete panel + strict category masquerades as dispute

**Severity:** Medium  
**Confirmation:** Confirmed (C01, C08: A2/R0 still `arbitration` /
`category_requires_full_panel_unanimity`)

**Expected (product question):** Operators understand this is unanimity/quorum
geometry, not a split vote.  
**Actual:** Report shows arbitration with “accept majority” default while both
valid reviewers already accept — easy to misread as panel conflict.

**Suggested fix:** Reason code or dispute text that says “strict category
requires full configured panel; N of M valid accepted” rather than only
`category_requires_full_panel_unanimity`.

### Issue L-04 — Question-form documents invite role confusion

**Severity:** Medium (UX / rubric)  
**Confirmation:** Confirmed (smoke trolley); mitigated by corpus framing

**Expected:** Reviewers critique the document.  
**Actual:** Bare Q&A invites some models to answer the moral question inside
finding fields or free text.

**Suggested fix:** Doc templates / recommend mode guidance; integrity finding
when the packet is an instruction-to-answer rather than a claim-under-review.

### Issue L-05 — Template defects dominate short ethics briefs

**Severity:** Note  
**Confirmation:** Confirmed (E04 vs E20 near-identical secondary findings)

Shared skeleton (“minority views,” empty risk register) produces parallel
findings that can overshadow the distinctive policy error. Useful for
integrity/structure, but playtesters should vary boilerplate when measuring
claim-level discrimination.

## Notable seam tests

- Bare philosophical question vs flawed brief (smoke vs corpus).
- Strict security findings under 2-of-3 valid panel.
- All-findings-quarantined science run still exits “success/final.”
- Friday-ship + manual-test boilerplate always draws process findings.
- Harness disagree→consensus with operator accept (5/5).

## Verification

- `go test ./internal/tribunal/app/ -run 'TestDomainDeliberation' -count=1`: **PASS**
  (100 + 4 clustering + 5 arb e2e).
- Live sample: **9 completed multi-model runs** + 1 aborted smoke (table above).
- `scripts/check.sh`: run at end of session (see completion note).

## Residual / not executed

- Full live 100-document batch with real providers (time/token cost; Claude
  reliability blocker).
- `01-review-workflow.md` CLI lifecycle cards (still library-only from prior pass).
- Persona lenses (`skeptic`, `methodologist`, `governor`) not varied this pass.
- Edit/apply path not exercised (review-only deliberation).

## Recommended next pass

1. Repair L-01 (Claude schema) and L-02 (packet-item quarantine); re-run the
   8-doc stratified live sample and expect 3-of-3 panels + fewer false arbs.
2. Live smoke of all 25 **coding** docs (highest signal, strict-category stress).
3. Persona-differentiated panel on ethics (governor vs skeptic) to measure
   deliberate dissent quality.
4. Explicit “question packet” integrity fixture to lock L-04.

## Bottom line

**How Tribunal solves:** by independent, evidence-anchored document critique
under a shared rubric — not by deliberating a single joint answer.

**How it argues:** multi-layer attack (core claim → evidence gap → process
integrity), strongest and most concrete on coding/security and math-shaped
science errors; more structural/meta on philosophy.

**How it agrees:** true content agreement is common (models rediscover the same
planted defects), but **recorded** agreement is fragile when one adapter fails
schema, when anchors quarantine, or when strict categories meet an incomplete
panel. The 100-scenario harness shows the consensus machinery itself is
sound; live multi-provider reliability is the binding constraint on seeing that
agreement in production runs.
