# OPS-009 live acceptance runbook

**Purpose:** Operator-executed evidence gate for OPS-009 Complete. Code is validated with fakes in CI; this runbook proves the critical path against **real** Cloudflare, certbot, cloudflared, and UFW on **one throwaway Ubuntu host** and **one dedicated test zone**.

**Authority:** PRD OPS-009, [DOMAIN_AND_NETWORK.md](../DOMAIN_AND_NETWORK.md), [CLOUDFLARED.md](../CLOUDFLARED.md), [CONFIGURATION.md](../CONFIGURATION.md), [ADR-013](../decisions/ADR-013-confirm-reachable-attestation.md).

**Agents must not** run this runbook against production DNS, real customer zones, or commit secrets. **You** execute; agents update code/docs from failures you report.

---

## Before you start

### Baseline

Record in every evidence file:

```text
baseline_commit: <git rev-parse HEAD>
run_date: YYYY-MM-DD
ubuntu: <lsb_release -ds>
zone: <test-zone.example.com>   # dedicated test zone, not production
```

### Throwaway environment

| Item | Requirement |
|---|---|
| VM | Ubuntu 22.04 or 24.04, public IPv4, sudo |
| Zone | Cloudflare-managed test zone; you can delete all wizard-created resources |
| Redgres | Built from baseline commit; `REDGRES_ENVIRONMENT=production` paths under `/var/lib/redgres` |
| Bootstrap | `REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989`; UFW allow `8989/tcp` from your IP only |
| Loopback app | `REDGRES_ADDRESS=127.0.0.1:8790`, `curl http://127.0.0.1:8790/api/v1/healthz` → 200 |

### Secrets layout (production paths)

```bash
install -d -m 0700 -o root -g root /var/lib/redgres/secrets
# Files created by wizard/API only — never paste into Git
# cloudflare-api-token, cloudflared-tunnel-token, cloudflare-oauth-*.json, certbot-dns.ini
```

### API token permissions (Cloudflare dashboard)

UI checklist uses legacy labels; live test **records actual dashboard names**:

- Account · **Cloudflare Tunnel · Edit** (dashboard may say **Cloudflare One Connectors**)
- Account · **Access: Apps and Policies · Edit**
- Zone · **Zone · Read**
- Zone · **DNS · Edit**

---

## Execution order (required)

Run gates in this order on the **Cloudflare API token path**. OAuth callback uses `https://console.<zone>/api/v1/domain/oauth/callback` — **cloudflared (G3) must work before OAuth connect (G4)**.

| Step | Gate | Notes |
|---|---|---|
| 1 | G1 | Apply + Access allow; **keep** deployment |
| 2 | G2 | Scope pin only (documentation) |
| 3 | G3 | cloudflared + console through Access |
| 4 | G4 | OAuth connect (after G3) |
| 5 | G5 | certbot DNS-01 (Cloudflare path only) |
| 6 | G6 | confirm-reachable + UFW |
| 7 | G7 | Playwright (agents/CI) |
| 8 | G8 | Security sign-off |
| 9 | G9 | Disconnect teardown + TRACEABILITY Complete |

Full **G1 disconnect proof** is step 9, not during the journey.

---

## Gate G1 — Cloudflare apply + Access allow

**Proves:** DiscoverZone → tunnel → ingress → DNS → Access app → allow policy.

### Steps (Cloudflare API token path)

1. Sign in on bootstrap URL `http://<server-ip>:8989` (UFW-restricted).
2. Domain & Network → Cloudflare API → paste token → Apply with:
   - `zone`: test zone
   - `origin_ip`: server public IPv4 (or IPv6 for AAAA-only test — run twice if testing both)
   - `hostnames`: `console.<zone>`, `db.<zone>`, `redis.<zone>`
3. Add Access allow email → **Add Access allow policy**.
4. **Do not** confirm-reachable yet. **Do not** disconnect yet.
5. Cloudflare dashboard: confirm tunnel, proxied console CNAME, grey-cloud db/redis **A or AAAA** (one type per origin IP), Access app (deny default + allow policy).

