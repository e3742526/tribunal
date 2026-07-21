# Requirement Traceability Matrix

| REQ | Pri | Requirement | Acceptance | Design | Tests/evidence | Status |
|---|---|---|---|---|---|---|
| REQ-001 | P0 | Clean Tribunal identity, no Tagteam compatibility | binary/module/config/artifacts use Tribunal only | ADR-0001 | Gate 3/8 scans | planned |
| REQ-002 | P0 | Git-free canonical document packets | review succeeds with Git absent and workspace unchanged | ADR-0002 | packet + CLI E2E | planned |
| REQ-003 | P0 | Independent reviewer barrier and delivery records | no pass-1 prompt contains peer output | ADR-0004 | prompt/golden tests | planned |
| REQ-004 | P0 | Typed findings, anchors, evidence and quarantine | invalid anchors never vote | architecture | domain/document tables | planned |
| REQ-005 | P0 | Exact quorum, consensus, dissent and arbitration | exhaustive decision tables pass | ADR-0003 | domain tables | planned |
| REQ-006 | P0 | Durable external state and resumable lifecycle | faulted mandatory writes cannot report success | ADR-0002 | fault/resume E2E | planned |
| REQ-007 | P0 | Fail-closed canonical paths and locks | replacement/escape/contention tests pass | ADR-0002 | path/multiprocess tests | planned |
| REQ-008 | P0 | Secret/PII scan and value redaction | secrets absent from persisted artifacts | ADR-0002 | redaction fixtures | planned |
| REQ-009 | P0 | Bounded output, calls, tokens and time | each cap reaches typed terminal behavior | architecture | cap tables | planned |
| REQ-010 | P0 | Findings ledger and decision memory | cross-run lifecycle and rulings persist | ADR-0003 | storage round trips | planned |
| REQ-011 | P0 | Separate, scoped, stale-safe edit/revert | over-broad/stale edits fail without mutation | ADR-0005 | edit integration | planned |
| REQ-012 | P1 | Codex, Claude, Agy and OpenAI-compatible reviewers | golden requests plus bounded smoke | ADR-0004 | adapter suites | planned |
| REQ-013 | P1 | Personas and diversity disclosure | lint/fence/grid tests pass | ADR-0004 | persona tables | planned |
| REQ-014 | P1 | Deterministic and network evidence workers | typed provenance, allowlist and SSRF tests pass | ADR-0006 | local HTTP tests | planned |
| REQ-015 | P1 | Generic/manuscript/strategy/governance rubrics | each rubric validates and hashes | architecture | rubric goldens | planned |
| REQ-016 | P1 | Plaintext, Markdown, DOCX and PDF review | extraction fixtures produce anchored packets | ADR-0007 | extractor tests | planned |
| REQ-017 | P1 | Markdown/HTML reports, bench and status TUI | outputs escape content and consume RunSnapshot | architecture | report/TUI/bench tests | planned |
| REQ-018 | P1 | Complete CLI and JSON contracts | every specified command has a real handler | io-contract | CLI tests | planned |
| REQ-019 | P1 | Rebranded CI/release/security/docs | clean-checkout check and archive smoke | ADR-0001 | Gate 8 | planned |
| REQ-020 | P1 | Evidence-driven handoff artifacts | every P0/P1 ends verified/deferred/cut with evidence | build contract | Gate 8 audit | planned |

