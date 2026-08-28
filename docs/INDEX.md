# Documentation index

Do not preload this file. Agents and humans route from [AGENTS.md](../AGENTS.md). Setup: [README.md](../README.md). Public site: [site/](../site/) → https://redgres.com/ (not this `docs/` tree).

## Canonical owners

Listed in `AGENTS.md` (product, HTTP, architecture, versions, UI, config, installer, PostgreSQL, backup, security, parity, tests, milestones). Status matrix: [TRACEABILITY.md](TRACEABILITY.md). ADRs: [decisions/](decisions/).

## Operator extras (not always-loaded)

- [GLOSSARY.md](GLOSSARY.md)
- [RISK_REGISTER.md](RISK_REGISTER.md)
- [CLOUDFLARED.md](CLOUDFLARED.md) — cloudflared systemd (`LoadCredential` + ingress)
- [agents/OPS-009-LIVE-ACCEPTANCE.md](agents/OPS-009-LIVE-ACCEPTANCE.md) — live domain gates G1–G9

## Status labels

- **Verified current**: inspected in a named source repository and pinned baseline.
- **Target**: approved design, not necessarily implemented.
- **Provisional**: must be checked against the live VPS/provider state.
- **Gate**: objective evidence required before progression.
