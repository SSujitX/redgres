# PostgreSQL provisioning, adoption, extensions, and PgBouncer

Status: target contract. No package, extension, or service is supported until its exact release artifact and applicable PostgreSQL-major job pass the release gates.

This document is the canonical PostgreSQL host-lifecycle contract for Redgres. It separates four operations that must never be confused:

1. installing or adopting the PostgreSQL server;
2. installing extension files/packages on the host;
3. enabling an extension with `CREATE EXTENSION` in a named database;
4. installing or adopting PgBouncer as a separate service.

Installing an extension package does not enable it in any database. `CREATE EXTENSION` is normally per database and is a schema mutation. PgBouncer is not a PostgreSQL extension and is never managed with extension SQL.

## 1. Supported lifecycle choices

PostgreSQL and PgBouncer are selected independently.

| Component | Mode | Meaning | Default safety behavior |
|---|---|---|---|
| PostgreSQL | `existing` | Adopt a PostgreSQL cluster already installed on this server. | Inventory and preserve; no `initdb`, major upgrade, package-source replacement, restart, or extension mutation. |
| PostgreSQL | `fresh` | Install a supported PostgreSQL major and create a new cluster on this server. | Refuse if a conflicting/non-empty cluster or data directory exists. |
| PgBouncer | `existing` | Adopt an existing PgBouncer service. | Inventory version/config/listener/pool behavior; preserve until an explicit plan is approved. |
| PgBouncer | `fresh` | Install the release-pinned PgBouncer package and configure a new service. | Refuse unexplained listener/config conflicts; verify direct and pooled paths independently. |
| PgBouncer | `disabled` | Do not provide a pooled endpoint. | Development/test only unless the release profile explicitly permits it; never advertise port 6432. |

Production installation must use PostgreSQL `existing` or `fresh`. It must use PgBouncer `existing` or `fresh` when pooled URLs are enabled. Redgres administrative SQL, migrations, extension DDL, backups, and health checks use the direct PostgreSQL path, never PgBouncer.

The supported PostgreSQL majors and exact package policy are owned by [COMPATIBILITY.md](COMPATIBILITY.md). A mode or expected-version setting is an identity/safety assertion; it never authorizes a major upgrade.

## 2. Existing-cluster adoption

Existing mode is the safest and recommended choice when production PostgreSQL is already present.

Preflight must discover and record, without secrets:

- server full/major version, package origin and pinned candidate versions;
- cluster name, system identifier, data directory, configuration/HBA/identity files and checksums;
- service unit, owner, port, socket directories, listeners, TLS and SCRAM posture;
- databases, installed and available extensions per database, extension owners/versions/schemas;
- `shared_preload_libraries`, pending-restart settings, worker limits and memory headroom;
- PgBouncer version, service/config/auth source, listeners, pool mode, prepared-statement behavior and direct target;
- backup/restore readiness and current maintenance window constraints.

Default existing-mode extension policy is `preserve`: report drift and missing requested capabilities, but make no package, preload, restart, `CREATE EXTENSION`, `ALTER EXTENSION`, or drop change.

An operator may submit an explicit extension plan later. Before applying it, Redgres must produce a diff, verify a current backup for every existing target database, pin package versions, check free disk/RAM/worker capacity, identify reload versus restart, and require explicit approval for a PostgreSQL restart. Existing `shared_preload_libraries` entries are merged and preserved; Redgres never replaces the list blindly or uses `ALTER SYSTEM` as an opaque shortcut.

## 3. Fresh PostgreSQL installation

Fresh mode performs these stages:

1. verify Ubuntu/architecture and confirm that no existing data-bearing cluster or conflicting data directory will be touched;
2. configure a reviewed package source and key only when needed;
3. resolve the selected supported major to exact release-pinned PostgreSQL server, client, and contrib packages;
4. create one named cluster with explicit data/config paths and record its system identifier;
5. apply the TLS, SCRAM, protected-role, listener and backup baseline before opening public firewall rules;
6. install only extension packages selected by the operator and supported by the release manifest;
7. merge selected preload libraries, validate configuration, restart once if required and approved;
8. enable selected extensions only in explicitly named databases;
9. install or adopt PgBouncer independently and verify direct versus pooled behavior;
10. emit a redacted manifest and run end-to-end verification.

Fresh does not mean “install everything.” The default extension selection is empty. The interactive installer may recommend profiles, but it must show their exact capabilities and consequences and convert the result into an explicit plan before mutation. Redgres never enables optional extensions in `template1` automatically.

