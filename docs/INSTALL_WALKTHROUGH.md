# Redgres install & domain walkthrough

> **Status: APPROVED OPERATOR JOURNEY.** This is the operator-facing install + domain journey. Its decisions are folded into canonical docs: [ADR-012](decisions/ADR-012-ui-bootstrap.md), PRD OPS-008/OPS-009, and updated [DOMAIN_AND_NETWORK.md](DOMAIN_AND_NETWORK.md) / [INSTALLER_SPEC.md](INSTALLER_SPEC.md); it is linked from [INDEX.md](INDEX.md) and recorded in [TRACEABILITY.md](TRACEABILITY.md).
>
> **Target vs current:** everything below describes the **target** installer. Today `deploy/install.sh` is a validated **dry-run** shell (trust bootstrap, config parser, inventory, skip matrices). The live install — packages, services, TLS, Cloudflare, firewall — is specified but **not implemented**. This is a plan you are reviewing, not a working installer.

## 1. The goal in one sentence

One public GitHub repo with a single `sh` installer. Clone it on a fresh Ubuntu server, run one command, answer a short Q&A (PostgreSQL version → PgBouncer yes/no → extensions select/skip → Redis version), and end with every endpoint documented and reachable — plus a clear, correctable set of steps to attach your domain and TLS.

## 2. Decisions locked for this draft

| Question | Decision |
|---|---|
| UI reachability | Redgres UI: **bootstrap `IP:8989` (source-restricted, auto-closes after domain)** → then loopback + Tunnel + Access. pgAdmin/RedisInsight: loopback + tunnel only, never public. |
| Domain/SSL driving | Runtime first-run **Domain & Network wizard** in the UI + manual-DNS fallback; **Cloudflare OAuth (self-created app, minimal scopes; per-zone API token as fallback)**. |
| pgAdmin | **Out of scope** for now (manual/expert tool). A domain is *reserved* but not provisioned. |
| apt behavior | Install only **exact pinned packages**. No full-system `apt upgrade`, no unrelated `apt remove`. |
| Deliverable | This walkthrough for review; canonical specs updated only after you correct it. |

## 3. Prerequisites

- Ubuntu Server 24.04 LTS, one server, at least 4 vCPU / 8 GB RAM, SSD/NVMe (starting guidance — see [DEPLOYMENT.md](DEPLOYMENT.md) §8).
- A domain whose DNS you control (Cloudflare in the documented profile).
- Docker Engine + Compose plugin (for Redis; installed or verified by the installer).
- You can `sudo` on the server and reach it over SSH.

## 4. Step 0 — get the installer & choose the Redgres release

There are two separate things being installed, from two different sources:

1. **The installer itself** — you clone the public repo and run its entry point.
2. **The Redgres app** — a pinned release artifact (tarball + `SHA256SUMS`), downloaded and verified during install. It is **not** the cloned `main` checkout.

```bash
git clone <your-public-repo-url> redgres && cd redgres
sudo ./deploy/install.sh --mode fresh-postgres ...
```

The installer is **not** `curl | bash`. Never run a downloaded-and-piped unverified script; the repo ships version-controlled modules and `install.sh` only dispatches to them.

### Choosing the Redgres app version

The installer resolves the Redgres release the same safe way it resolves PostgreSQL/Redis versions — exact, pinned, checksum-verified, never a floating `latest`.

| Input | Behavior |
|---|---|
| no version flag (default) | resolve **latest stable, tested** release from reviewed release metadata → exact version + digest → download → verify → install |
| `--release VERSION` | install that exact version |
| interactive | list available tested releases, default to latest, let you pick |

"Latest" means: resolve the latest **stable, non-prerelease** release to a **concrete version + checksum before any mutation**. It is a convenience default, never an upstream floating tag and never an unreviewed prerelease. Every release is immutable and carries `SHA256SUMS` (optionally signature/provenance); the installer verifies the checksum before installing and records the exact version/digest in the install report.

## 5. Step 1 — run the installer (interactive Q&A)

Run:

```bash
sudo ./deploy/install.sh
```

Interactive setup presents **only** versions the current Redgres release supports, recommended choices first. The flow you described maps like this:

