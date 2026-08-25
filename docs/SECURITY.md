# Security architecture and threat model

## 1. Security objective

Redgres holds authority to create, modify, reveal, rotate, and destroy database access. Treat compromise of its owner session or administrator credentials as a high-impact control-plane incident. Cloudflare Access reduces exposure but does not eliminate application-level controls.

## 2. Trust boundaries

1. Internet ↔ Cloudflare edge.
2. Cloudflare Tunnel ↔ loopback Redgres origin.
3. Browser ↔ Redgres session/API.
4. Redgres process ↔ SQLite state.
5. Redgres process ↔ PostgreSQL administrative connection/vault.
6. Redgres process ↔ Redis ACL administrator connection.
7. Raw application clients ↔ public PostgreSQL/Redis TLS endpoints.
8. Host ↔ containers, filesystem secrets, backups, and off-host storage.

Cloudflare, the VPS provider, OS root, and backup administrator are privileged trust anchors. Document this honestly; Redgres cannot defend against a fully compromised root host.

## 3. Principal threats and controls

| Threat | Required controls |
|---|---|
| Credential theft from response/cache/history | POST-only reveal/issue, `no-store`, no-referrer, no URL secrets, frontend memory clearing, Access + app auth |
| Session theft/fixation | 256-bit random opaque tokens, hash-at-rest, regenerate on login, idle+absolute expiry, Secure/HttpOnly/SameSite Strict, logout deletion |
| CSRF | Same-origin Origin/Referer validation plus per-session CSRF token for all mutations |
| Brute force | Argon2id, generic login errors, persistent username+IP throttling (5 failures / 15m, exponential then 15m cap), Cloudflare Access/rate controls |
| SQL injection/identifier confusion | Parameterized values, `pgx.Identifier`/quoted identifiers, strict normalized identifiers, no arbitrary SQL endpoint |
| Redis privilege escalation | Dedicated ACL admin, protected users, one prefix, `-@all`, explicit command allow-list, no generic command API |
| Destructive operator mistake | Disabled-by-default features, protected targets, typed confirmation, fresh reauth, exact impact, audit, pre-action backup policy |
| Secret leakage in logs/audit/errors | Central redactor, structured allow-listed metadata, browser-safe typed errors, tests with canary secrets |
| Compromised dependency/build | Lockfiles, CI scans, SBOM, signed/checksummed releases, minimal production runtime |
| Tunnel bypass/UI direct exposure | Loopback bind, firewall verification, CI/config fail-closed, socket-listener verification |
| Raw database attack | TLS, SCRAM/ACL, least privilege, source firewall restrictions, patching, connection/rate monitoring |
| Backup theft or unusable backup | root-only local permissions, encryption off-host/in transit, checksums, retention, periodic isolated restore |
| Vault incompatibility/key loss | Immutable legacy secret backup, compatibility vectors, copied-record dry run, no in-place conversion during cutover |
| Cross-site scripting | React escaping, no unsafe HTML by default, self-only CSP, no external runtime script/font CDN |
| SSRF | No arbitrary connection tester/URL fetch endpoint; admin endpoints come only from trusted server config |

`rediss` administrator URLs that set go-redis `skip_verify` so TLS `InsecureSkipVerify` is true are rejected in every environment; the error names `REDGRES_REDIS_ADMIN_URL_FILE` and never includes the URL or password.

## 4. Authorization model

Migration release has one owner role. Authorization remains capability-based internally so future roles do not require rewriting handlers:

- `platform.read`
- `audit.read`
- `postgres.read`, `postgres.provision`, `postgres.credentials`, `postgres.destructive`
- `redis.read`, `redis.provision`, `redis.credentials`, `redis.destructive`

The owner receives all capabilities. HTTP handlers ask the authorization service; they do not assume “authenticated means allowed.” Cloudflare identity is not automatically mapped to a Redgres role in v1.

## 4.1 Owner password and bootstrap (implemented)

