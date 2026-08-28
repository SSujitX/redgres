![Redgres self-hosted PostgreSQL and Redis control plane](web/public/assets/redgres-logo-light.png)

**One secure control plane for PostgreSQL and Redis.**  
Redgres is a Self-hosted database administration for a single PostgreSQL cluster and Redis instance.

![Redgres CI status](https://img.shields.io/github/actions/workflow/status/SSujitX/redgres/ci.yml?branch=master&style=flat-square&label=CI)![GitHub stars](https://img.shields.io/github/stars/SSujitX/redgres?style=flat-square&logo=github)![Project status: development partial](https://img.shields.io/badge/status-development--partial-f59e0b?style=flat-square)![Self-hosted software](https://img.shields.io/badge/deployment-self--hosted-2563eb?style=flat-square)[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square)](LICENSE)

![Go 1.27.0](https://img.shields.io/badge/Go-1.27.0-00ADD8?style=for-the-badge&logo=go&logoColor=white)![React 19.2.8](https://img.shields.io/badge/React-19.2.8-20232A?style=for-the-badge&logo=react&logoColor=61DAFB)![TypeScript 7.0.2](https://img.shields.io/badge/TypeScript-7.0.2-3178C6?style=for-the-badge&logo=typescript&logoColor=white)![PostgreSQL 17 and 18 compatibility targets](https://img.shields.io/badge/PostgreSQL-17%20%7C%2018-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)![Redis 8.2 and 8.8 compatibility targets](https://img.shields.io/badge/Redis-8.2%20%7C%208.8-DC382D?style=for-the-badge&logo=redis&logoColor=white)

Redgres is a self-hosted Go and React administration console for securely managing one PostgreSQL cluster and one Redis instance. It combines PostgreSQL project database provisioning, Redis ACL user management, credential-safe workflows, audit history, health monitoring, and protected links to expert tools in one operator-focused interface.

> [!WARNING]
> Redgres is under active development and is **not production accepted**. Do not retire an existing database console until the migration, recovery, compatibility, staging, and production gates are complete. See the evidence-backed [implementation matrix](docs/TRACEABILITY.md) and [acceptance checklist](docs/ACCEPTANCE_CHECKLIST.md).

## Install / upgrade (application release)

After a GitHub Release exists (see below), on a Linux host:

```bash
# Latest release
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash

# Pin a version (application replace; prefer upgrade.sh to move forward)
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install.sh | sudo bash -s -- v=1.0.0

# Upgrade to latest (preserves /etc/redgres, /var/lib/redgres, PostgreSQL, Redis)
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/upgrade.sh | sudo bash

# Full purge — no prompt:
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y

# Full purge — interactive (y/n):
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash

# Application binary only (preserve databases):
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/uninstall.sh | sudo bash -s -- -y --app-only

# Dev build from master (not a release)
curl -fsSL https://raw.githubusercontent.com/SSujitX/redgres/master/install-dev.sh | sudo bash
```

Assets required on each release: `redgres_<version>_linux_amd64.tar.gz` and adjacent `SHA256SUMS`.

## Publish a release (Actions → VERSION → assets)

1. GitHub → **Actions** → **release** → **Run workflow**.
2. Choose **bump** (no free-typed version):
   - `current` — publish root [`VERSION`](VERSION) as-is (first release: **1.0.0**)
   - `patch` — `1.0.0` → `1.0.1`
   - `minor` — `1.0.0` → `1.1.0`
   - `major` — `1.0.0` → `2.0.0`
3. The workflow writes `VERSION`, syncs `web/package*.json` via [`scripts/set-version.sh`](scripts/set-version.sh), commits onto the branch, tags `vX.Y.Z`, builds the linux/amd64 tarball, and creates a GitHub Release with **categorized notes** (What's new / Fixes / Docs / … via [`scripts/generate-release-notes.sh`](scripts/generate-release-notes.sh)) plus assets **`redgres_X.Y.Z_linux_amd64.tar.gz`** and **`SHA256SUMS`**.

Local helpers:

```bash
./scripts/set-version.sh 1.0.1          # write VERSION + sync packages
./scripts/sync-version.sh check         # fail if package files drift
./deploy/build-release.sh               # reads VERSION; emits dist/release/*
```
