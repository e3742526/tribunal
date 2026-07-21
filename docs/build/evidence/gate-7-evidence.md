# Gate 7 evidence — 2026-07-21

The README, user/developer manuals, architecture, configuration, I/O contract,
troubleshooting, security, contribution, release, changelog, and test ledger
describe the implemented Tribunal surface. Obsolete predecessor campaigns,
screenshots, logs, and architecture documents were removed after durable design
decisions were captured in ADRs.

Authenticated bounded smoke history:

1. The first run was deliberately terminated after adapter contract failures.
   It durably published `ABORTED` and exit 6.
2. The second run classified Agy valid and exposed Codex provider-schema and
   Claude output-contract failures without constructing consensus.
3. After D-016 through D-018 were fixed, a fresh run completed in 93 seconds.
   Codex and Agy returned valid independent findings and blind votes; Claude's
   invalid output was isolated. Majority quorum held, two recommendations were
   accepted, one tie entered arbitration, and the command returned exit 2.

The final smoke used an external state root, a 5-minute wall cap, 50k-token
preflight ceiling, and 256 KiB per-call output cap. The reviewed file remained
unchanged. This verifies successful real invocation of all three configured
families and the required degraded-member/quorum behavior; it does not claim
that a vendor will always obey the schema.

Focused tests after the fixes:

- `go test ./internal/tribunal/adapters ./internal/tribunal/app ./internal/cli`
  — pass.
- Provider-schema regression recursively requires explicit types and every
  declared property in the strict output surface.
- Codex/Agy argv regression covers non-Git execution and duration syntax.
