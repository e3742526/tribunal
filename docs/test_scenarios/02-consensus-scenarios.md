# 02 — Consensus and Voting Scenarios

Cards exercising `domain.ResolveVotes` and `domain.RankArbitration` through
the real panel pipeline (`Service.Review`), not by calling the domain
function in isolation — every card below runs an end-to-end review with a
scripted 3-reviewer (or N-reviewer) panel via `adapters.FuncAdapter`, so
clustering, blind-vote delivery, and consensus math are all exercised
together as a real user would trigger them.

Each card specifies: panel size/weights, the finding(s) raised, the votes
cast, and the expected `Decision.Outcome`/`Reason`. The runnable form lives
in `internal/tribunal/app/consensus_playtest_test.go` as a table, one entry
per card ID (`S01`..`S50+`), so a future engineer changing consensus
semantics gets an immediate, named regression signal per scenario rather
than one opaque table failure.

## Vote-outcome legend

- **accepted** — majority (weighted) accept, non-strict category, evidenced.
- **rejected** — majority (weighted) reject.
- **arbitration** — tie, insufficient non-abstain votes, strict-category
  non-unanimity, or configured-unanimity not reached.
- **degraded** — quorum unmet (fewer than 2 valid reviewers, or valid
  reviewers ≤ half of configured).
- **unverified-claim** — accepted, but category is `factual-claim` and
  evidence status is `unevidenced`.

## Disagree-then-consensus track

Five or more cards below are explicitly marked **[DISAGREE→CONSENSUS]**:
the panel's first-pass reviewers disagree (split accept/reject votes,
producing an `arbitration` decision), and a follow-on `Service.Arbitrate`
call with an operator ruling (or `--accept-majority`) resolves the dispute
to a final consensus. These exercise the full disagreement-resolution loop,
not just the vote tally.

---

### S01 — Unanimous accept, non-strict category

3 reviewers, 3 accept votes, category `correctness`, evidence `anchored`.
Expected: **accepted / majority_accept**.

### S02 — Unanimous reject

3 reviewers, 3 reject votes.
Expected: **rejected / majority_reject**.

### S03 — 2-1 accept majority, non-strict category

3 reviewers: 2 accept, 1 reject, category `style`.
Expected: **accepted / majority_accept**.

### S04 — 2-1 reject majority

3 reviewers: 1 accept, 2 reject.
Expected: **rejected / majority_reject**.

### S05 — Exact tie via abstain-adjusted count

4 reviewers: 2 accept, 2 reject.
Expected: **arbitration / vote_tie**. **[DISAGREE→CONSENSUS]** — operator
rules accept via `--decisions`.

### S06 — Weighted tie (unequal counts, equal weight sums)

3 reviewers weighted 2.0/1.0/1.0: senior rejects, two juniors accept.
Weighted: accept 2.0 vs reject 2.0.
Expected: **arbitration / vote_tie**. **[DISAGREE→CONSENSUS]** — operator
rules on the merits, accepts.

### S07 — Weighted majority despite 1-2 raw split

3 reviewers weighted 2.0/1.0/1.0: senior accepts, two juniors reject.
Weighted: accept 2.0 vs reject 2.0 → still a tie (2.0 == 1.0+1.0).
Expected: **arbitration / vote_tie**.

### S08 — Weighted majority, clear winner

3 reviewers weighted 2.0/0.5/0.5: senior accepts, two juniors reject.
Weighted: accept 2.0 vs reject 1.0.
Expected: **accepted / majority_accept**.

### S09 — Strict category (security) with full unanimity

3 reviewers, all valid, all accept, category `security`.
Expected: **accepted / majority_accept** (strict categories require full
panel unanimity to accept, which is satisfied here).

### S10 — Strict category (security) with one dissent

3 reviewers, 2 accept + 1 reject, category `security`.
Expected: **arbitration / category_requires_full_panel_unanimity**.
**[DISAGREE→CONSENSUS]** — operator arbitrates; security findings default
to reject-leaning per majority recommendation, operator confirms reject.

### S11 — Strict category (data-loss) with a degraded panel

Configured 3 reviewers, only 2 valid (1 adapter failure), category
`data-loss`, both valid reviewers accept.
Expected: **arbitration / category_requires_full_panel_unanimity** (valid
!= configured, so strict unanimity check fails even though both valid
reviewers agree) — NOT degraded, since 2 of 3 valid still clears quorum.

### S12 — Strict category (citation-integrity), non-full valid panel accept

