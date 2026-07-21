# Gate 7 adversarial audit

The operator workflow was executed against installed Codex, Claude, and Agy
CLIs rather than inferred from help text. This found D-016 through D-018. All
three were fixed with focused tests and a fresh-state rerun.

The rerun also demonstrated the fail-closed model-output boundary: Claude
returned invalid JSON twice, so Tribunal recorded `invalid_output` and excluded
that reviewer. Codex emitted an unrecognized packet item name for one finding;
anchor resolution quarantined it before voting. Neither failure was hidden or
converted to a successful result.

No additional actionable code finding remained after the focused suite.
External vendor response quality is recorded as an operational risk rather
than a product correctness claim.
