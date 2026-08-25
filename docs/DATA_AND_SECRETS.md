# Data and secrets contract

## Data stores

| Store | Contains | Must not contain |
|---|---|---|
| Redgres SQLite | Owner hashes, hashed session/CSRF tokens, login attempts, redacted audit events, operation state | Project passwords, credential URLs, admin URLs, TLS/private keys, Cloudflare tokens |
| PostgreSQL cluster | Project databases/roles and legacy `database_console_vault` | Redgres browser sessions or Cloudflare credentials |
| Redis data volume | Application Redis data, RDB/AOF state | Redgres owner password or Cloudflare credentials |
| Redis ACL file/config | Redis ACL users and password hashes/rules | Plaintext issued project passwords |
| `/etc/redgres` | Non-secret service configuration | World-readable secrets |
| `/etc/redgres/secrets` | Root-protected token/admin credential files | Application logs or backups without encryption |
| `/var/backups/redgres` | Consistent backup sets, checksums, manifests | Unencrypted off-host transport credentials |

## Secret classes

1. Redgres owner password: known only to operator; stored as Argon2id hash.
2. Redgres session/CSRF tokens: browser gets raw value; SQLite stores only SHA-256 hashes; expire and rotate.
3. PostgreSQL console administrator credential: gives controlled cluster privileges; file-readable by Redgres only.
4. PostgreSQL project passwords: encrypted in the legacy-compatible PostgreSQL vault; revealed only through explicit action.
5. Legacy vault key material: current Python `SESSION_SECRET`; must remain stable and root-protected throughout migration.
6. Redis ACL administrator URL/password: root-owned file consumed by Redgres service; never displayed.
7. Redis project passwords: shown only on create/rotate, not persisted by Redgres.
8. Cloudflare tunnel token: root-owned bearer token file.
9. Cloudflare Certbot DNS token: separate least-privilege root-owned file.
10. TLS private keys: readable only by root and the exact service group where necessary.
11. Backup encryption/off-host credentials: isolated from application runtime where practical.

## Credential response policy

All credential-bearing endpoints:

- use POST, never GET;
- require authentication, same-origin, and CSRF;
- use `Cache-Control: no-store, max-age=0` and `Pragma: no-cache` defense-in-depth;
- use `Referrer-Policy: no-referrer` globally;
- never include the credential in URL/query/path, redirects, request IDs, error metadata, metrics, audit records, or logs;
- return a fixed schema with explicit “one-time” semantics;
- clear frontend memory when dismissed, on logout, route change, or another target selection;
- do not support browser autofill or persistence.

PostgreSQL reveal is repeatable only because an encrypted vault entry exists. Redis create returns a one-time password (and an optional public `rediss://` URL only when both public host and port are configured). Redgres does not persist that password. Redis reveal is impossible; rotate again if lost.

## Legacy Fernet compatibility

Source record:

```text
Database: database_console_vault
Table: public.project_credentials
Primary key: role_name text
Ciphertext: encrypted_password text (ASCII Fernet token)
Timestamp: updated_at timestamptz
Key bytes: SHA-256("database-console-vault-v1:" + SESSION_SECRET)
Fernet key: urlsafe-base64(key bytes)
```

Compatibility gates:

1. Python-generated fixtures decrypt in Go.
2. Go-generated tokens decrypt in Python if Redgres ever writes legacy-format entries.
3. Invalid/tampered/wrong-key/expired-policy test vectors fail safely.
4. Copied production ciphertext records decrypt in an isolated environment using the production secret without modifying source data.
5. Role names and Unicode/password bytes round-trip exactly as UTF-8.
6. Timing/timestamps are not incorrectly treated as credential expiry.

PG-005 Partial in-process contract: `internal/secrets` decrypts committed Python `cryptography==49.0.0` fixtures (`internal/secrets/testdata/python49.json`; official [Fernet spec](https://github.com/fernet/spec/blob/master/Spec.md) and [cryptography 49.0.0 fernet.py](https://github.com/pyca/cryptography/blob/49.0.0/src/cryptography/fernet.py)) in Go using the KDF above and **no Fernet TTL** (gate 6; old timestamps still decrypt). Invalid, tampered, malformed, and wrong-key tokens fail as a single invalid-token class and must not echo the key, token, or plaintext (gate 3 without treating token age as expiry). Gate 1 and UTF-8 plaintext (gate 5) are the fixture seam. Gate 2 is not required until Redgres writes vault rows. Gate 4 copied production ciphertext remains outstanding and blocks Complete. This package does not load `REDGRES_LEGACY_VAULT_SECRET_FILE`, query PostgreSQL, or expose HTTP.

PG-005/PG-012 Partial existence GET (no decrypt): existing `GET /api/v1/postgres/databases/{db}` and `GET /api/v1/postgres/security` query whether owner role names exist in `database_console_vault.public.project_credentials`. SQL is `SELECT role_name FROM public.project_credentials WHERE role_name = ANY($1)` (or parameterized `IN` of unique owners, capped by the existing 500-database list) after the same `connectTarget` pattern used for table list, with `Database=database_console_vault`. The query must not mention `encrypted_password` or `updated_at`. Empty role lists return an empty set without connecting. Vault DB missing (`3D000`), undefined table (`42P01`), CONNECT/query denied, or timeout is an explicit unavailable class mapped to HTTP 200 `not_available` / `vault_unavailable` — not 503 and not a silent empty set. Sibling `ensure_vault()` DDL and `except Exception: return set()` are not copied. `internal/secrets` is not on this request path. `REDGRES_LEGACY_VAULT_SECRET_FILE` is not loaded. POST reveal and Gate 4 remain outstanding.

Do not rotate or repurpose the legacy secret in the same change as the application migration. A future dedicated key/versioned envelope needs an ADR and reversible migration tool.

## Configuration files

Recommended:

```text
/etc/redgres/redgres.env                         root:redgres 0640
/etc/redgres/secrets/postgres-admin-password     root:root    0600
/etc/redgres/secrets/legacy-vault-secret         root:root    0600
/etc/redgres/secrets/redis-admin-url              root:root    0600
/etc/redgres/secrets/cloudflared-tunnel-token     root:root    0600
/etc/redgres/secrets/certbot-dns-token            root:root    0600
```

If systemd runs as `redgres`, root-only secret files cannot be read directly after privilege drop. Prefer systemd credentials (`LoadCredential=`) or a narrowly permissioned `root:redgres 0640` application secret file. The implementation must choose and document one coherent mechanism; do not publish an impossible permission model.

## Rotation

Every secret class needs owner, trigger, steps, verification, rollback/compensation, and dependent-service inventory. Rotating a database/Redis project credential requires explicit downstream application coordination; Redgres cannot update unknown consumers automatically.
