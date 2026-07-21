# Gates 3–5 adversarial audit

Audited provider locks, mandatory terminal persistence, prompt/semantic
identity, provider envelopes, lifecycle sequencing, packet artifact coverage,
blind-vote identity stripping, command reachability, external-only state,
edit/revert mutation boundaries, release identity, and forbidden legacy/Git
surface.

Five defects were entered as D-001 through D-005 and fixed. Regression tests
cover the lock, synthetic barrier run, local HTTP CLI path, Claude envelope,
and terminal artifact sequence. Remaining gaps are tracked in the traceability
matrix rather than being labeled verified.