- Owner bootstrap is CLI-only: `redgres create-owner --username NAME [--sqlite-path PATH] [--replace]`. There is no HTTP bootstrap route. An existing owner is not overwritten unless `--replace` is set.
- Passwords are hashed with Argon2id (`golang.org/x/crypto/argon2` `IDKey`) using RFC 9106’s second option: time=3, memory=64 MiB, threads=4, 16-byte salt, 32-byte key. The stored value is the PHC string `$argon2id$v=19$m=65536,t=3,p=4$…` as UTF-8 bytes in `owners.password_hash`.
- v1 password policy (NIST SP 800-63B-4 / OWASP, no MFA): at least 15 Unicode code points; reject empty/all-whitespace and passwords equal to the normalized username; reject bodies longer than 1024 bytes; no composition rules. There is no `REDGRES_MIN_PASSWORD_LENGTH` setting.
- Session and CSRF tokens are 256-bit hex; SQLite stores SHA-256 only. Login deletes prior sessions for that owner.
- Lockout is per normalized username + `RemoteAddr` host. Forwarded-for / `CF-Connecting-IP` headers are not trusted.
- Login success and logout fail closed if the audit insert fails.
- Redis user create fail-closes if the audit insert fails after `ACL SETUSER` succeeds; the one-time credential is not returned. Named-preset create (`cache-read-write`, `read-only`, `queue-worker`) uses the same fail-closed path. Named-preset PATCH fail-closes the same way after prefix/grant `ACL SETUSER` and does not return `user`; it uses `redis.provision`, not `redis.destructive`. Enable/disable fail-closes the same way after `on`/`off` and do not return `user`. Rotate fail-closes the same way after `resetpass`/`>password` and does not return `credential`, `password`, or `user`.
- GET `/api/v1/redis/presets` is a session + `redis.read` catalog of named command sets. It is not a credential-bearing route: it does not return passwords, URLs, or tickets, does not call Redis, and does not write audit.

## 5. Protected resources

Minimum PostgreSQL deny set:

```text
postgres
template0
template1
database_console_vault
redgres (if a PostgreSQL control database is ever introduced)
```

Also deny configured maintenance/admin databases and any database whose owner is a protected role. Minimum protected PostgreSQL roles include `postgres`, `database_console`, `onelife_pg_admin`, PgBouncer auth roles, Redgres admin roles, and built-in `pg_*` roles.

Minimum Redis protected users include `default`, `admin`, legacy `redact_admin`, and the configured Redgres Redis administrator. Protection is case-normalized according to Redis username semantics used by the adapter.

Protection checks occur in use cases immediately before mutation and are tested even when feature flags are enabled.

## 6. Destructive-operation protocol

For drop/truncate/row delete/Redis user delete:

1. Feature/capability enabled.
2. Current authenticated session and valid CSRF/origin.
3. Resource re-read from the backend.
4. Protected-resource policy passes.
5. Exact target typed confirmation passes.
6. Fresh owner-password reauthentication passes (or a short-lived server-side reauth grant in a future ADR).
7. Optional backup freshness requirement passes for database-level destruction.
8. Mutation executes with timeouts/target lock.
9. Success/failure and safe metadata are audited.
10. Response reports actual effect and any partial failure.

Never make browser confirmation the sole guard.

## 7. HTTP controls

- CSP: `default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`.
- `X-Content-Type-Options: nosniff`.
- `Referrer-Policy: no-referrer`.
- `Permissions-Policy: accelerometer=(), autoplay=(), camera=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), screen-wake-lock=(), usb=(), xr-spatial-tracking=()`.
- `Cross-Origin-Opener-Policy: same-origin`.
- The origin does not set `Strict-Transport-Security`. Cloudflare owns HSTS so the application cannot emit a conflicting partial policy.
- `style-src 'self'` (no `unsafe-inline`) blocks React `style={{…}}` attributes. Use stylesheets/classes, or propose a reviewed CSP change.
- No wildcard CORS. Same-origin deployment normally needs no CORS middleware.
- Request bodies, timeouts, headers, and concurrent long operations are bounded.
- Health endpoint reveals no versions, hostnames, secrets, or internal errors to unauthenticated callers.

## 8. Service hardening

The systemd unit should use a dedicated unprivileged user, `UMask=0077`, `NoNewPrivileges=true`, `PrivateTmp=true`, `ProtectSystem=strict`, `ProtectHome=true`, explicit `ReadWritePaths=/var/lib/redgres`, restricted address families, and the minimum required capabilities (normally none). Validate each directive against SQLite, DNS/TLS, and credential loading; do not copy hardening flags blindly.

Redis container runs without publishing plaintext 6379 publicly, uses a pinned image digest/version, a read-only root filesystem where compatible, dropped capabilities, resource limits, restart policy, and explicit persistent mounts. PostgreSQL/PgBouncer remain dedicated host services with least-readable TLS keys.

## 9. Secret scanning and tests

CI must scan repository history/diff for secrets and run canary tests that place recognizable fake credentials in adapter errors and verify absence from:

- structured logs;
- audit metadata;
- HTTP errors;
- request IDs/metrics;
- SQLite state;
- frontend storage/snapshots.

Security review is mandatory for auth, crypto, URL generation, destructive operations, Redis command sets, proxy/IP handling, install scripts, and backups.

## 10. Incident priorities

1. Contain public exposure or active account compromise.
2. Preserve evidence without logging additional secrets.
3. Rotate the exact secret class and all dependent credentials.
4. Verify service/configuration and review audit/host logs.
5. Restore/rebuild only from validated artifacts.
6. Document root cause and add a regression test/runbook change.

Specific procedures belong in [OPERATIONS.md](OPERATIONS.md); live commands must be validated against the actual host first.