## 4. Declarative extension plan

Installer configuration carries the lifecycle mode and a path to a non-secret, machine-validated JSON plan. Package names and versions are not accepted from the operator; Redgres resolves capability IDs through the signed/reviewed release manifest for the detected PostgreSQL major and Ubuntu architecture.

Example target shape:

```json
{
  "policy": "apply-selected",
  "selections": [
    {
      "capability": "pg_stat_statements",
      "databases": ["app_production"]
    },
    {
      "capability": "vector",
      "databases": ["search_production"]
    },
    {
      "capability": "pg_partman",
      "databases": ["events_production"],
      "scheduler": "pg_cron"
    }
  ]
}
```

Rules:

- `policy` is `preserve` or `apply-selected`; existing mode defaults to `preserve`.
- Capability identifiers come from the registry below. Unknown names fail before package or SQL changes.
- Every database name is explicit, validated, protected-policy checked, and connected through the direct administrative path.
- The extension plan never creates a database. Every named database must already exist; database provisioning remains a separate audited operation. After Redgres creates a future project database, the operator may apply a new reviewed extension plan to that database.
- An empty `databases` list may install host files only when the capability allows it; it never implies “all databases.”
- Preload libraries, package names, SQL extension names, companion components and restart requirements are derived from release metadata, not copied from user input.
- `CREATE EXTENSION IF NOT EXISTS` is not used as a substitute for inspection. The installer first records installed version, schema and owner and verifies that an existing installation matches policy.
- Extension upgrades, relocations, owner changes, drops and `CASCADE` are separate change workflows and are never implicit in install, update or rollback.
- Each database mutation is journaled independently. Failure stops subsequent mutations and reports exact completed/pending targets without claiming global rollback.

The target command surface is:

```bash
sudo ./deploy/install.sh --mode fresh-postgres --postgres-version 18 --pgbouncer-mode fresh --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart --redis-mode fresh --redis-version 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh postgres-plan --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json
sudo ./deploy/install.sh postgres-extensions apply --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json
sudo ./deploy/install.sh postgres-extensions apply --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart
```

`postgres-plan` is read-only. The apply command refuses a required restart without `--approve-postgres-restart`; non-interactive mode also requires the reviewed plan digest so a changed file cannot be applied accidentally.

**This Partial (OPS-007):** `deploy/install.sh postgres-plan --config PATH --extension-plan PATH` validates the plan JSON (policy, capability IDs from the initial registry below, explicit non-empty identifier-safe databases, optional `pg_partman`-only scheduler) and prints a plan preview plus a skip matrix (`result=partial`). `postgres-extensions apply --non-interactive --dry-run` validates the same plan and prints an apply skip matrix (`result=partial`), and the main install `--dry-run` validates an optional `--extension-plan` with the same rules. It never resolves packages/preload/restart (no release manifest in this Partial), never sources `--config`, never mutates, and never touches `template1`. Live `postgres-extensions apply` and live install remain unimplemented (exit 2). Not Complete.

## 5. Initial capability registry

The names below are candidate capabilities, not a promise that every combination is packaged or tested. A Redgres release manifest maps each supported capability and PostgreSQL major to exact package versions, checksums/repository origin, SQL extension versions and verification queries. Missing artifacts fail closed.

