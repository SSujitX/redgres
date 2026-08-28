<div align="center">

<img src="web/public/assets/redgres-logo-light.png" alt="Redgres — self-hosted PostgreSQL and Redis control plane" width="280">

<h1>Redgres</h1>

<p><strong>Self-hosted PostgreSQL + Redis control plane</strong> on your Ubuntu VPS.<br>
Raw package install — not a hosted DBaaS, not an ORM.</p>

<p>
  <a href="https://github.com/SSujitX/redgres/actions/workflows/install.yml"><img src="https://img.shields.io/github/actions/workflow/status/SSujitX/redgres/install.yml?branch=master&style=for-the-badge&label=CI" alt="Installer CI status"></a>
  <a href="https://github.com/SSujitX/redgres/releases"><img src="https://img.shields.io/github/v/release/SSujitX/redgres?style=for-the-badge&logo=github" alt="Latest GitHub release"></a>
  <a href="https://github.com/SSujitX/redgres/stargazers"><img src="https://img.shields.io/github/stars/SSujitX/redgres?style=for-the-badge&logo=github" alt="GitHub stars"></a>
  <img src="https://img.shields.io/badge/Status-Development-f59e0b?style=for-the-badge" alt="Project status: development">
  <img src="https://img.shields.io/badge/Deploy-Self--hosted-2563eb?style=for-the-badge" alt="Self-hosted deployment">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue?style=for-the-badge&logo=apache&logoColor=white" alt="Apache 2.0 license"></a>
  <a href="https://redgres.com/sponsor/"><img src="https://img.shields.io/badge/Sponsor-looking-db61a2?style=for-the-badge&logo=githubsponsors&logoColor=white" alt="Looking for sponsorship"></a>
</p>

<p>
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.27">
  <img src="https://img.shields.io/badge/React-19.2-20232A?style=for-the-badge&logo=react&logoColor=61DAFB" alt="React 19.2">
  <img src="https://img.shields.io/badge/TypeScript-7.0-3178C6?style=for-the-badge&logo=typescript&logoColor=white" alt="TypeScript 7.0">
  <img src="https://img.shields.io/badge/PostgreSQL-17%20%7C%2018-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL 17 and 18">
  <img src="https://img.shields.io/badge/Redis-8.2%20%7C%208.8-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis 8.2 and 8.8">
  <img src="https://img.shields.io/badge/Ubuntu-24.04%20%7C%2026.04-E95420?style=for-the-badge&logo=ubuntu&logoColor=white" alt="Ubuntu 24.04 and 26.04">
</p>

</div>

Redgres is a self-hosted Go and React administration console for **one PostgreSQL cluster** and **one Redis instance** on a server you control. A raw installation uses Ubuntu packages (`postgresql-17` or `postgresql-18`), Docker Redis, optional PgBouncer, and the Redgres systemd unit — then you connect with `psql`, Prisma, Drizzle, or any PostgreSQL driver.

