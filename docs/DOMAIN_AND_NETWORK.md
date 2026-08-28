# Domain and network plan

## Canonical names

| Hostname | Purpose | Origin/port | Cloudflare mode | Final disposition |
|---|---|---|---|---|
| `console.onelifeltd.xyz` | Redgres Console | `127.0.0.1:8790` during migration; selected final loopback port | Tunnel + Access | Primary UI |
| `database.onelifeltd.xyz` | Legacy FastAPI PostgreSQL console | `127.0.0.1:6969` | Tunnel + Access | Retire after observation |
| `redis-admin.onelifeltd.xyz` | Legacy Redact ACL console | `127.0.0.1:8787` | Tunnel + Access | Retire after observation |
| `pgadmin.onelifeltd.xyz` | Optional pgAdmin | loopback Apache/container port | Tunnel + Access | Expert tool |
| `redis-insight.onelifeltd.xyz` | Optional RedisInsight | `127.0.0.1:5540` | Tunnel + Access | Data explorer |
| `db.onelifeltd.xyz` | PostgreSQL direct and pooled client endpoint | public `5432`, `6432` | DNS-only | Permanent raw DB endpoint |
| `rs.onelifeltd.xyz` | Redis TLS client endpoint | public `6380` | DNS-only | Permanent raw DB endpoint |

`redis-admin` always means ACL user administration. `redis-insight` always means RedisInsight. Do not reuse `redis.onelifeltd.xyz`; if it already exists, redirect/retire it deliberately to remove ambiguity.

## Traffic rules

### Browser applications

- Bind origins to `127.0.0.1` only, except the self-closing first-run bootstrap ([ADR-012](decisions/ADR-012-ui-bootstrap.md)).
- Publish through one remotely managed Cloudflare Tunnel with multiple hostname-to-service routes.
- Require Cloudflare Access policies for every administration hostname.
- Keep application authentication even behind Access; Cloudflare is an outer control, not the only login.
- Trust forwarding headers only from the local tunnel path/configured proxy, never arbitrary internet clients.

**First-run bootstrap ([ADR-012](decisions/ADR-012-ui-bootstrap.md)):** the Redgres console alone gets a temporary `0.0.0.0:8989` listener, source-restricted to the operator's IP. Domain & Network apply leaves bootstrap open under Access deny-by-default; the operator adds an Access allow email, confirms the console hostname is reachable through Tunnel + Access, then Redgres closes the bootstrap listener. Firewall-rule removal and loopback-only steady-state remain installer/ops follow-through. pgAdmin and RedisInsight never get a public listener.

### PostgreSQL

- PostgreSQL must listen on an externally reachable or private network interface if remote applications connect directly; it cannot be loopback-only in that topology.
- Require TLS and SCRAM in `pg_hba.conf`; reject non-TLS remote connections explicitly.
- Restrict source IPs in UFW/security-group rules whenever application egress IPs are stable.
- `5432` is direct and required for migrations, session features, and administrative workloads.
- `6432` is PgBouncer, normally transaction pooling for application traffic. Document client limitations.
- Redgres administrative connections use 5432, not PgBouncer.

### Redis

- Local plaintext Redis may remain bound only to loopback/container network for trusted local services.
- Public `6380` is TLS + Redis ACL only; no anonymous/default-user access.
- Restrict source IPs when possible. Redis ACLs do not replace firewalling or TLS.
- Redgres uses a dedicated Redis ACL administrator with only the commands needed to inspect status and manage ACL users.

## Public inbound ports

Expected public listeners:

- `22/tcp` — SSH, source-restricted where possible; key authentication; no password/root login according to host policy.
- `5432/tcp` — PostgreSQL direct TLS/SCRAM.
- `6432/tcp` — PgBouncer TLS/SCRAM.
- `6380/tcp` — Redis TLS/ACL.

HTTP UI origin ports must not appear in public firewall allow rules or bind to `0.0.0.0`/`::`, except the temporary, source-restricted, self-closing Redgres bootstrap on `8989` ([ADR-012](decisions/ADR-012-ui-bootstrap.md)).

## DNS/TLS ownership

- Cloudflare Tunnel terminates public browser HTTPS; no Certbot certificate is needed for the tunneled UI origin. Operator `cloudflared` wiring: [CLOUDFLARED.md](CLOUDFLARED.md).
- Certbot DNS validation is for raw PostgreSQL and Redis service certificates.
- Cloudflare access uses a self-created OAuth app with minimal scopes, or a per-zone API token as fallback; the OAuth/tunnel tokens are stored server-side only and never in browser storage or the control-state SQLite.
- The zone SSL/TLS mode is **Full (strict)** (never Flexible); the tunnel hop is already encrypted, so the loopback UI origin stays plain HTTP.
- Let's Encrypt DNS-01 issues the raw database certificates and reuses the `dns.write` permission for auto-renewal.
- Tunnel token is a bearer credential and must be rotated if exposed.
- Certificate renewal deploy hooks must copy/set ownership to service-readable paths and reload only after configuration validation.

## Domain & Network wizard (OPS-009 Partial)

Runtime wizard (System → Domain & Network) implements token-first Cloudflare apply or manual DNS instructions:

1. **Cloudflare path:** per-zone API token → apply `{zone, origin_ip, hostnames}` (console proxied CNAME + db/redis grey-cloud **A or AAAA**) → Access allow emails (API) → OAuth Connect on live console hostname → optional certbot DNS-01 for db/redis → operator confirms console reachable (human attestation; no automated probe) → bootstrap closes; optional `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD`.
2. **Manual DNS path:** apply with `dns_provider:manual` returns operator instructions; `POST /manual/confirm-access` attests Access configured manually; `POST /manual/verify` checks public DNS for db/redis; then confirm-reachable closes bootstrap.

Confirm-reachable is operator attestation only (no server-side HTTP probe) per [ADR-013](decisions/ADR-013-confirm-reachable-attestation.md). Live acceptance: [agents/OPS-009-LIVE-ACCEPTANCE.md](agents/OPS-009-LIVE-ACCEPTANCE.md).

Wizard secrets stay server-side (`0600`); never SQLite, browser storage, logs, or audit. Token-first apply (bootstrap host is not a valid OAuth callback); OAuth runs on the live console hostname. Capability is `platform.network`. Disconnect deletes only wizard-created tunnel/DNS/Access objects. Bootstrap closes on confirm-reachable success, with a 30-minute hard-cap timer. Mutations: session + CSRF + origin + audit allow-list.

Apply uses the API token file for bootstrap; steady-state Cloudflare mutations use OAuth when connected (`resolveCloudflareBearer`). OAuth callback is a session-bound GET with PKCE state (no CSRF header). See [API.md](API.md) and [CONFIGURATION.md](CONFIGURATION.md).


Documentation examples outside the OneLife profile use:

- `console.example.com`
- `db.example.com:5432/6432`
- `redis.example.com:6380`
- `pgadmin.example.com`
- `redis-insight.example.com`

The application must not hard-code `onelifeltd.xyz`; these values are deployment configuration.