| Your question | What the installer asks | Choices | Default |
|---|---|---|---|
| "PostgreSQL — which version?" | PostgreSQL major | 17, 18 | 18 (fresh) |
| "PgBouncer — yes/no?" | PgBouncer mode | `fresh` / `existing` / `skip` (no) | — |
| "Extensions — which / necessary / skip?" | Extension policy + plan | `preserve` (default) or `apply-selected` from a validated plan | `preserve`, empty optional set |
| "Redis — which version?" | Redis series (Docker) | 8.2, 8.8 | 8.2 (production) |

Rules you should know (so you can correct me):

- **PostgreSQL and PgBouncer are host-native.** **Redis is Docker.** This is a deliberate hybrid (ADR-002), not "everything in Docker."
- **PgBouncer is a separate lifecycle**, exactly your "yes/no" — skip means no pooled `6432` endpoint, and Redgres then uses direct `5432` for administration.
- **Extensions are opt-in.** The default is `preserve` (inventory and leave alone) with an empty optional set. Choosing extensions means a non-secret, machine-validated plan (`policy: apply-selected`, explicit databases, optional scheduler); nothing is enabled in `template1`, and a required restart needs explicit `--approve-postgres-restart`.
- **Versions resolve to exact pins**, never `latest`. Supported today: PostgreSQL 17/18, Redis 8.2/8.8, a release-pinned PgBouncer.

Non-interactive equivalents exist for every choice (see [INSTALLER_SPEC.md](INSTALLER_SPEC.md) "Command interface"):

```bash
sudo ./deploy/install.sh --mode fresh-postgres --postgres-version 18 \
  --pgbouncer-mode fresh \
  --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart \
  --redis-mode fresh --redis-version 8.2 --config /root/redgres/install.env
```

## 6. Step 2 — what the installer reports

At the end you get a **redacted report** (no credentials) listing:

- Selected and detected versions, exact package/image versions, image digests, cluster identity.
- The bootstrap UI URL, loopback bindings, and the public DNS-only ports.
- What changed, what was skipped, and exact follow-ups.

### UI origins

| Service | Bind | How you reach it |
|---|---|---|
| Redgres UI (canonical) | `127.0.0.1:8790` | Cloudflare Tunnel + Access |
| Redgres UI (bootstrap, self-closing) | `0.0.0.0:8989` (source-restricted) | `https://<VPS_IP>:8989` until domain is connected, then auto-closed |
| pgAdmin (optional) | loopback port | Tunnel + Access only, never public |
| RedisInsight (optional) | `127.0.0.1:5540` | Tunnel + Access only, never public |

### Public DNS-only endpoints (raw clients — TLS required)

| Endpoint | Port | Protocol |
|---|---|---|
| PostgreSQL direct | 5432 | TLS + SCRAM |
| PgBouncer pooled | 6432 | TLS + SCRAM |
| Redis | 6380 | TLS + Redis ACL |

### Firewall (UFW)

UFW is default-deny. Public allows are SSH, raw database ports, and the **temporary** bootstrap UI port:

| Direction | Ports | Notes |
|---|---|---|
| Public allow | 22, 5432, 6432, 6380 | SSH, PostgreSQL direct, PgBouncer, Redis TLS |
| Temporary bootstrap | 8989 | Redgres UI only, source-restricted to the operator IP; auto-removed after domain + tunnel verified |
| Never opened (loopback) | 8790, pgAdmin, 5540 | Canonical UI origins bind `127.0.0.1`; pgAdmin/RedisInsight reachable only via tunnel |
| Not opened (no listener) | 80, 443 | Browser HTTPS ends at Cloudflare's edge; the server has no inbound TLS listener for the UIs |

The tunnel dials **out** to Cloudflare, so the UIs need **no public port** — opening them would be a mistake, not a requirement. `443` on *your server* stays closed: the `https://` you type in the browser is served by Cloudflare, not by the server.

**Why the two categories differ:** after bootstrap closes, browser consoles (Redgres, pgAdmin, RedisInsight) ride the tunnel — no permanent public UI port. Raw database endpoints (`5432` / `6432` / `6380`) **cannot** ride the tunnel: ordinary `psql` / `redis-cli` / app drivers don't speak the tunnel protocol, so they stay public DNS-only with end-to-end TLS (SCRAM/ACL), source-restricted to the application's egress IPs where those are stable.

