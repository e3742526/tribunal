# Defect Ledger

| ID | Gate | Finding | Root cause | Disposition | Regression evidence |
|---|---|---|---|---|---|
| D-001 | 4 | Serialized provider lock failed when its parent did not exist | lock assumed precreated directory | fixed: lock creates private parent | storage lock tests |
| D-002 | 4 | degraded final ignored publication failure | error discarded in terminal helper | fixed: mandatory error propagation | application synthetic run |
| D-003 | 5 | first-pass prompt omitted required reviewer ID | semantic contract not represented in prompt | fixed: explicit bound ID | CLI local HTTP E2E |
| D-004 | 5 | Claude JSON envelope reached schema validator intact | adapter returned transport envelope | fixed: bounded structured-output unwrap | adapter regression test |
| D-005 | 4 | lifecycle omitted clustered/consensus/recommended states and active pointer | initial orchestration persisted only major phases | fixed: complete journal sequence and pointers | application artifact assertions |
| D-006 | 6 | top-level symlink input was accepted after canonicalization | `Lstat` occurred on resolved target | fixed: inspect requested path before resolution and again after read | document symlink regression |
| D-007 | 6 | injected HTTP client bypassed worker redirect policy | redirect callback installed only on default client | fixed: clone every client, enforce redirect target and public-IP dialing | redirect escape regression |
| D-008 | 6 | panel weights were clamped on a copy and never enabled in consensus | validation/value semantics mismatch | fixed: pointer normalization and weighted resolution | domain weight table |
| D-009 | 6 | same-millisecond ULIDs could collide | raw entropy used for each ID | fixed: locked monotonic entropy | consecutive fixed-clock ULID test |
| D-010 | 6 | arbitration/edit/revert lacked run locks and default editing selected disabled Claude | mutation paths assumed CLI serialization | fixed: run revalidation/locks and capable editor selection | app edit and lock suites |
| D-011 | 6 | blind packets retained reviewer-derived finding IDs | identity fields removed but IDs left intact | fixed: deterministic opaque ballot IDs and host mapping | blind-packet regression |
| D-012 | 6 | TOML/user JSON accepted unknown fields | decoders did not inspect undecoded keys/strict recovery | fixed: fail-closed config, persona, arbitration, and proposal decoding | config/contract tests |
| D-013 | 6 | edit apply did not reread immediately before write; multi-file revert lacked rollback | validation and mutation separated by a race window | fixed: pre-write hash check and revert rollback | edit stale/user-change suite |
| D-014 | 6 | lock and JSONL paths could follow pre-existing symlinks | append/open calls lacked no-follow policy | fixed: `O_NOFOLLOW` locks and non-regular journal rejection | external sentinel regression |
| D-015 | 6 | deterministic chunk map was not part of packet identity | hash computed before split projection | fixed: manifest/chunks/redactions in projection and rehash after split | packet identity regression |
