# Domain & Network wizard — pre-implementation security review

Status: **pre-code review notes.** This is the threat/control analysis that grounds OPS-008/OPS-009 and [ADR-012](decisions/ADR-012-ui-bootstrap.md) before any implementation. It is **not** the final post-implementation security review required by [SECURITY.md](SECURITY.md) §9.

## Scope

- **OPS-008** — self-closing, source-restricted bootstrap listener (`0.0.0.0:8989` → `127.0.0.1:8790`).
- **OPS-009** — Domain & Network wizard: hostname entry, Cloudflare OAuth (or per-zone API token fallback), server-side tunnel + DNS + Let's Encrypt + Access provisioning, live verification, bootstrap auto-close.

## Secrets touched

| Secret | Class | Storage |
|---|---|---|
| Cloudflare OAuth access + refresh token | bearer (new — see [DATA_AND_SECRETS.md](DATA_AND_SECRETS.md) #9) | systemd credential / `/etc/redgres/secrets`, `0600` |
| Cloudflare tunnel token | bearer (existing #8) | systemd credential, `0600` |
| Let's Encrypt account private key | crypto | root-owned file, `0600` |
| DB TLS private keys | crypto (#11) | root / exact service group only |

None of these may appear in SQLite control state, browser storage, logs, audit metadata, or error JSON (invariant #3).

## Threats and controls

| Threat | Required control |
|---|---|
| OAuth/tunnel token theft from the browser | Tokens never persisted in browser storage; submitted once, held server-side only, never returned by the API; credential responses `no-store`; frontend memory cleared on dismiss/navigation. |
| OAuth authorization-code interception / CSRF on callback | Bind an unguessable `state` to the owner session + PKCE; single-use code; validate the callback origin. |
| OAuth redirect over a self-signed bootstrap host | Resolve before code (see open questions): the bootstrap `https://<VPS_IP>:8989` is self-signed, but Cloudflare requires a valid HTTPS redirect URI. |
| Over-broad grant / cross-zone reach | Enforce the minimal-scope allow-list at authorization; document that OAuth scopes are account-wide across zones (the wizard still only *acts* on the declared zone). |
| Zone guessing / touching other zones | Resolve the zone deterministically from explicit hostnames; never enumerate or guess; touch only the records it declares. |
| Tunnel token exposure | Generated server-side; stored `0600`; injected to `cloudflared` via systemd `LoadCredential`, never unit text, env dumps, or shell history. |
| CSRF / unauthorized wizard actions | Every wizard mutation is POST + session + CSRF + origin check + a dedicated capability (e.g. `platform.network`); audit each action. |
| Secret leakage in logs/audit/errors | Central redactor covers OAuth/tunnel tokens, DNS-01 `TXT` challenge values, and private keys; audit uses a per-action metadata allow-list. |
| Bootstrap left open (fail-open) | Auto-close is mandatory **and** a max-lifetime timer closes `8989` even if the wizard errors or crashes. |
| Partial failure / duplicate DNS/tunnel/cert | Idempotent apply; re-run converges without duplicates; long steps use the operations ledger (ADR-010); partial failures reported secret-safe. |
| Unauthorized domain change / disconnect | "Disconnect" revokes the Cloudflare grant and removes only records/tunnel the wizard created; it never sweeps the zone or touches unrelated records. |
| Raw DB TLS downgrade | DB certs stay Let's Encrypt (publicly trusted); client guidance remains `verify-full` / Redis hostname verification — the wizard must not relax this. |

## Open questions (resolve before code)

1. **OAuth redirect URI** — where does the callback land given the self-signed bootstrap host? Options: run the handshake after the tunneled domain exists (callback on `console.example.com`), or use a loopback redirect. Decide explicitly.
2. **Tunnel scope name** — confirm `Cloudflare One Connectors` vs `Argo Tunnel (Legacy)` against Cloudflare's current API permission mapping.
3. **Capability name** — add a dedicated capability (e.g. `platform.network`) for the wizard rather than reusing `platform.read`.
4. **Disconnect semantics** — revoke-only, or revoke + delete only-wizard-created resources? (Recommend the latter, never a zone sweep.)
5. **Bootstrap max lifetime** — pick a hard cap (e.g. 30–60 minutes) so `8989` can never remain open after an abandoned setup.

## Implementation invariants

- All wizard endpoints: POST-only, authenticated, same-origin, CSRF, capability-gated.
- Secrets live server-side only; the browser holds nothing persistent.
- Scope allow-list verified at authorization; unexpected scopes are rejected or surfaced, never silently accepted.
- Every action audited with a secret-safe allow-list (hostname, zone id, record type, tunnel id, outcome, request id, client IP).
- Idempotent apply; fail-closed bootstrap close; no unrelated DNS deletion.

## Required follow-ups

- Independent post-implementation security review (SECURITY.md §9).
- Canary-secret tests proving OAuth/tunnel tokens and DNS-01 challenge values never reach logs/audit/errors/SQLite/browser storage.