3 reviewers, all valid, 2 accept + 1 abstain, category `citation-integrity`.
Expected: **arbitration / category_requires_full_panel_unanimity** (Accepts
2 != ConfiguredReviewers 3).

### S13 — Quorum unmet: only 1 valid reviewer

3 configured, 2 adapter failures, 1 valid reviewer votes accept.
Expected: **degraded / quorum_unmet** at the panel level (finalizeDegraded);
no per-finding decision is computed at all because `Review` short-circuits
before clustering.

### S14 — Quorum boundary: exactly half valid

4 configured, 2 valid (2 failures), both valid accept.
Expected: **degraded / quorum_unmet** (`valid*2 <= configured` → 2*2<=4 is
true → degraded). Confirms the boundary is inclusive of "exactly half
fails", matching the documented "majority quorum with at least two valid
reviewers" contract in README.md.

### S15 — Quorum boundary: one more than half valid

5 configured, 3 valid (2 failures), 3 accept.
Expected: NOT degraded (3*2=6 > 5) — proceeds to normal consensus,
**accepted / majority_accept**.

### S16 — Insufficient non-abstain votes

3 reviewers: 1 accept, 2 abstain.
Expected: **arbitration / insufficient_non_abstain_votes** (nonAbstain=1
< 2). **[DISAGREE→CONSENSUS]** — treated as effectively no consensus;
operator rules based on the sole accept vote's reasoning, accepts.

### S17 — All abstain

3 reviewers, all abstain.
Expected: **arbitration / insufficient_non_abstain_votes**.

### S18 — Configured-unanimity option, reached

3 reviewers, `Unanimous: true`, all 3 accept (no abstain).
Expected: **accepted / majority_accept**.

### S19 — Configured-unanimity option, not reached

3 reviewers, `Unanimous: true`, 2 accept + 1 abstain (nonAbstain=2,
Accepts=2 — matches nonAbstain, so unanimity condition
`Accepts != nonAbstain` is FALSE) → falls through to weight comparison.
Expected: **accepted / majority_accept** (this card exists to confirm
abstains don't defeat "unanimous among voters" semantics — only an actual
reject defeats it).

### S20 — Configured-unanimity option, defeated by one reject

3 reviewers, `Unanimous: true`, 2 accept + 1 reject.
Expected: **arbitration / unanimity_not_reached**. **[DISAGREE→CONSENSUS]**
— operator reviews the dissent and rules reject (respecting the lone
dissenter for a unanimity-required category).

### S21 — Factual claim, evidenced

3 reviewers accept, category `factual-claim`, `EvidenceStatus: anchored`.
Expected: **accepted / majority_accept** (evidenced claims are not
downgraded).

### S22 — Factual claim, unevidenced

3 reviewers accept, category `factual-claim`, `EvidenceStatus: unevidenced`.
Expected: **unverified-claim / factual_claim_lacks_evidence**.

### S23 — Factual claim, unevidenced, majority reject

3 reviewers: 1 accept + 2 reject, category `factual-claim`, unevidenced.
Expected: **rejected / majority_reject** (the unverified-claim demotion only
applies on the accept branch, per domain code — reject wins outright
regardless of evidence status).

### S24 — Non-factual-claim category, unevidenced, majority accept

3 reviewers accept, category `correctness` (NOT factual-claim), unevidenced.
Expected: **accepted / majority_accept** — the factual-claim demotion must
be category-scoped and not leak into other categories (this guards the
historical bug fixed by repair D-022 in the original audit: "unevidenced
severity cap applied only to factual claims" — here confirming the
consensus outcome side of that same category-scoping is still correct).

### S25 — Severity divergence produces dissent record

3 reviewers: 2 accept at `major`, 1 accepts at `nit` (rank gap ≥ 2).
Expected: **accepted / majority_accept**, with `Decision.Dissent`
containing the nit-severity voter's entry (severity divergence flagged even
on the winning side).

### S26 — Reject vote against an accepted outcome flagged as dissent

3 reviewers: 2 accept, 1 reject.
Expected: **accepted / majority_accept**, `Decision.Dissent` contains the
rejecting reviewer (rejectsAccepted branch).

### S27 — Median severity computation, odd count

5 reviewers, severities on accept/reject votes: nit, minor, major, major,
blocker (sorted ranks), 3 accept + 2 reject.
Expected: **accepted / majority_accept**, `Decision.Severity` equals the
middle-ranked value (median of 5 = 3rd element).

### S28 — Median severity computation, even count

4 reviewers, 2 accept + 2 reject but weighted so accept wins narrowly (not
a tie) — severities nit, minor, major, blocker.
Expected: `Decision.Severity` uses `(len-1)/2` index per the documented
lower-median rule; confirms no off-by-one vs an actual even-count run.

