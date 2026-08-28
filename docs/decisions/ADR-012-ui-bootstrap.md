# ADR-012: Self-closing, source-restricted UI bootstrap

Status: Accepted
Date: 2026-08-27

## Context

[ADR-003](ADR-003-network-boundaries.md) requires browser applications to bind loopback only and be reached through Cloudflare Tunnel + Access, and states that UI origin ports are not publicly exposed. In practice the first-time operator must reach the Redgres console *before* the tunnel and domain exist; otherwise setup forces an SSH tunnel or pre-configuring Cloudflare at install time, which [DOMAIN_AND_NETWORK.md](../DOMAIN_AND_NETWORK.md) and [README.md](../../README.md) deliberately avoid (install first, domain wizard after login).

## Decision

- During first install only, the Redgres console binds a **bootstrap** listener on `0.0.0.0:8989` and opens a UFW rule **source-restricted to the operator's current IP**.
- The operator completes the Domain & Network wizard (OAuth/token, tunnel, DNS, TLS, Access) from that bootstrap URL.
- Once the tunnel + Access are verified live, Redgres **auto-rebinds to `127.0.0.1:8790` and removes the `8989` firewall rule**; the bootstrap port is never left open.
- pgAdmin and RedisInsight remain loopback + Tunnel + Access only and are never given a bootstrap or public listener.
- Re-entry after close is via SSH (re-run the bootstrap or use an SSH tunnel), never a permanently open port.

## Consequences

- This is a bounded exception to ADR-003's "UI origin ports are not publicly exposed": a temporary, source-restricted, self-closing UI listener.
- The bootstrap window still relies on the owner password plus rate limiting; the source restriction is the primary boundary.
- Internet-wide scanning (Shodan/masscan) makes a permanently open UI port unacceptable, so the auto-close is mandatory, not optional.
- The Domain & Network wizard and this bootstrap journey are recorded as PRD OPS-008/OPS-009; operator steps are [DOMAIN_AND_NETWORK.md](../DOMAIN_AND_NETWORK.md) and [README.md](../../README.md).