| Capability ID | Source/type | SQL extension | Preload/restart | Enablement and operational boundary |
|---|---|---|---|---|
| `pg_stat_statements` | PostgreSQL contrib | `pg_stat_statements` | `pg_stat_statements`; restart | Tracks cluster-wide statements after preload; views/functions are enabled in each named database. Query text is sensitive operational data and requires restricted access/retention. |
| `pg_repack` | Third-party package + matching client | `pg_repack` | None normally | Enable in each database to be maintained. Client and server extension versions must match. Running repack is a separate high-impact maintenance action requiring target locks, capacity checks and roughly table/index-scale temporary disk headroom; installer never runs it automatically. |
| `pg_buffercache` | PostgreSQL contrib | `pg_buffercache` | None | Diagnostic extension enabled only in named administrative databases. PostgreSQL 18 includes cache-eviction functions, so grants remain tightly restricted and Redgres exposes no eviction action. |
| `vector` | pgvector third-party package | `vector` | None | Enable once in every database that stores vector columns/indexes. Package must match the PostgreSQL major; index/resource tuning belongs to the application/database design. |
| `pg_trgm` | PostgreSQL contrib, trusted | `pg_trgm` | None | Enable in each named database needing trigram similarity or GiST/GIN operator classes. Redgres still performs extension creation with its controlled administrator and records ownership/schema. |
| `postgis` | PostGIS third-party packages | `postgis` | None normally | Enable only in spatial databases. Raster, topology, SFCGAL, geocoder and address-standardizer components are separate explicit capabilities; core `postgis` never implies all companions. |
| `timescaledb` | TimescaleDB third-party repository/package | `timescaledb` | `timescaledb`; restart | Enable in each time-series database. Tuning can change memory/WAL/worker settings and is never auto-accepted on an existing cluster; license/telemetry settings and backup/restore procedure are recorded explicitly. |
| `pg_partman` | Third-party/PGDG package | `pg_partman` | Optional `pg_partman_bgw`; restart if BGW selected | Install in a dedicated schema in each named database. Scheduler is explicit: `pg_partman_bgw`, `pg_cron`, or external. Redgres never enables two schedulers for the same maintenance plan. |
| `pg_cron` | Third-party/PGDG package | `pg_cron` | `pg_cron`; restart | Extension metadata lives in exactly one configured control database per cluster. Jobs for other databases use supported cross-database scheduling. Local connection/HBA behavior, execution role, job visibility and logging are verified. |
| `pgcrypto` | PostgreSQL contrib, trusted | `pgcrypto` | None | Enable per named database. Requires PostgreSQL built with OpenSSL. It is application SQL functionality and is separate from Redgres’s legacy Fernet vault implementation. |
| `pgaudit` | pgAudit package aligned to PostgreSQL major | `pgaudit` | `pgaudit`; restart | Create the extension before enabling `pgaudit.log`. Audit class, role scope, log volume, retention and secret-redaction consequences require an approved policy; “log all” is not a safe default. |

PgBouncer is deliberately absent from the capability registry. It is a host service with its own release, systemd unit, configuration, authentication source, TLS/listener policy, pooling mode, capacity and health checks.

## 6. Safe execution order

For an approved extension plan:

1. lock the installer operation and re-read cluster identity;
2. inventory current packages, available/installed extensions and configuration;
3. validate the plan against the release manifest and target PostgreSQL major;
4. verify package signatures/checksums, repository origin, free disk/RAM/workers and a current target backup;
5. install exact packages without PostgreSQL major upgrade, package purge or autoremove;
6. write a minimal owned configuration fragment after merging current preload libraries;
7. validate settings and determine whether reload/restart is required;
8. pause/drain PgBouncer when a PostgreSQL restart is approved, restart PostgreSQL once, verify direct health, then resume/verify PgBouncer;
9. execute extension DDL one named database at a time using fixed SQL identifiers and a safe `search_path`;
10. verify `pg_available_extensions`, `pg_extension`, expected library/config state and capability-specific smoke queries;
11. record exact artifacts, config diff, restart, database/schema/owner/version and test results in a secret-free report.

No public listener or firewall rule is opened until PostgreSQL direct TLS/authentication and PgBouncer tests pass.

## 7. Upgrade, rollback, backup, and restore

- Redgres application update never upgrades PostgreSQL, PgBouncer or an extension package implicitly.
- Installing a newer package does not authorize `ALTER EXTENSION UPDATE`; each database update path and release notes must be checked and backed up.
- Application rollback does not drop extensions, downgrade packages, remove preload libraries or reverse extension-created data.
- Package removal is forbidden while any database depends on its extension files.
- Restore hosts must install the exact required extension packages/control files before restoring dumps that contain `CREATE EXTENSION` records.
- PostGIS, TimescaleDB, pg_partman and other extension-owned metadata receive their documented backup/restore checks in addition to ordinary PostgreSQL backup tests.
- pgAudit and pg_stat_statements may contain sensitive query/object metadata; backup and log-retention policy must reflect that.

## 8. Verification gates

Every supported capability/major combination requires:

- clean fresh-package installation and existing-host adoption/preserve tests;
- unavailable/wrong-major/untrusted-repository rejection before mutation;
- preload merge, pending-restart detection, approved restart and PgBouncer drain/resume tests where applicable;
- per-database create, idempotent re-plan, version/schema/owner verification and restore test;
- explicit tests proving no unrequested database, `template1`, package, extension, config entry or service changed;
- secret scanning of commands, logs and reports;
- documented resource-impact tests for pg_repack, TimescaleDB, schedulers and pgAudit.

Until those jobs pass, the UI/installer may report a capability as detected or available, but not as Redgres-supported.
