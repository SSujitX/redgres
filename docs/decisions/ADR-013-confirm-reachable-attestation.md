# ADR-013: confirm-reachable uses operator attestation (v1)

Status: Accepted
Date: 2026-08-28

## Context

[ADR-012](ADR-012-ui-bootstrap.md) states that bootstrap closes once tunnel + Access are "verified live." `POST /api/v1/domain/confirm-reachable` closes the bootstrap listener and optionally runs `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD`. The handler does **not** perform an automated HTTP probe to the console hostname.

Reviewers asked whether v1 should add a server-side reachability check before closing bootstrap and removing firewall rules.

## Decision

- **v1 (OPS-009 Partial → Complete acceptance):** `confirm-reachable` remains **operator attestation only**. The operator must open the console hostname through Cloudflare Tunnel + Access in a browser, then confirm in the UI (hostname echo) or call the API with session + CSRF + `platform.network`.
- **No automated probe in v1.** Redgres does not curl the public hostname from the server before close (Access cookies, Cloudflare identity headers, and split-horizon DNS make a naive probe misleading or brittle).
- **UFW removal** stays best-effort via `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD` after successful audit; failure returns `bootstrap_ufw_removed: false` and does not block bootstrap close.
- **Live acceptance evidence** (see [OPS-009-LIVE-ACCEPTANCE.md](../agents/OPS-009-LIVE-ACCEPTANCE.md)) must include operator-recorded proof that the console was reachable **before** confirm-reachable (redacted browser screenshot or curl through Access with session, not server-side automation).

## Consequences

- Closes the product tension between ADR-012 prose ("verified live") and implementation: **human verification is the v1 definition of verified** for bootstrap close.
- A future slice may add an **optional** probe (loopback tunnel health, cloudflared metrics, or Access-aware check) behind explicit config; that is out of scope for OPS-009 Complete.
- Security reviewers must treat confirm-reachable as irreversible operator action, not as automated health validation.

## Related

- [API.md](../API.md) — `POST /api/v1/domain/confirm-reachable`
- [DOMAIN_AND_NETWORK.md](../DOMAIN_AND_NETWORK.md) — wizard step 6
- PRD OPS-008, OPS-009
