# Service compatibility and version policy

Status: target; no version is production-supported until the corresponding release jobs, installer rehearsals, and restore tests pass.

This document is the authoritative Redgres service-version matrix. Other documents should link here instead of independently claiming support. Exact package versions, container tags, and image digests belong in each release manifest.

## 1. Terms

- **Supported** means the Redgres release passed the required integration, installer, backup, restore, and security tests for that service version.
- **Provisioning support** means the installer may create and configure that version on a fresh target.
- **Connection support** means Redgres may administer an existing instance after detecting its real version and capabilities.
- **Latest tested** means the newest stable version explicitly approved in the Redgres release manifest. It never means an upstream `latest` package, container tag, beta, RC, or nightly build.
- **Expected version** is an optional operator assertion used to detect the wrong server. It cannot expand the supported matrix.

## 2. Initial target matrix

| Component | Version unit | Fresh provisioning choices | Existing-instance connection | Local default | Production default |
|---|---|---|---|---|---|
| PostgreSQL | major | 17, 18 | 17, 18 | 18 | 18 |
| Redis Open Source | minor series | 8.2, 8.8 | 8.2, 8.8 | 8.8 | 8.2 |
| PgBouncer | full release | release-pinned tested version | detected version must pass compatibility checks | release-pinned | release-pinned |

PostgreSQL 17 remains supported for existing deployments and compatibility testing; a fresh installation defaults to PostgreSQL 18. Redis 8.2 is the conservative production default because it has a published long support window. Redis 8.8 is the Redgres local latest-tested default ([ADR-008](decisions/ADR-008-service-version-policy.md)); it is not Hub `redis:latest`. On 2026-08-26 the official Redis image grouped `latest` with **8.10.1**, a series this matrix does not include. Discovery of a newer upstream series is not support (see §7). These are initial targets, not implementation evidence.

PostgreSQL extension support is a second release-owned matrix keyed by PostgreSQL major, Ubuntu architecture and capability. The candidate registry and lifecycle are in [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). A server major may be supported while a particular optional capability is unavailable; installer input cannot turn missing/unverified extension packaging into support. PgBouncer remains a separately pinned service, not an extension row.

PostgreSQL patch releases and Redis patch releases must be updated to an approved security/current patch within their selected major or series. Automation must pin the exact resolved package version and, for containers, the immutable image digest. Floating `postgres:latest`, `redis:latest`, unbounded package upgrades, prereleases, and unsupported repositories are forbidden.

## 3. Selection and detection behavior

### Fresh services

- The operator selects a supported PostgreSQL major and Redis series.
- Interactive setup presents recommended choices first and labels compatibility alternatives clearly.
- Non-interactive setup requires explicit values; `latest-tested` may be accepted only if it resolves through the signed/reviewed Redgres release manifest and the resolved version is recorded before mutation.
- Preflight rejects a selection outside this matrix before installing packages, creating data directories, or changing listeners.

### Existing services

- Redgres detects PostgreSQL with `SHOW server_version_num` and records the server-reported full version.
- Redgres detects Redis from `INFO server` and records `redis_version`.
- Redgres detects PgBouncer with `SHOW VERSION` and verifies required pooling/authentication behavior.
- An expected-version input is an identity guard. A mismatch fails before mutation; it does not trigger an upgrade or override detection.
- Unsupported or unparseable versions fail closed for administrative mutations. A future read-only diagnostic mode requires its own documented contract and must not imply support.

Version strings are necessary but insufficient. Startup/preflight also checks required capabilities, catalog/query behavior, authentication, TLS, Redis ACL commands, persistence layout, and PgBouncer pooling behavior.

## 4. Upgrade boundaries

- Redgres application updates never perform a PostgreSQL major upgrade implicitly.
- Moving PostgreSQL 17 to 18 requires a separate operator-approved workflow using an appropriate method such as `pg_upgrade`, dump/restore, or logical replication, with current backups and rollback planning.
- Redis series upgrades require release-note review, verified RDB/AOF/ACL backups, isolated restore/load testing, and explicit operator approval.
- Patch updates may be automated only by a separately tested maintenance policy; they remain visible in the operation report and release manifest.
- Application rollback never rolls back PostgreSQL, Redis, or PgBouncer binaries or data formats automatically.

## 5. Installer and configuration ownership

Provisioning inputs are non-secret installer settings such as:

