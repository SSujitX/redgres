![Redgres self-hosted PostgreSQL and Redis control plane](web/public/assets/redgres-logo-light.png)

**One secure control plane for PostgreSQL and Redis.**  
Redgres is a Self-hosted database administration for a single PostgreSQL cluster and Redis instance.

![Redgres CI status](https://img.shields.io/github/actions/workflow/status/SSujitX/redgres/ci.yml?branch=master&style=flat-square&label=CI)![GitHub stars](https://img.shields.io/github/stars/SSujitX/redgres?style=flat-square&logo=github)![Project status: development partial](https://img.shields.io/badge/status-development--partial-f59e0b?style=flat-square)![Self-hosted software](https://img.shields.io/badge/deployment-self--hosted-2563eb?style=flat-square)![License pending](https://img.shields.io/badge/license-pending-6b7280?style=flat-square)

![Go 1.27.0](https://img.shields.io/badge/Go-1.27.0-00ADD8?style=for-the-badge&logo=go&logoColor=white)![React 19.2.8](https://img.shields.io/badge/React-19.2.8-20232A?style=for-the-badge&logo=react&logoColor=61DAFB)![TypeScript 7.0.2](https://img.shields.io/badge/TypeScript-7.0.2-3178C6?style=for-the-badge&logo=typescript&logoColor=white)![PostgreSQL 17 and 18 compatibility targets](https://img.shields.io/badge/PostgreSQL-17%20%7C%2018-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)![Redis 8.2 and 8.8 compatibility targets](https://img.shields.io/badge/Redis-8.2%20%7C%208.8-DC382D?style=for-the-badge&logo=redis&logoColor=white)

Redgres is a self-hosted Go and React administration console for securely managing one PostgreSQL cluster and one Redis instance. It combines PostgreSQL project database provisioning, Redis ACL user management, credential-safe workflows, audit history, health monitoring, and protected links to expert tools in one operator-focused interface.

> [!WARNING]
> Redgres is under active development and is **not production accepted**. Do not retire an existing database console until the migration, recovery, compatibility, staging, and production gates are complete. See the evidence-backed [implementation matrix](docs/TRACEABILITY.md) and [acceptance checklist](docs/ACCEPTANCE_CHECKLIST.md).



## Features

- Manage PostgreSQL project databases and least-privilege project roles.
- Inspect schemas, tables, bounded row data, connections, and database security state.
- Issue, reveal, and rotate PostgreSQL credentials through the legacy-compatible encrypted vault.
- Create and manage Redis ACL users with explicit command allow-lists and key-prefix isolation.
- Review secret-safe audit history and PostgreSQL, PgBouncer, Redis, and control-plane health.
- Use a responsive React interface with authenticated search and links to pgAdmin and RedisInsight.
- Build one Go binary with the production frontend embedded.
- Follow deterministic installer, backup, verification, update, and rollback contracts as they become accepted.



## Quick start: clone, build, and run Redgres



### Prerequisites

- [Git](https://git-scm.com/)
- [Go 1.27.0](https://go.dev/)
- [Node.js 24.19.0](https://nodejs.org/) with npm

PostgreSQL and Redis are optional for interface development. Without service configuration, Redgres starts and reports those dependencies as unavailable.

### 1. Clone the repository

```shell
git clone https://github.com/SSujitX/redgres.git
cd redgres
```



### 2. Install frontend dependencies and build

```shell
npm --prefix web ci
npm --prefix web run build
go build ./cmd/redgres
```

The frontend build is embedded into the Go application. Required versions are pinned in `go.mod`, `web/.nvmrc`, and the lockfiles.

### 3. Create a local development owner

```shell
go run ./cmd/redgres create-owner --username admin
```

Enter the owner password only at the secure interactive prompt. Do not place passwords, tokens, connection URLs, or private keys in `.env`, command arguments, logs, commits, or browser storage.

### 4. Run the embedded API and web interface

```shell
npm --prefix web run dev:full
```

Open [http://127.0.0.1:8989](http://127.0.0.1:8989). Stop the development stack with `Ctrl+C`. A frontend rebuild briefly clears the embedded assets; refresh after Vite finishes.

## Development usage



### Frontend hot module replacement

Run the Go API and Vite frontend in separate terminals.

Terminal 1 — PowerShell:

```powershell
$env:REDGRES_BASE_URL = "http://127.0.0.1:5173"
go run ./cmd/redgres serve
```

Terminal 2:

```shell
npm --prefix web run dev
```

Open [http://127.0.0.1:5173](http://127.0.0.1:5173). Vite proxies `/api` requests to the local Go process.

### Run tests and production builds

```shell
go test ./...
go vet ./...
npm --prefix web run test:run
npm --prefix web run build
```

Browser tests require Playwright Chromium:

```shell
npm --prefix web exec -- playwright install chromium
npm --prefix web run test:e2e
```



## Product scope

Redgres is designed for one trusted owner operating one PostgreSQL cluster and one Redis instance. It is a focused self-hosted control plane—not a public DBaaS, SQL workbench, Redis key browser, Kubernetes operator, fleet manager, or replacement for pgAdmin and RedisInsight.

The browser never connects directly to PostgreSQL or Redis. Browser/admin origins bind to loopback and are intended to be reached through Cloudflare Tunnel and Access; native PostgreSQL and Redis clients use separate end-to-end TLS endpoints. Read [Security](docs/SECURITY.md), [Deployment](docs/DEPLOYMENT.md), and [Domain and network](docs/DOMAIN_AND_NETWORK.md) before planning a real deployment.

## Documentation

Start with the [documentation index](docs/INDEX.md). Key references:

- [Project requirements](docs/PRD.md)
- [Architecture and data flow](docs/ARCHITECTURE.md)
- [Current implementation and test evidence](docs/TRACEABILITY.md)
- [Supported compatibility matrix](docs/COMPATIBILITY.md)
- [Security architecture](docs/SECURITY.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Deployment topology](docs/DEPLOYMENT.md)
- [Installer specification](docs/INSTALLER_SPEC.md)
- [Backup and recovery](docs/BACKUP_RECOVERY.md)
- [Contributing guide](CONTRIBUTING.md)



## Project status and license

Redgres is the migration successor to the local read-only reference systems `../database-app` and `../redis-ui`; production does not depend on those repositories. Current requirements remain Partial until their documented acceptance evidence is complete.

No project license has been declared yet. Apache-2.0 is the recommended license, but the repository must not be represented as Apache-2.0 until a license file is approved and added. Vendored agent skills retain their upstream notices in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). The Redgres name has not been represented as legally cleared.

Cursor users can open `Redgres.code-workspace` and run `/start-redgres`. The workflow may create reviewed local commits, but it never pushes or changes production, DNS, Cloudflare, or secrets without separate explicit authorization.