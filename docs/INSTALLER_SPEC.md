# Installer and command-interface specification

The installer is one operator entry point backed by small testable modules. “One shell command” does not mean one unstructured shell file or one file containing every secret.

## Command interface

```bash
sudo ./deploy/install.sh --mode existing-postgres --expect-postgres-major 17 --pgbouncer-mode existing --redis-mode existing --expect-redis-series 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh --mode existing-postgres --expect-postgres-major 17 --pgbouncer-mode existing --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart --redis-mode existing --expect-redis-series 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh --mode fresh-postgres --postgres-version 18 --pgbouncer-mode fresh --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart --redis-mode fresh --redis-version 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh postgres-plan --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json
sudo ./deploy/install.sh postgres-extensions apply --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart
sudo ./deploy/install.sh verify --config /root/redgres/install.env
sudo ./deploy/install.sh backup --config /root/redgres/install.env
sudo ./deploy/install.sh update --non-interactive --dry-run --release /root/redgres/redgres_VERSION_linux_amd64.tar.gz
sudo ./deploy/install.sh rollback --non-interactive --dry-run --to VERSION
```

`install.sh` dispatches to version-controlled modules. It must support `--dry-run` for safe checks where feasible and `--non-interactive` only when every required decision is explicit.

**This Partial (OPS-005):** `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` print skip matrices (`result=partial`). `--release` must be an existing regular file (`[[ -f ]]`) and is never sourced, extracted, checksummed, or printed. `--to VERSION` must be a path-safe token (no `/`, `.` or `..` as a path component, no NUL) and is never printed. Live update/rollback without `--dry-run` exits 2. This Partial is not Complete.

The initial selections and defaults are defined in [COMPATIBILITY.md](COMPATIBILITY.md). Interactive setup presents only versions supported by the current Redgres release. Non-interactive fresh setup requires explicit versions. `latest-tested` may resolve only through reviewed release metadata and must be converted to an exact version before mutation; upstream `latest` tags are forbidden.

## Required configuration classes