```text
POSTGRES_MODE=fresh|existing
POSTGRES_MAJOR=17|18
REDIS_MODE=fresh|existing
REDIS_SERIES=8.2|8.8
```

For an existing service, the version value is an optional expected-version assertion; runtime detection remains authoritative. The supported-version list is owned by Redgres release metadata/code and is not configurable through environment variables.

Every successful install or adoption report records:

- selected and detected version;
- exact package/container version and repository;
- image digest where applicable;
- PostgreSQL cluster identity and PgBouncer version;
- compatibility checks executed and their results;
- Redgres release and compatibility-policy revision.

The report contains no credentials or credential-bearing URLs.

## 6. Required CI and release matrix

The initial service matrix contains four primary combinations:

| Job | PostgreSQL | Redis |
|---|---:|---:|
| compatibility-17-8.2 | 17 | 8.2 |
| compatibility-17-8.8 | 17 | 8.8 |
| compatibility-18-8.2 | 18 | 8.2 |
| compatibility-18-8.8 | 18 | 8.8 |

Each applicable job covers the administrative catalog/SQL behavior, roles and protected resources, PgBouncer direct-versus-pooled behavior, Redis ACL presets and representative workloads, TLS/authentication, backup capture, and isolated restore. Installer VM tests cover both defaults and at least one non-default supported selection.

Optional PostgreSQL capabilities add focused jobs rather than multiplying every capability into the core PostgreSQL × Redis matrix. Each focused job records the PostgreSQL major, exact package/extension versions, fresh versus existing-preserve/apply path, preload/restart behavior, named database state and restore result.

## 7. Adding, deprecating, or removing a version

A version-matrix change requires:

1. An ADR update or superseding ADR explaining the lifecycle decision.
2. Official release/support-policy review and dependency/package availability verification.
3. Green integration and installer jobs for the complete affected matrix.
4. Backup and isolated restore evidence for the new version or upgrade path.
5. Updated release metadata, documentation, examples, and operator-facing warnings.
6. A documented deprecation window before removing an already supported version, except for an urgent security reason.

Discovery that an upstream version exists is not enough to claim support.

## 8. Local skippable live-test pins (not §6 evidence)

This section freezes artifacts for **skippable** local live tests when Docker is available. It does **not** make any version production-supported. It does **not** pass §6 (four jobs, PgBouncer, TLS, backup, restore). [TESTING.md](TESTING.md) already treats skippable live tests as distinct from that matrix. Redis CI matrix jobs stay blocked until `redis-ui` has Git provenance.

First cell (local defaults: PostgreSQL 18 × Redis 8.8):

| Service | Pin | Forbidden |
|---|---|---|
| PostgreSQL | `postgres:18.6` | `postgres:latest`, floating `postgres:18` |
| Redis Open Source | `redis:8.8.2` | `redis:latest`, floating `redis:8.8` |
| PgBouncer | omitted | any third-party Hub `pgbouncer` image |

Sources (retrieved 2026-08-26, no `docker pull` in that review): PostgreSQL 18.6 news and [release notes](https://www.postgresql.org/docs/release/18.6/); [Docker Official Image `postgres`](https://hub.docker.com/_/postgres); Redis 8.8 [release notes](https://github.com/redis/redis/blob/8.8/00-RELEASENOTES) (8.8.2 SECURITY, 2026-08-17); [Docker Official Image `redis`](https://hub.docker.com/_/redis). PgBouncer publishes [source/tarballs](https://www.pgbouncer.org/downloads/) and distro packages, not a Docker Official Image (`library/pgbouncer` 404). Production PgBouncer remains host-native ([ADR-002](decisions/ADR-002-hybrid-deployment.md)).

Official `postgres` and `redis` images do **not** start TLS by default. A plaintext loopback cell cannot satisfy §6 TLS/authentication.

Hub tags are mutable. The Hub v2 index digest snapshot from 2026-08-26 is **not** pull evidence:

```text
postgres:18.6@sha256:1957b2ff3137e4ef7f3bc813e74fff50b1e1ffddc85c8b9d6f14ade972be8687
redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3
```

Authoritative freeze is `RepoDigests` from the first successful `docker pull`, plus runtime `SHOW server_version` / `SHOW server_version_num` and Redis `INFO server` `redis_version`. If pull digests differ from the snapshot, freeze the pull digest. A green job on floating `latest` is not evidence.
