# ADR-0006: Allowlisted evidence workers

Accepted. Deterministic checks precede model interpretation. HTTP workers use
exact trusted host allowlists, reject private/link-local destinations and
redirect escapes, and cap time and bytes. Evidence stores source, retrieval
time, excerpt, and content hash; fetched text is fenced as untrusted.

