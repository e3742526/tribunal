# Risk Register

| ID | Risk | Likelihood/impact | Mitigation | Status |
|---|---|---|---|---|
| R-001 | Legacy Git/coding behavior leaks into Tribunal | medium/blocker | replacement packages, final source scan | open |
| R-002 | Model prompt isolation is only conventional | medium/blocker | isolated cwd/env/argv plus forbidden-text tests | open |
| R-003 | Path replacement escapes a trusted root | medium/blocker | canonical revalidation before sensitive I/O | open |
| R-004 | Persistence failure reports success | low/blocker | journal-first and mandatory terminal writes | open |
| R-005 | Optional extractor/provider behavior drifts | medium/major | doctor, caps, golden tests, typed unavailable result | open |
| R-006 | Broad fork migration deletes useful substrate | medium/major | port with tests before exact legacy deletion | open |

