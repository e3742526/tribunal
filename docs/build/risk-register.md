# Risk Register

| ID | Risk | Likelihood/impact | Mitigation | Status |
|---|---|---|---|---|
| R-001 | Legacy Git/coding behavior leaks into Tribunal | medium/blocker | replacement packages, final source scan | closed: packages removed; no Git subprocess |
| R-002 | Model prompt isolation is only conventional | medium/blocker | isolated cwd/env/argv plus forbidden-text tests | mitigated; authenticated bounded smoke completed without tool/workspace access |
| R-003 | Path replacement escapes a trusted root | medium/blocker | canonical revalidation before sensitive I/O | closed for tested boundaries; syscall race residual documented |
| R-004 | Persistence failure reports success | low/blocker | journal-first and mandatory terminal writes | mitigated by terminal propagation, abort/resume, and repetition |
| R-005 | Optional extractor/provider behavior drifts | medium/major | doctor, caps, golden tests, typed unavailable result | mitigated; live smoke caught and closed three argv/schema drifts; vendor output quality remains external |
| R-006 | Broad fork migration deletes useful substrate | medium/major | port with tests before exact legacy deletion | closed: replacement green before exact deletion |