### S29 — Two findings cluster into one via overlapping anchors

Two reviewers independently flag overlapping quote spans in the same
category on the same packet item.
Expected: `ClusterFindings` merges them into one cluster with 2 member IDs
and the higher of the two severities retained; voting proceeds against the
merged cluster, not two separate ones.

### S30 — Two similar findings do NOT merge (different category)

Two reviewers flag overlapping spans but different categories (`style` vs
`correctness`).
Expected: two separate clusters, two separate consensus decisions.

### S31 — Quarantined finding excluded from clustering/voting entirely

One reviewer's finding fails anchor resolution (ambiguous quote — see
repair D-039) and is quarantined.
Expected: it never enters `ClusterFindings`, never receives a vote, never
appears in `Decisions`; it still appears in `Final.Findings` marked
`Quarantined: true` with `QuarantineWhy` populated.

### S32 — Arbitration ranking: severity-first ordering

Two disputed clusters: one `minor`/tied 1-1, one `blocker`/tied 1-1 (2
valid reviewers, so nonAbstain=2, nonAbstain<2 is false, tie triggers
arbitration on both).
Expected: `RankArbitration` orders the blocker dispute first.

### S33 — Arbitration ranking: closer vote-gap tiebreak

Two disputed clusters at equal severity: one 3-3 (gap 0), one 4-2 raw but
still weighted-tied via custom weights (gap computed from raw
Accepts/Rejects, not weights).
Expected: the smaller-gap dispute (gap 0) ranks first per the documented
tiebreak, confirming `abs(Accepts-Rejects)` uses raw counts consistently.

### S34 — Arbitration overflow beyond MaxArbitration

6 disputed clusters, `Limits.MaxArbitration = 3`.
Expected: only 3 disputes appear in `Final.Arbitration`; the other 3 IDs
appear in the `overflow` list and `"arbitration_overflow"` is added to
`ReasonCodes`.

### S35 — Decision memory match sets MemoryHint, not Default

**[DISAGREE→CONSENSUS]** A prior run recorded a decision-memory ruling
"accepted" for a fingerprint-matching finding on the same packet item; the
current run reproduces a tied vote on that same finding.
Expected: dispute has `Default: "arbitration..."`-derived panel
recommendation text (NOT overwritten) and `MemoryHint: "previous ruling:
accepted"` populated separately (repair D-031 regression guard). Operator
then runs `--accept-majority`, which must read `Default`'s "accept"/"reject"
prefix — not `MemoryHint` — to decide the ruling.

### S36 — Legacy decision-memory Default format still honored

**[DISAGREE→CONSENSUS]** A hand-crafted dispute simulates a pre-repair
persisted final where `Default == "previous ruling: accepted"` (no
`MemoryHint`).
Expected: `arbitrationRulings` under `--accept-majority` still resolves to
`"accepted"` via the legacy-format fallback parse (`CutPrefix "previous
ruling: "`), confirming backward compatibility for already-persisted runs.

### S37 — Accept-majority applied across multiple disputes, one excepted

**[DISAGREE→CONSENSUS]** 3 disputes: 2 with `Default` starting "accept", 1
with `Default` starting "reject"; `--accept-majority --except <id2>`.
Expected: dispute 1 → accepted, dispute 2 → excluded entirely (not in the
ruling list), dispute 3 → rejected.

### S38 — Interactive arbitration with an explicit reject ruling

**[DISAGREE→CONSENSUS]** A tied dispute (S05-style) is arbitrated
interactively with an operator ruling of `reject`, reason "insufficient
evidence provided in dissent", operator "reviewer-lead@example.com".
Expected: `Service.Arbitrate` returns a final with the dispute resolved to
`rejected`, `Final.Arbitration` empty (no more pending disputes), exit code
reflects no remaining blocking findings if that was the only dispute, and a
`DecisionRecord` is appended to workspace decision memory.

### S39 — Arbitrate with no pending disputes

A completed run with `Status != "arbitration_pending"`.
Expected: `Service.Arbitrate` returns `ExitInvalidArguments` — "no pending
arbitration" — never silently succeeds.

### S40 — Arbitrate re-run after all disputes resolved (idempotency)

**[DISAGREE→CONSENSUS follow-up]** After S38 resolves the sole dispute,
call `Arbitrate` again on the same run.
Expected: fails cleanly (no pending arbitration) rather than re-litigating
or duplicating the decision-memory record.

### S41 — Single reviewer panel (below minimum)