The report always includes the **bootstrap UI URL** (`https://<VPS_IP>:8989`, source-restricted) plus loopback/canonical bindings and the public DB ports. The only temporary public UI port is that bootstrap listener; it is removed once Tunnel + Access are verified.

## 7. Step 3 — open the Redgres UI

Right after install, the report gives you a **bootstrap URL**:

```text
https://<VPS_IP>:8989
```

That bootstrap listener is **source-restricted to your current IP** and closes itself once the domain is connected. Log in with the owner password, and the UI pushes you into the **Domain & Network wizard** (Step 4).

After the domain + tunnel + Access are verified live, Redgres **auto-rebinds to `127.0.0.1:8790` and removes the 8989 firewall rule** — the public port is gone. From then on you reach it at `https://console.example.com`.

pgAdmin and RedisInsight are **never** reachable by IP:port; they are loopback + tunnel only.

### How it closes — and how to get back in

- **Closing** is two actions: remove the `8989` UFW rule **and** rebind Redgres from `0.0.0.0:8989` back to `127.0.0.1:8790`. After that, nothing on the server answers 8989 for anyone.
- **Getting back in** (tunnel/Access broken): SSH into the server and re-run the bootstrap command, which re-opens `8989` **to your current IP only**, or use the SSH tunnel as the emergency path. You never need a permanently open UI port.
- **Why source-restrict matters:** your server's IP is scanned continuously by internet bots (Shodan/masscan) regardless of whether anyone "knows" it. An open `0.0.0.0:8989` is discovered in minutes. Source-restricting to your IP makes "nobody else can reach it" literally true — not a guess.

> **Security-model change ([ADR-012](decisions/ADR-012-ui-bootstrap.md)):** this bootstrap exposure is a deliberate, self-closing exception to the "loopback only" invariant — source-restricted to the operator IP and auto-closed once the domain is live. pgAdmin/RedisInsight remain loopback + tunnel only.

## 8. Step 4 — connect your domain (Domain & Network wizard)

Domain wiring happens **in the Redgres UI**, not on the `sh` command line: open the UI (Step 3), go to **Settings → Domain & Network**, and the server applies your domain. This replaces the earlier "installer + instructions" idea.

In the wizard, in order:

1. Enter the hostnames you want (`console`, `db`, `redis`; optional `pgadmin`, `redis-insight`).
2. Click **Connect Cloudflare** and approve the minimal scopes once.
3. Redgres (server-side) creates the tunnel, the DNS records, the TLS certs, and the Access policy, then verifies them live.
4. Bootstrap `8989` auto-closes; you now reach the UI at `https://console.example.com`.

### Credential model: Cloudflare OAuth (self-created app, minimal scopes)

- You create the OAuth app yourself and grant **only the scopes Redgres needs**: `zone.read`, `dns.write`, `ssl-and-certificates.write`, `user-details.read`, `offline_access`, **Access: Apps and Policies** (Edit), and the **tunnel-management** scope (`Cloudflare One Connectors` / `Argo Tunnel (Legacy)`). **No** billing, `zone.write`, `zone-settings.write`, `zone-dns-settings.read`, or the rest of the Zero Trust surface. The wizard's **Connect Cloudflare** button opens the authorization flow; you approve once.
- **Tunnel nuance:** the OAuth token *creates and manages* the tunnel and its DNS routes via the API; the `cloudflared` daemon on the server still connects with its own **tunnel token**, which Redgres also stores server-side only (never in the browser).
- The issued OAuth tokens are stored **server-side only** (`/etc/redgres/secrets`, `0600`, systemd credential). They are **never persisted in browser storage** and **never returned** by the API.
- They are **not** stored in the control-state SQLite. Encryption-at-rest on the same host (with the key beside it) only guards a stolen backup of the DB file — it does not protect against a live server compromise.
- **Fallback without OAuth:** a per-zone Cloudflare API token (`zone.read`, `dns.write`, `ssl-and-certificates.write`, tunnel edit) works the same way server-side. OAuth is the default for the "connect account" flow; the token is the manual alternative.