### Pass criteria

- Apply returns 200, `bootstrap_still_open: true`, `access: deny_by_default` then allow.
- GET `/domain` → `configured: true`, `access: allow`.

### Evidence

- `G1-apply-response-redacted.txt`
- `G1-dashboard-records-redacted.png` (optional)

### On failure

Report HTTP status + `request_id` + Cloudflare error code (never token). Agent loop fixes code; re-run G1.

---

## Gate G2 — OAuth scope pin (documentation)

**Proves:** Code scope strings still match Cloudflare’s OAuth scope catalog.

### Steps

1. Open Cloudflare OAuth app creation UI and/or [OAuth scopes documentation](https://developers.cloudflare.com/fundamentals/api/how-to/create-via-api/#fetch-oauth-scopes).
2. Compare each string in `internal/cloudflare/oauth.go` (`RequiredOAuthScopes`) to the dashboard picker labels.
3. Record mapping in evidence (code scope → dashboard label).

Expected scopes:

- `zone.read`, `dns.write`, `ssl-and-certificates.write`, `user-details.read`, `offline_access`
- `access:apps_and_policies:edit`, `cloudflare_one.connectors:edit`

Do **not** use `/user/tokens/verify` for this step — that endpoint is for API tokens, not OAuth scope pinning.

### Pass criteria

- Every required scope is selectable on a new OAuth app; mismatches filed as code/doc bugs before G4.

### Evidence

- `G2-scopes-mapping.txt`

---

## Gate G3 — cloudflared + console through Access

**Proves:** Public hostname reaches loopback Redgres through tunnel + Access (not API reflection alone). **Required before G4** (OAuth redirect URI is the tunneled console hostname).

Follow [CLOUDFLARED.md](../CLOUDFLARED.md):

1. Install `cloudflared` package; enable `cloudflared-redgres.service` + path unit.
2. Token file populated from apply (`REDGRES_TUNNEL_TOKEN_FILE`).
3. `systemctl status cloudflared-redgres` active.
4. Browser: open `https://console.<zone>` → Cloudflare Access → Redgres login.
5. Optional on server: `curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8790/api/v1/healthz` (loopback only).

### Pass criteria

- Browser reaches Redgres UI after Access allow.
- Tunnel connector healthy in Cloudflare Zero Trust dashboard.

### Evidence

- `G3-cloudflared-status.txt`
- `G3-console-login-redacted.png` (no cookies/tokens visible)

---

## Gate G4 — OAuth connect (live)

**Proves:** authorize → callback → token exchange → steady-state bearer; API token removed.

**Prerequisite:** G3 passed (console hostname reachable through tunnel + Access).

### Steps

1. Open wizard from `https://console.<zone>` (not bootstrap `:8989`) if bootstrap still open.
2. Create Cloudflare OAuth app; redirect URI: `https://console.<zone>/api/v1/domain/oauth/callback`
3. UI step 4 → paste client ID/secret → **Connect Cloudflare** (or API `oauth-client`, `oauth/start`, browser authorize, callback).
4. Verify: API token file removed; OAuth token file exists mode 0600; GET `/domain` → `credential: oauth`.
5. Optional negative: OAuth pending row expires after 10m (stale callback → 400).

### Pass criteria

- Callback 302 to wizard; no secrets in response/audit.

### Evidence

- `G4-oauth-connect-redacted.txt` (status codes only)

---

## Gate G5 — certbot DNS-01 (real, Cloudflare path only)

**Proves:** `POST /api/v1/domain/tls/issue` invokes real certbot for db + redis hostnames.

**Not available** for `dns_provider: manual` (API returns `409`).

### Prerequisites

- G4 complete (OAuth steady-state recommended).
- `REDGRES_CERTBOT_BIN=certbot` (or full path)
- `REDGRES_CERTBOT_DNS_TOKEN_FILE` → certbot-dns-cloudflare ini (0600, not in Git)
- Grey-cloud db/redis DNS resolves to origin

### Steps

1. UI **Issue TLS certificates** or `POST /api/v1/domain/tls/issue`.
2. On host: `sudo certbot certificates` — certs for `db.<zone>` and `redis.<zone>`.

### Pass criteria

- API 200, `tls.db` / `tls.redis` → `issued`.
- Valid Let's Encrypt chain (not fake/test PEM).

### Evidence

- `G5-certbot-certificates-redacted.txt` (names + expiry only)

---

## Gate G6 — UFW bootstrap rule removal

**Proves:** `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD` runs on confirm-reachable.

### Setup

- Bootstrap listener open; UFW rule allowing `8989/tcp` from operator IP.
- Set `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD` to **absolute path** helper script (no shell).

### Steps

1. Complete G3 (console reachable through Access) — **human verification** per [ADR-013](../decisions/ADR-013-confirm-reachable-attestation.md).
2. UI → **Console is reachable — close bootstrap** (hostname echo).
3. API response: `bootstrap_closed: true`, `bootstrap_ufw_attempted: true`, `bootstrap_ufw_removed: true` (when helper succeeds).
4. `sudo ufw status` — no `8989` rule.
5. `:8989` refused from internet; loopback app still on 8790.

### Evidence

- `G6-confirm-response-redacted.txt`
- `G6-ufw-status-after.txt`

---

## Gate G7 — Playwright wizard e2e

**Owner:** code slice — agents implement; you run:

```bash
cd web && npm run test:e2e
```

Target flows: Cloudflare path + manual DNS toggle (see `DomainNetworkPage.test.tsx` for component coverage).

### Pass criteria

- E2E green on baseline commit (link CI run URL in evidence).

---

## Gate G8 — Independent security sign-off

1. Freeze immutable commit SHA after code fixes merged.
2. Run **security-review** on that SHA only.
3. Record in [ACCEPTANCE_CHECKLIST.md](../ACCEPTANCE_CHECKLIST.md) Security section.

### Evidence

- `G8-security-review.md`

---

## Gate G9 — Disconnect teardown + OPS-009 Complete

**Proves:** API disconnect deletes Cloudflare resources created by apply.

1. UI → **Disconnect domain** (hostname confirmation) or `DELETE /api/v1/domain`.
2. Dashboard: tunnel, DNS records, Access app/policy **gone**.
3. Stop/disable cloudflared; revoke OAuth in dashboard; remove secret files (see Teardown).
4. When G1–G8 evidence exists, append [TRACEABILITY.md](../TRACEABILITY.md) block marking OPS-009 Complete (or stay Partial with explicit remaining deferrals).

### Evidence

- `G9-teardown-empty-zone-redacted.txt`

---

## Manual DNS path (alternate to G1 apply)

1. Apply with `dns_provider: manual` (UI toggle or API).
2. Follow returned `instructions`; create records + Access manually in dashboard.
3. **Verify public DNS** → **Access configured manually** → G3 → G6 (skip **G4 OAuth** if no Cloudflare API path needed; skip **G5 certbot** — TLS issue returns `409` for manual mode).
4. Teardown: `DELETE /domain` clears SQLite state only; **manually delete** DNS/tunnel/Access records you created outside Redgres.

Evidence: `G1-manual-instructions.txt`, `G1-manual-verify-results.txt`.

---

## Teardown (always)

After acceptance or abort:

1. Run **G9** disconnect if Cloudflare apply path was used.
2. Revoke OAuth tokens in Cloudflare dashboard.
3. Delete OAuth/API token files on host.
4. Stop/disable cloudflared unit.
5. Remove test certs if desired: `certbot delete` (operator choice).
6. Destroy VM or snapshot for audit.

---

## Agent loop when a gate fails

1. Operator captures **request_id**, HTTP status, redacted body, Cloudflare error code.
2. Parent agent: `redgres-planner` → bounded fix packet → `go test` / vitest → **ruthless-reviewer**.
3. No production/DNS changes by agents.
4. Operator re-runs failed gate only.
5. Repeat until G1–G8 pass; then G9 + TRACEABILITY.

Multi-agent roles: [AGENTS.md](../../AGENTS.md) and `.cursor/rules/50-multi-agent-orchestration.mdc`.
