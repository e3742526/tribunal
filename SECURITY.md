# Security Policy

## Reporting

If you find a security issue in `tagteam`, please do not open a public issue with exploit details.

Use GitHub's
[private vulnerability reporting form](https://github.com/cephalopod-ai/tagteam/security/advisories/new)
to report it privately to the maintainer with:

- a description of the issue;
- affected version or commit;
- reproduction steps or proof of concept;
- impact assessment.

Do not include vulnerability details in a public issue. If the private form is
unavailable, open a minimal public issue asking the maintainer to restore the
private reporting channel without disclosing the exploit.

## Scope

Security issues may include:

- command injection or unsafe shell execution;
- prompt or artifact handling that can cause unintended destructive actions;
- secret leakage in run artifacts or logs;
- sandbox or permission-boundary bypasses;
- unsafe adapter behavior that causes execution outside the intended workdir.

## Expectations

`tagteam` orchestrates third-party agent CLIs. Security fixes may require coordinated changes across prompt construction, adapter argv construction, run artifact handling, and preflight validation.