1 configured reviewer, that reviewer valid, votes accept.
Expected: quorum check `ValidReviewers < 2` fires regardless of majority →
**degraded / quorum_unmet** at the panel level.

### S42 — Two-reviewer panel, split vote

2 configured, both valid, 1 accept + 1 reject.
Expected: `ValidReviewers*2 <= ConfiguredReviewers` is 4<=2 false, so quorum
passes; nonAbstain=2 not <2; not strict; not unanimous-configured; weighted
tie (equal weight 1 each) → **arbitration / vote_tie**.
**[DISAGREE→CONSENSUS]** — smallest possible panel that can disagree and
still reach consensus via arbitration.

### S43 — Two-reviewer panel, unanimous

2 configured, both valid, both accept.
Expected: **accepted / majority_accept**.

### S44 — Large panel (7 reviewers), fractious split

7 reviewers: 4 accept, 3 reject, mixed severities including one blocker
dissent.
Expected: **accepted / majority_accept**, with `Dissent` populated for the
severity-divergent and reject-side voters; confirms consensus math scales
past the default 3-reviewer panel without special-casing.

### S45 — Modify vote counts as accept for consensus purposes

3 reviewers: 2 `modify` + 1 reject.
Expected: **accepted / majority_accept** (VoteModify accumulates into
`Accepts`/`acceptWeight` per the domain code, same as VoteAccept).

### S46 — All-modify unanimous

3 reviewers, all `modify` votes.
Expected: **accepted / majority_accept**, `Decision.Accepts == 3`.

### S47 — Mixed non-strict categories in one run, independent outcomes

Single review run producing 3 findings across categories `style` (2-1
accept), `correctness` (1-2 reject), `evidence` (tied 1-1, 1 abstain, so
nonAbstain=2 not<2, weighted tie).
Expected: three independent decisions in the SAME run: accepted, rejected,
arbitration — confirms per-cluster resolution doesn't cross-contaminate.
**[DISAGREE→CONSENSUS]** on the third finding via a follow-on Arbitrate
call.

### S48 — Zero findings, unanimous "nothing to report"

3 reviewers, no findings raised by anyone.
Expected: **final / no blocking findings**, exit code 0, empty
`Decisions`/`Arbitration`, run still completes and publishes normally (not
degraded — a clean bill of health is a valid outcome, not a failure mode).

### S49 — Reviewer produces a finding but voter is unavailable for it

3 reviewers produce findings and vote; 1 voter's call fails entirely (adapter
error at vote time, after having succeeded at review time).
Expected: `"voter_unavailable"` reason code recorded; consensus computed
from the 2 remaining voters' ballots on the affected cluster(s); the panel
overall does not degrade retroactively (voter failures are post-quorum-check
by design).

### S50 — Boundary: severity rank gap of exactly 2 triggers dissent

Two reviewers accept at `major` and `nit` — confirm the exact boundary
(`abs(rank diff) >= 2`) fires dissent, and a gap of exactly 1 (`major` vs
`minor`) does NOT.
Expected: two sub-cases in one card — gap-2 flags dissent, gap-1 does not.

### S51 — Strict category with a quarantined dissenting finding

Strict category (`security`), 3 reviewers: 2 raise the same finding and
accept it, 1 raises an anchor-ambiguous variant of the same underlying issue
that gets quarantined before voting.
Expected: the quarantined finding never enters the strict-unanimity count;
the surviving cluster's decision depends only on the 2 non-quarantined
findings/votes that made it to clustering — confirms quarantine doesn't
silently corrupt strict-category headcounts.

### S52 — Replay of a disagree-then-consensus run reproduces the dispute

**[DISAGREE→CONSENSUS]** Take the S06 weighted-tie scenario to completion
(dispute recorded, `arbitration_pending`), then `Service.Replay` the run
fresh (same packet, same panel) with the identical scripted votes.
Expected: Replay reproduces the same tie/dispute deterministically — replay
is not supposed to "get lucky" and resolve on its own; determinism confirms
the panel simulation and consensus math are pure functions of the votes.

---

## Coverage note

52 scenario cards are defined above (S01–S52), exceeding the 50-scenario
minimum. Cards marked **[DISAGREE→CONSENSUS]** (S05, S06, S10, S16, S20,
S35, S36, S37, S38, S40, S42, S47, S52 — 13 cards, exceeding the 5-minimum)
specifically exercise panel disagreement (an `arbitration` first-pass
outcome) followed by a resolving second step (operator ruling,
`--accept-majority`, or a documented idempotent no-op), reaching a final
recorded consensus.