- Deployment profile: public hostnames, loopback ports, release source/checksum.
- Service lifecycle: fresh/existing mode independently for PostgreSQL and Redis; selected version for fresh services; optional expected version for existing services.
- PostgreSQL capabilities: independent PgBouncer mode plus an optional non-secret, machine-validated extension plan defined by [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
- Existing services: detected PostgreSQL version/cluster, config paths, PgBouncer version/path, Redis version/Compose project/volume.
- Network: intended public listeners, allowed source CIDRs, SSH port.
- Cloudflare: zone/account/tunnel identifiers supplied explicitly; token file paths.
- TLS: certificate names, Certbot credentials file, deploy hooks.
- Backup: destination, retention, off-host target, encryption mechanism, restore-test target.
- Feature flags and protected databases/roles.

The installer does not guess zones, delete records, generate publicly trusted credentials, or print secrets.

## Stage contract

1. **Preflight** — root, supported OS/architecture, requested service versions, package/image availability, clock, DNS, RAM/disk/inodes, network, commands, port conflicts.
2. **Inventory** — record package/service versions, cluster identities, listeners, data directories, configs, DNS/TLS observations, and checksums without secrets. **This Partial (OPS-002):** `--non-interactive --dry-run` only; resolve the PATH candidate for `postgres`, `redis-server`, and `pgbouncer` once, require an absolute executable regular file, reject a symlink or group/world-writable binary, and execute only that validated absolute path when the component’s mode is existing. When the installer runs as root, the binary and every containing directory must be root-owned and not group/world writable, so a PATH-shadowed binary under an untrusted directory is never executed. After the PATH candidate is resolved, non-root inventory also validates every ancestor directory (owner is 0 or the installer EUID, not a symlink, not group/world-writable) or refuses to execute; the binary is re-validated immediately before `--version`. Invocation is `--version` only (no `-D`/`PGDATA`, no Redis conf, no PgBouncer ini; not `psql -V`, not `redis-cli`); skip fresh/disabled without invoking the binary; missing, untrusted, non-zero, empty, unparseable, unsupported, or expected-version mismatch fails closed (exit 1). SQL `SHOW`, Redis `INFO`, PgBouncer `SHOW VERSION`, cluster identity, listeners, data directories, and backup evidence are deferred. Inventory does not run without `--dry-run`. `--config` is never sourced.
3. **Safety gate** — in existing mode, require a fresh verified backup manifest before PostgreSQL/Redis configuration changes.
4. **Packages** — install approved repositories and exact release-pinned packages/images idempotently; record container digests.
5. **Identity/filesystem** — create `redgres` system user/group and exact FHS paths/modes.
6. **Redis** — detect or install the selected supported series, validate capabilities, Compose and persistent mounts; never replace a named volume silently.
7. **PostgreSQL/PgBouncer** — apply independent existing/fresh lifecycles; fresh bootstrap only for the selected supported major; existing mode defaults to preserve; validate an optional extension plan, exact packages, preload merge and restart impact under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
8. **TLS/DNS** — issue/validate raw-service certificates and renewal hooks; configure declared DNS records/routes only.
9. **Application release** — verify artifact checksum, install immutable release, migrate SQLite, configure systemd credentials/unit. **This Partial (OPS-005):** `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` only; PATH must be an existing regular file (`[[ -f ]]`) and is never sourced, evaluated, extracted, checksummed, or printed. VERSION is a path-safe token (no `/`, `.` or `..` as a path component, no NUL) and is never printed. Prints an explicit skip matrix (`result=partial`). Update does not write `/opt/redgres/releases`, does not switch `current`, does not migrate SQLite, does not write systemd units/credentials, does not probe GET `/api/v1/healthz` (curl not invoked), and does not install PostgreSQL packages. Rollback does not switch the symlink, does not check SQLite schema compatibility, does not restore config, does not restart systemd, does not probe healthz, and never reverses PostgreSQL/Redis/vault/credentials/DNS/schema. Missing `--non-interactive`, missing/non-file `--release`, missing/unsafe `--to`, or unknown flags (including `--config`/`--mode` on update) exit 1. Live update/rollback without `--dry-run` exits 2. Does not call tar, ln, sha256sum, curl, gpg, pg_dump, pg_restore, or redis-cli. Checksum is skipped because [CONFIGURATION.md](CONFIGURATION.md) has no expected digest key. This Partial is not Complete.
10. **Cloudflare** — install/validate cloudflared and routes; token stays protected.
11. **Firewall** — calculate intended rules, preserve SSH access, apply, then verify listeners externally and locally.
12. **End-to-end verify** — run [TESTING.md](TESTING.md) deployment checks. **This Partial (OPS-003):** `verify --non-interactive --dry-run --config PATH` only; PATH must be an existing regular file (`[[ -f ]]`) and is never sourced, evaluated, or printed. Prints an explicit skip matrix (`result=partial`) for DNS, Cloudflare Tunnel/Access/routes, public TLS, GET `/api/v1/healthz` (curl not invoked), GET `/api/v1/status` auth boundaries, live bindings (intended redgres `127.0.0.1:8790`), cluster SHOW/INFO, and backup prerequisites (no named backup keys; OPS-004). Does not call inventory, curl, wget, cloudflared, or certbot. Live verify without `--dry-run` exits 2. Missing `--non-interactive`, missing/non-file `--config`, or unknown flags (including `--mode`) exit 1. DNS/Cloudflare/public TLS remain skipped; this Partial is not Complete.
13. **Report** — produce a redacted manifest with changed/skipped items and exact follow-ups. Never emit credentials.

## Idempotency

Every step classifies state as `already-correct`, `change-required`, `blocked`, or `failed`. Re-running after interruption must converge without creating duplicate users, tunnels, DNS records, certificates, timers, or releases.

The script uses strict shell mode, explicit paths, traps, and temporary directories from `mktemp -d`; it never parses `ls`, evaluates config as shell, downloads and executes unverified scripts, or uses broad recursive deletion.

## Existing PostgreSQL protections

- Detect and confirm cluster major/full version, cluster name, data directory, system identifier, port, and service unit.
- Reject unsupported versions and expected-major mismatches before mutation; never interpret an expected version as an upgrade request.
- Refuse if expected data directory/system identifier changes during run.
- Never run `initdb`, `pg_dropcluster`, package purge, data-directory move, or destructive restore.
- Back up configuration and logical/global objects before modifications.
- Apply config fragments only after syntax checks; reload before restart when possible.
- Preserve local emergency access and validate a second SSH session before firewall changes.
- Inventory installed/available extensions in every in-scope database, extension owners/schemas/versions, host packages and `shared_preload_libraries` without changing them.
- Default to extension policy `preserve`. Package installation, preload/config changes, restart and extension DDL require a separate reviewed plan; a required restart also requires explicit approval.

## Fresh PostgreSQL protections

- Require explicit `fresh-postgres` mode.
- Require an explicit supported major in non-interactive mode; the interactive default is PostgreSQL 18.
- Refuse when a cluster/data directory already contains data unless a separate destructive workflow is approved outside the installer.
- Generate/administer credentials through protected files/stdin, not process arguments.
- Complete TLS/SCRAM/ACL baseline before public firewall rules open.
- Install only release-pinned operator-selected extension capabilities; the default optional set is empty and nothing is enabled in `template1` automatically.

## PostgreSQL extension and PgBouncer protections

- Treat PostgreSQL server packages, extension host packages, per-database SQL extension state, preload libraries and PgBouncer as separate resources.
- Resolve capability IDs through release metadata; never accept arbitrary package names, repositories, SQL, preload libraries or floating versions from installer input.
- Execute all extension inventory/DDL through the direct PostgreSQL path, never PgBouncer.
- Merge and preserve existing preload libraries. Validate configuration and report a pending restart; do not restart without `--approve-postgres-restart`.
- For an approved restart, pause/drain PgBouncer, restart PostgreSQL once, verify direct health, resume PgBouncer and verify pooled application behavior.
- Require explicitly named databases; never interpret empty scope as all databases, never mutate `template1`, and never silently change extension owner/schema/version.
- Never run `ALTER EXTENSION UPDATE`, `DROP EXTENSION`, package removal, TimescaleDB tuning, pg_repack maintenance or pgAudit “log all” as an install/update side effect.
- Record exact package/repository/digest where applicable, SQL extension version/schema/owner, preload/config diff, restart and verification evidence without secrets.

## Redis version protections

- Require independent `fresh` or `existing` Redis mode.
- Fresh mode accepts only a supported series and resolves it to an exact image tag and digest. The production default is Redis 8.2; local development defaults to the release's latest-tested Redis 8.8 series.
- Existing mode detects `redis_version`, verifies ACL/persistence capabilities, and rejects unsupported versions or expected-series mismatches before mutation.
- Never replace a persistent volume or perform a Redis series upgrade as a side effect of installing/updating Redgres.

## Rollback limits

The installer automatically rolls back a failed application symlink/unit deployment. It may restore a configuration file it changed after validation. It does not automatically undo package upgrades, SQL migrations, PostgreSQL roles/databases, Redis ACL mutations, credential rotations, DNS propagation, or production data.

**This Partial (OPS-005):** `rollback --non-interactive --dry-run --to VERSION` prints `data_reversal: skipped` and does not switch `current`, restore config, restart systemd, or probe health. Application rollback remains binary/config only; it never reverses PostgreSQL/Redis/vault/credentials/DNS/schema automatically. Live rollback without `--dry-run` exits 2. This Partial is not Complete.

## Acceptance tests

- Two consecutive installs make no unintended second-run changes.
- Interrupted run at every stage safely resumes.
- Existing mode leaves PostgreSQL system identifier/data checksum unchanged.
- Every combination in [COMPATIBILITY.md](COMPATIBILITY.md) passes the applicable fresh/adoption preflight and service integration suite.
- Install reports record selected/detected versions, exact packages/images/digests, compatibility checks, and policy revision without secrets.
- Port/firewall changes do not lock out the validation SSH path.
- Output and logs pass fake-secret scanning.
- Unavailable DNS/Cloudflare/apt/Docker dependencies fail with actionable state and safe retry.
- Existing PostgreSQL preserve mode changes no packages, preload libraries, service state or extension state; selected capability plans pass the PostgreSQL-major matrix, restart/PgBouncer orchestration, per-database idempotency and restore tests in [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
