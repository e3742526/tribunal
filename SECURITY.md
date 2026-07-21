# Security policy

Report vulnerabilities privately through the repository's GitHub Security
Advisory form. Do not publish exploit details in an issue.

Important boundaries include untrusted document, persona, model, and fetched
content; exact-domain network allowlists; subprocess environment restriction;
external state path containment; durable locks and writes; schema validation;
secret redaction; and host-only edit/revert enforcement.

Include the affected version, reproduction, impact, and whether later user
changes or external state are at risk. Tribunal `v0.1.0` supports macOS and
Linux release artifacts. Windows is not a supported release target.
