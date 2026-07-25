# Repair Session — Latest Pointer Compatibility

**Date:** 2026-07-25
**Mode:** Existing runtime finding; one low-risk defect
**Selected defect:** D-081
**Patch count:** 1 of 10

## Orientation and contract

An operator-authorized governance recommendation over a Cephalopod decision
packet completed as Tribunal run `01KYDAM0HM2NP7QXXKSEKC7MRT`. The immediately
following command

```text
./bin/tribunal status /tmp/cephalopod-fnd-review.ARhpoA --json
```

failed with `decode latest.json: json: unknown field "status"`.

The active I/O contract says workspace `latest` projections live outside the
reviewed workspace and readers ignore unknown fields within a supported
version. `completePublication` writes `schema_version`, `run_id`, `status`,
and `updated_at`. `locateRun` instead used `ReadJSONStrict` against a struct
containing only `schema_version` and `run_id`. The pre-repair disposition was
an implementation defect against the active contract; no competing
architecture source was found.

D-074 was excluded from this repair. The same live run confirms that Claude
can still return a foreign flat response shape after the prompt/schema repair.
That provider-compatibility problem is broader and remains explicitly partial
in the defect ledger.

## Test-first evidence and patch

The real synthetic three-reviewer workflow now calls `Service.Status` without
an explicit run ID after current publication. Before the patch:

```text
go test ./internal/tribunal/app \
  -run '^TestReviewPersistsBarrierAndCompletesWithoutGit$' -count=1
```

failed on `json: unknown field "status"`.

The patch changes only the workspace latest-projection read to `ReadJSON`.
JSON syntax and trailing data remain rejected by `json.Unmarshal`; the
existing explicit checks still require schema version 1, a non-empty run ID,
a safe single path component, and a valid run directory. Closed run artifacts
continue using strict readers.

## Validation

- The focused regression passes.
- `go test ./internal/tribunal/app -count=1` passes.
- A rebuilt `./bin/tribunal` successfully resolves and renders the actual
  arbitration run through the implicit latest pointer.
- `git diff --check` passes.
- `scripts/check.sh` is the final gate for the exact repair state.

## Adversarial review

The change does not broaden paths, accept a new schema version, mutate review
state, or weaken strict decoding of canonical run artifacts. It restores the
declared compatible-reader behavior only for the workspace projection whose
current writer already owns the additional fields. The existing explicit
identity and run-directory checks remain unchanged.

## Record closure

- D-081: added and closed with regression and live-runtime evidence.
- D-074: corrected from `fixed; live re-run pending` to `partial` because the
  new live run supplies contrary provider evidence.
- Changelog and test ledger: refreshed for the latest-pointer behavior.

Post-repair drift disposition: **no new drift**.