> **Tradeoff (be aware):** Cloudflare OAuth scopes are **account-level permissions** — even a minimal app applies them to **every zone** in the account, not just the one domain. A Cloudflare [API token](https://developers.cloudflare.com/flagship/api-tokens/) can instead be scoped to a **single zone**, which OAuth cannot. For a single-domain account the difference is negligible and OAuth is fine; for a multi-domain account, OAuth is broader than a per-zone token even though Redgres only *acts* on the zone you point it at.

### What the server does with it

1. Resolves the zone from the explicit hostnames you entered (deterministic — it never "guesses" your zone or account).
2. Creates/updates **only the records it declares**:
   - `console`, `pgadmin`, `redis-insight` → proxied (orange-cloud) + tunnel route.
   - `db`, `redis` → DNS-only (grey-cloud).
3. Provisions TLS:
   - UI hostnames → Cloudflare edge (tunnel terminates HTTPS) + Access — Cloudflare's free **Universal SSL** covers these proxied hostnames.
   - `db` / `redis` → Let's Encrypt DNS-01 (publicly trusted), atomic copy + reload hook + live TLS verify. The DNS-01 challenge and auto-renewal reuse the same `dns.write` permission, so renewal is hands-free.
4. Reports exactly what it changed; it never deletes unrelated records or touches other zones.

> **Cloudflare vs Let's Encrypt:** they solve different layers. Cloudflare's free **Universal SSL** terminates HTTPS at the edge for **proxied** hostnames (the UIs). The raw DB ports are **grey-cloud/DNS-only**, so clients connect straight to your server and need a **publicly-trusted** cert — that's **Let's Encrypt (ACME DNS-01)** via Certbot (or equivalent), renewed automatically (~90-day certs). Cloudflare's **Origin Certificates** are only trusted by Cloudflare and do **not** work for direct `psql`/`redis-cli` connections. There is no separate “Cloudflare ACME” product for grey-cloud DB ports — Cloudflare is only the DNS API for the Let's Encrypt challenge (`dns.write`).

### SSL mode: Full vs Full (strict)

These names mean different things in different places. Lock them like this:

| Surface | Setting | Why |
|---|---|---|
| Cloudflare dashboard → SSL/TLS for **proxied UI** hostnames (`console`, optional `pgadmin` / `redis-insight`) | **Full (strict)** | Browser ↔ Cloudflare is always HTTPS (Universal SSL). Set the zone to **Full (strict)** as a safe default so any *non-tunnel* proxied hostname also requires a valid origin cert. The tunnel hop is already encrypted, so the loopback UI origin stays plain HTTP. **Not Flexible** (that would allow cleartext to origin). |
| Redgres Tunnel origin | `http://127.0.0.1:8790` (loopback) | The hop Cloudflare → app rides inside the **encrypted tunnel**; the VPS does not need a public `:443` or a Let's Encrypt cert for the UI. |
| PostgreSQL / PgBouncer / Redis (`db`, `redis`, DNS-only) | Cloudflare Full / Full (strict) **does not apply** | Grey-cloud traffic never goes through Cloudflare's HTTP proxy. Clients use **Let's Encrypt** on the server and connect with **`sslmode=verify-full`** (Postgres) / TLS hostname verification (Redis). That is the DB equivalent of “strict.” |

**Do not use:**

- **Flexible** — encrypts only the browser hop; wrong for any admin UI.
- **Full** (without strict) as the long-term target when an HTTPS origin cert exists — it encrypts but skips proper origin cert validation.
- Cloudflare **Origin CA** certs on `db` / `redis` — apps and `psql` will not trust them for `verify-full`.

**Short rule:** UI = Cloudflare Universal SSL + Tunnel + zone **Full (strict)**; databases = **Let's Encrypt** + client **verify-full**.

### Non-Cloudflare domains

The app must not hard-code Cloudflare as the only provider. For any other DNS, the wizard shows the exact records to create manually (DNS-only for raw ports, tunnel/CNAME for UIs) and verifies them once you confirm they exist.

### Record map

| Hostname | Mode | Destination |
|---|---|---|
| `console.example.com` | Proxied (Tunnel) | Redgres UI loopback |
| `pgadmin.example.com` | Proxied (Tunnel) | optional pgAdmin (not provisioned now) |
| `redis-insight.example.com` | Proxied (Tunnel) | optional RedisInsight (not provisioned now) |
| `db.example.com` | DNS-only | 5432 (direct) and 6432 (pooled) |
| `redis.example.com` | DNS-only | 6380 (TLS + ACL) |

### Example client URLs

**PostgreSQL (Prisma / any driver):**

```text
DATABASE_URL="postgresql://<user>:<password>@db.example.com:6432/<database>?sslmode=verify-full&pgbouncer=true"
DIRECT_URL="postgresql://<user>:<password>@db.example.com:5432/<database>?sslmode=verify-full"
```

- `DATABASE_URL` (pooled `6432`) is runtime traffic; `DIRECT_URL` (`5432`) is for migrations/schema changes that need a direct connection.
- Prefer `sslmode=verify-full` (verifies the hostname against a trusted CA) over `require` (encrypts but does not verify identity).
- With PgBouncer in transaction mode, add `pgbouncer=true` so Prisma disables prepared statements.

**Redis (any Redis client):**

```text
rediss://<acl_user>:<password>@redis.example.com:6380/0
```

- `rediss://` is TLS; `redis://` is plaintext and is **not** exposed publicly. No `default`/anonymous access.
- Username is the **Redis ACL user** Redgres created; isolation is by the ACL **key prefix** (e.g. `project:*`), not the `/0` DB number.
- Works with `redis-py`, `node-redis`, `ioredis`, `go-redis`, `StackExchange.Redis`, etc.

- Both URL types are returned **one-time, `no-store`**; never log or share the real password — rotate any credential that leaks.

See [DOMAIN_AND_NETWORK.md](DOMAIN_AND_NETWORK.md) for the canonical table and traffic rules.

## 9. Correct-me checklist

Mark each line right or wrong so I can fix the draft:

1. The installer is `git clone` + `sudo ./deploy/install.sh`, never `curl | bash`.
2. PostgreSQL and PgBouncer are host-native; **only Redis runs in Docker**.
3. The Q&A order is: PostgreSQL version → PgBouncer yes/no → extensions select/skip → Redis version.
4. The Redgres UI is reached via a **bootstrap `IP:8989` (source-restricted, auto-closes)** → then Cloudflare Tunnel + Access; pgAdmin/RedisInsight are never public.
5. UI HTTPS = Cloudflare Universal SSL + Tunnel; zone SSL/TLS mode **Full (strict)** (never Flexible). **Let's Encrypt/Certbot is only for raw DB ports**; DB clients use **`sslmode=verify-full`**.
6. pgAdmin is out of scope for now.
7. apt installs only pinned packages; no full-system upgrade/remove.
8. Domain/SSL is a first-run **Domain & Network wizard** in the UI (server-side **Cloudflare OAuth**, minimal scopes; optional per-zone API token fallback); manual-DNS fallback for non-Cloudflare.
9. Supported versions today: PostgreSQL 17/18, Redis 8.2/8.8, release-pinned PgBouncer.
10. The live installer is **not implemented yet** — this is target documentation.
11. The Redgres app is installed from a **verified release artifact** (tarball + `SHA256SUMS`), not the cloned `main` checkout.
12. No `--release`/`-v` → installer auto-uses the **latest stable, tested** release, resolved to an exact version + checksum before installing; or you select a version interactively. Never a floating `latest`, never a prerelease.
13. UFW: **22, 5432, 6432, 6380** public; **8989** temporary bootstrap (source-restricted, auto-closes); **8790/pgAdmin/5540** loopback-only; **80/443** not opened (HTTPS ends at Cloudflare's edge).
14. Cloudflare credential is a **self-created OAuth app with minimal scopes**; tokens stored server-side only; never in browser storage; never in the control-state SQLite. (OAuth scopes are account-wide across all zones, unlike a per-zone API token.)

## 10. References

- [INSTALLER_SPEC.md](INSTALLER_SPEC.md) — command interface, stages, protections.
- [DEPLOYMENT.md](DEPLOYMENT.md) — services, filesystem, release, firewall.
- [DOMAIN_AND_NETWORK.md](DOMAIN_AND_NETWORK.md) — hostnames, traffic rules, TLS ownership.
- [COMPATIBILITY.md](COMPATIBILITY.md) — supported versions, defaults, detection.
- [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md) — extension lifecycle.