**Site:** [redgres.com](https://redgres.com/) · [Raw install](https://redgres.com/install/) · [vs Supabase / Neon / Prisma](https://redgres.com/compare/)

**Looking for sponsorship:** [redgres.com/sponsor](https://redgres.com/sponsor/)

> [!WARNING]
> Not production-accepted. Do not retire an existing console until [docs/TRACEABILITY.md](docs/TRACEABILITY.md) and [docs/ACCEPTANCE_CHECKLIST.md](docs/ACCEPTANCE_CHECKLIST.md) are complete.

## Not Supabase, Neon, Prisma, or Drizzle

Redgres installs and administers **raw PostgreSQL and Redis on your VPS**. It is not a hosted backend and not a query library.

| | Redgres | Supabase | Neon | Prisma / Drizzle |
|---|---|---|---|---|
| Where it runs | Your Ubuntu 24.04/26.04 VPS | Their cloud (or their stack) | Their cloud | Inside your app |
| What you get | One Postgres + one Redis + operator console | Postgres + Auth + Storage + API | Serverless Postgres | ORM / query builder |
| Install | `git clone` + `sudo ./deploy/install.sh` | Platform project | Platform project | `npm` package |
| Client apps | Any: `psql`, pgx, Prisma, Drizzle, `node-postgres` | Their SDKs + Postgres | Their driver + Postgres | Talks to *your* Postgres |
| Use Redgres when | You want a single-server control plane and credentials you hold | You want a hosted BaaS | You want serverless scale-to-zero | You want typed SQL in application code |

Pick **one** install path. Full stack already installs the Redgres app — do not also run `curl …/install.sh`.

---

# Full stack (PostgreSQL + Redis + app)

Ubuntu 24.04 or 26.04. Clone anywhere you can write; the commands below use your home directory and create `~/redgres`. The running binary still lands in `/opt/redgres`.

## Go home

```bash
cd ~
```

## Clone

Creates `~/redgres` (the repo). Stay in that folder for every `deploy/install.sh` command below.

```bash
git clone https://github.com/SSujitX/redgres.git
```

## Enter the repo

```bash
cd ~/redgres
```

PostgreSQL **17 or 18**. Redis **8.2 or 8.8**. PgBouncer **disabled or fresh**. No `latest`. Live install is a **new** cluster + **new** Redis only.

## Ask on the terminal

Defaults: PostgreSQL 18, Redis 8.2, PgBouncer off.

```bash
sudo ./deploy/install.sh
```

## Preview (no host changes)

Add `--dry-run` to any live command. Swap `17`/`18`, `8.2`/`8.8`, `disabled`/`fresh`.

```bash
sudo ./deploy/install.sh --non-interactive --dry-run --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode disabled
```

## PostgreSQL 18 + Redis 8.2

```bash
sudo ./deploy/install.sh --non-interactive --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode disabled
```

## PostgreSQL 17 + Redis 8.2

```bash
sudo ./deploy/install.sh --non-interactive --mode fresh-postgres --postgres-version 17 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode disabled
```

## PostgreSQL 18 + Redis 8.8

```bash
sudo ./deploy/install.sh --non-interactive --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.8 --pgbouncer-mode disabled
```

## PostgreSQL 17 + Redis 8.8

```bash
sudo ./deploy/install.sh --non-interactive --mode fresh-postgres --postgres-version 17 --redis-mode fresh --redis-version 8.8 --pgbouncer-mode disabled
```

## PostgreSQL 18 + Redis 8.2 + PgBouncer

```bash
sudo ./deploy/install.sh --non-interactive --mode fresh-postgres --postgres-version 18 --redis-mode fresh --redis-version 8.2 --pgbouncer-mode fresh
```

Password prints once on your TTY. Open the printed `:8989` URL.

## Later: upgrade the app only

Databases stay. Run from any directory.

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/upgrade.sh | sudo bash
```

---

# App only (you already have PostgreSQL and Redis)

No clone. Does not install databases.

## Latest Redgres

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash
```

## Pin Redgres `v=1.0.0`

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash -s -- v=1.0.0
```

## Latest, no prompts

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash -s -- --non-interactive
```

## Upgrade app

Keeps `/etc/redgres` and `/var/lib/redgres`.

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/upgrade.sh | sudo bash
```

## Dev snapshot (not a release)

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install-dev.sh | sudo bash
```

---

# Debug, logs, and fixes (on the server)

SSH to the host. Do not paste `/etc/redgres/redgres.env`, password files, or connection URLs into tickets or chat.

| Path | What it is |
|---|---|
| `/opt/redgres/current/redgres` | Running binary |
| `/etc/redgres/redgres.env` | Non-secret app config (`root:redgres`) |
| `/var/lib/redgres/redgres.db` | Control-plane SQLite (not project credentials) |
| `/etc/redgres/redis-compose.yml` | Redis Compose file |
| `redgres.service` | systemd unit (`User=redgres`) |

## Is it running?

```bash
systemctl status redgres.service --no-pager
/opt/redgres/current/redgres version
ss -lntp | grep -E '8790|8989|5432|6380|6432'
```

First-run console is the printed `:8989` URL. The app origin stays loopback `127.0.0.1:8790`. After Domain & Network confirm-reachable (or bootstrap TTL), `:8989` closes.

## Logs

`journald` is the log. There is no required `/var/log/redgres` file.

```bash
journalctl -u redgres.service -n 200 --no-pager
journalctl -u redgres.service -f
journalctl -u redgres.service --since "1 hour ago" --no-pager
```

Turn on debug (never logs passwords or request bodies), then restart:

```bash
sudo sed -i 's/^REDGRES_LOG_LEVEL=.*/REDGRES_LOG_LEVEL=debug/' /etc/redgres/redgres.env
sudo systemctl restart redgres.service
```

Set it back to `info` when finished.

## Health

```bash
curl -fsS http://127.0.0.1:8790/api/v1/healthz
```

Expect HTTP 200. If this fails, read `redgres.service` logs before changing the firewall.

## Restart / stop

```bash
sudo systemctl restart redgres.service
sudo systemctl stop redgres.service
sudo systemctl start redgres.service
```

## Cannot sign in (owner password)

Password prints once at install. Reset on a real TTY (prints the new password to the terminal only):

```bash
sudo /opt/redgres/current/redgres create-owner --generate --replace --username admin --sqlite-path /var/lib/redgres/redgres.db
```

`--replace` revokes existing owner sessions. There is no HTTP bootstrap route.

## PostgreSQL not up

Fresh install creates cluster `main` for the major you chose (`17` or `18`).

```bash
pg_lsclusters
sudo -u postgres pg_isready -h 127.0.0.1 -p 5432
sudo systemctl status 'postgresql@*-main' --no-pager
journalctl -u 'postgresql@18-main' -n 100 --no-pager
```

Use `17` instead of `18` when `pg_lsclusters` shows 17. Do not run `pg_dropcluster` or `initdb` to “fix” a failed start.

## Redis not up

Redis is the `redgres-redis` container, published on `127.0.0.1:6380`.

```bash
sudo docker compose -f /etc/redgres/redis-compose.yml ps
sudo docker logs redgres-redis --tail 100
sudo systemctl status docker --no-pager
```

Do not put the Redis password on a command line.

## PgBouncer (only if you installed `fresh`)

```bash
systemctl status pgbouncer --no-pager
journalctl -u pgbouncer -n 100 --no-pager
ss -lntp | grep 6432
```

## Listeners and firewall

```bash
sudo ufw status verbose
ss -lntp | grep -E '8790|8989|5432|6380|6432'
```

Bootstrap UFW is `allow from <your IP> to any port 8989` only — never a world-open `8989/tcp`. Inactive UFW does not enforce that rule.

## App failed after an upgrade — roll back the binary

From the cloned repo. This switches `/opt/redgres/current` only. It does not undo PostgreSQL, Redis, vault, or credentials.

```bash
cd ~/redgres
sudo ./deploy/install.sh rollback --to VERSION
```

`VERSION` is a path-safe release directory name under `/opt/redgres/releases/`.

```bash
ls /opt/redgres/releases
```

## Common failures

| Symptom | What to run |
|---|---|
| Unit `failed` / `activating` | `journalctl -u redgres.service -n 200 --no-pager` |
| `healthz` refused | `ss -lntp \| grep 8790` then the same journal |
| `:8989` closed | Expected after confirm-reachable or TTL; use the HTTPS console origin |
| Forgot owner password | `create-owner --generate --replace` above |
| Postgres `pg_isready` fails | `pg_lsclusters` + `journalctl -u 'postgresql@NN-main'` |
| Redis container exited | `docker logs redgres-redis --tail 100` |
| Need a newer app, keep data | `upgrade.sh` (app only) |

Full runbook: [docs/OPERATIONS.md](docs/OPERATIONS.md). Tunnel units: [docs/CLOUDFLARED.md](docs/CLOUDFLARED.md).

---

# Uninstall

## App only (keep databases)

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y --app-only
```

## Ask first

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash
```

## Everything (destroys PostgreSQL data on this host)

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y
```

---

# Looking for sponsorship

Redgres is Apache-2.0 and still finishing production gates. Sponsorship pays for test hosts, release work, and time — not influence over security defaults.

Write about funding, infrastructure, or partnership: [redgres.com/sponsor](https://redgres.com/sponsor/)

---

# FAQ

**Is Redgres a self-hosted Supabase or Neon alternative?**
No. Supabase and Neon are hosted (or platform) Postgres products. Redgres is an operator console plus a raw Ubuntu install of one PostgreSQL cluster and one Redis instance. You keep the data directory, roles, and network on your VPS.

**Do I need Prisma or Drizzle to use Redgres?**
No. Prisma and Drizzle are application ORMs. Redgres talks to PostgreSQL with parameterized `pgx` and never embeds an ORM. Apps you provision can still use Prisma, Drizzle, `psql`, or any driver against the project database URLs.

**What does “raw installation” mean?**
The full-stack path installs named Ubuntu/PGDG packages (`postgresql-17` or `postgresql-18`), Docker Redis, optional `pgbouncer`, and the `redgres.service` unit. It does not wrap Postgres in a proprietary cloud API.

**Which versions are in scope?**
PostgreSQL 17 or 18, Redis 8.2 or 8.8, Ubuntu 24.04 or 26.04. See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md). Upstream `latest` tags are rejected.

Apache-2.0. [docs/INSTALLER_SPEC.md](docs/INSTALLER_SPEC.md) · [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) · [docs/CONFIGURATION.md](docs/CONFIGURATION.md)
