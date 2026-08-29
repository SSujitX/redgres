# Installer and command-interface specification

The installer is one operator entry point backed by small testable modules. “One shell command” does not mean one unstructured shell file or one file containing every secret.

## Command interface

```bash
sudo ./deploy/install.sh --mode existing-postgres --expect-postgres-major 17 --pgbouncer-mode existing --redis-mode existing --expect-redis-series 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh --mode existing-postgres --expect-postgres-major 17 --pgbouncer-mode existing --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart --redis-mode existing --expect-redis-series 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh --mode fresh-postgres --postgres-version 18 --pgbouncer-mode fresh --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart --redis-mode fresh --redis-version 8.2 --config /root/redgres/install.env
sudo ./deploy/install.sh postgres-plan --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json
sudo ./deploy/install.sh postgres-extensions apply --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json --approve-postgres-restart
sudo ./deploy/install.sh postgres-extensions apply --non-interactive --dry-run --config /root/redgres/install.env --extension-plan /root/redgres/postgres-extensions.json [--approve-postgres-restart]
sudo ./deploy/install.sh verify --config /root/redgres/install.env
sudo ./deploy/install.sh backup --config /root/redgres/install.env
sudo ./deploy/install.sh update --non-interactive --dry-run --release /root/redgres/redgres_VERSION_linux_amd64.tar.gz
sudo ./deploy/install.sh rollback --non-interactive --dry-run --to VERSION
```

`install.sh` dispatches to version-controlled modules. Invoke it directly (or explicitly with `/bin/bash -p`); plain `bash deploy/install.sh` is rejected because it bypasses the privileged shebang before `BASH_ENV` can be suppressed. It must support `--dry-run` for safe checks where feasible and `--non-interactive` only when every required decision is explicit.

**This Partial trusted bootstrap (OPS-001/002/006):** before sourcing any module, the Ubuntu entry point uses `/bin/bash -p`, requires privileged Bash mode, sets `umask 077`, `LC_ALL=C`, and runtime `PATH=/usr/sbin:/usr/bin:/sbin:/bin`, and clears shell-startup, search, and dynamic-loader environment inputs before its first external helper. The caller PATH is captured only as inert host-inventory search data. Trusted `stat`, `env`, and `sha256sum` are the first non-symlink of `/usr/bin/{stat,env,sha256sum}`, `/usr/bin/gnu{stat,env,sha256sum}`, `/usr/lib/cargo/bin/coreutils/{applet,coreutils}`, or `/usr/bin/coreutils` (invoked as `coreutils <applet>`). Ubuntu 26.04 rust-coreutils wrappers under `/usr/bin` are skipped. Live OS identity uses `/etc/os-release` when it is a regular file, otherwise the Debian/Ubuntu canonical `/usr/lib/os-release` when `/etc/os-release` is a symlink. The entry point, source directory, `lib/`, every source file, and their ancestors must be absolute, non-symlink, expected-kind, root-owned when EUID 0 (otherwise root/EUID-owned), and not group/world writable. Modules are opened once, pathname/descriptor identity is compared, and sourcing uses those descriptors. Host discovery scans the captured search list left-to-right without `command -v`, rejects empty/relative elements, fails on an unsafe first match, revalidates the selected absolute executable, and runs `--version` through `/usr/bin/env -i` with only fixed `PATH` and `LC_ALL`. Operator file inputs are absolute trusted regular files with the same ownership/mode/ancestor policy; release files remain unread, while extension plans and main-path lifecycle config use separate identity-pinned snapshots bounded to 64 KiB. This establishes local path trust, not release authenticity: signatures/digests, package provenance, bounded probes, and live mutation remain deferred. OPS-001/002/006 remain Partial.

**This Partial lifecycle-config parser (OPS-001/007):** main install `--dry-run --config PATH` reads one trusted descriptor-pinned snapshot and accepts only the documented keys `POSTGRES_MODE`, `POSTGRES_MAJOR`, `PGBOUNCER_MODE`, `POSTGRES_EXTENSION_POLICY`, and `POSTGRES_EXTENSION_PLAN_FILE`. Grammar is one uppercase `KEY=VALUE` per line plus empty lines and column-one `#` comments; duplicate keys, unknown keys, shell `export` syntax, malformed lines, NUL, and files larger than 64 KiB fail closed without printing values. Values remain literal data: no source, eval, interpolation, export, or command substitution. This slice validates syntax only; it does not apply values, establish CLI/config precedence, or authorize packages, preloads, restarts, extension DDL, or mutation. Other subcommands still trust-check but do not read `--config`.

**This Partial (OPS-005):** `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` print skip matrices (`result=partial`). `--release` must pass the trusted regular-file policy above. Update dry-run requires a trusted adjacent `SHA256SUMS` (≤64 KiB) with strict lowercase SHA-256 records and exactly one entry matching the release basename, hashes the pinned release descriptor, and fails on missing/malformed/duplicate/mismatched evidence without printing paths or digests. The dry-run path never extracts; signature/provenance remain unverified. `--to VERSION` must be a path-safe token (no `/`, `.` or `..` as a path component, no NUL) and is never printed. Live `update`/`rollback` without `--dry-run` apply application binaries under `/opt/redgres` (or `REDGRES_OPT_ROOT` for tests): checksum → extract → `releases/<version>` → `current` symlink → unit write → systemd restart when managing `/opt/redgres` → healthz (skippable with `REDGRES_SKIP_HEALTHZ=1`). They never reverse PostgreSQL/Redis/vault/credentials/DNS/schema. Public curl entry points `install.sh` / `upgrade.sh` download GitHub Release assets built from root `VERSION` via `.github/workflows/release.yml`. This Partial is not Complete.

**This Partial (OPS-007):** `postgres-plan --config PATH --extension-plan PATH` is a read-only extension-plan validator. `--config` and `--extension-plan` must pass the trusted regular-file policy above; `--config` is never sourced, evaluated, or printed, and the extension plan is read once through its pinned descriptor. It validates the non-secret plan JSON: `policy` is `preserve` or `apply-selected`; every `selections` entry has a capability ID from the release-owned registry (the initial list in [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md) section 5), a non-empty explicit `databases` array of PostgreSQL-identifier-safe names (never protected names like `postgres`, `template0`, `template1`, `database_console_vault`, never an empty list or empty-string element that implies "all databases"), and an optional `scheduler` that is valid only for `pg_partman` (`pg_partman_bgw`, `pg_cron`, or `external`). `pg_cron` selections must name exactly one control database; two distinct scheduler identities (e.g. `pg_cron` plus `pg_partman_bgw`) fail closed. The whole plan must be valid JSON with no unknown keys; malformed JSON, unknown capabilities, invalid or protected database names, empty-string database elements, scheduler misuse, and two schedulers fail closed (exit 1). It prints a plan preview and a skip matrix (`result=partial`): package resolution (no release manifest), live inventory, backup verification, preload merge, restart approval, extension DDL, and smoke verification are all skipped. `postgres-extensions apply --non-interactive --dry-run` validates the same plan and prints a plan preview plus an apply skip matrix (`result=partial`): package resolution, live inventory, backup verification, preload merge, restart approval (even with `--approve-postgres-restart`), extension DDL, and smoke verification are all skipped. The main `install.sh --dry-run` also validates an optional `--extension-plan` with the same rules when one is supplied, and an optional `--config` must pass the trusted regular-file policy when supplied (never sourced; still optional). It never mutates, never sources `--config`, never touches `template1`, and never resolves packages/preload/restart. Live `postgres-extensions apply` without `--dry-run` exits 2. This Partial is not Complete.
**This Partial (OPS-004):** `backup --non-interactive --dry-run --config PATH` prints a skip matrix (`result=partial`). `--config` must pass the trusted regular-file policy above and is never sourced, evaluated, or printed. It does not invoke `pg_dump`/`pg_dumpall`/`pg_restore`, Redis `BGSAVE`/`LASTSAVE`, SQLite online backup, checksums, pruning, or off-host copy — all are skipped (`installer-recovery`), and no backup manifest is written. Live backup without `--dry-run` exits 2. This Partial is not Complete.

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

**Deferred domain/TLS ([ADR-012](decisions/ADR-012-ui-bootstrap.md)):** public hostnames, Cloudflare credentials, and TLS may be omitted from the first `install.sh` pass. They are supplied later through the runtime Domain & Network wizard ([DOMAIN_AND_NETWORK.md](DOMAIN_AND_NETWORK.md); PRD OPS-008/OPS-009). The first pass must still complete preflight, packages, services, and report a source-restricted bootstrap URL for the console. **This Partial (OPS-008):** live install adds `ufw allow from <operator-ip> to any port 8989 proto tcp` (SSH client IP or `REDGRES_BOOTSTRAP_ALLOW_FROM`; never `ufw allow 8989/tcp` to the world). Confirm-reachable and the bootstrap TTL write `bootstrap.closed` next to SQLite so a systemd restart does not reopen `:8989`; confirm also rewrites `redgres.env` to empty `REDGRES_BOOTSTRAP_ADDRESS`, `REDGRES_COOKIE_SECURE=true`, and `https` `REDGRES_BASE_URL`. Inactive UFW cannot enforce the queued rule. Not production-ready without live UFW/cloud-firewall evidence.

## Stage contract

1. **Preflight** — root, supported OS/architecture, requested service versions, package/image availability, clock, DNS, RAM/disk/inodes, network, commands, port conflicts.
2. **Inventory** — record package/service versions, cluster identities, listeners, data directories, configs, DNS/TLS observations, and checksums without secrets. **This Partial (OPS-002):** `--non-interactive --dry-run` only; scan the caller PATH captured as data for `postgres`, `redis-server`, and `pgbouncer`, require absolute search entries and an absolute executable regular-file first match, reject a symlink or group/world-writable binary, and execute only that validated absolute path when the component’s mode is existing. When the installer runs as root, the binary and every containing directory must be root-owned and not group/world writable, so a PATH-shadowed binary under an untrusted directory is never executed. Non-root inventory permits root/EUID ownership with the same no-symlink/no-write ancestor policy. The binary is re-validated immediately before `/usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C BINARY --version`. Invocation is `--version` only (no `-D`/`PGDATA`, no Redis conf, no PgBouncer ini; not `psql -V`, not `redis-cli`); skip fresh/disabled without invoking the binary; empty/relative search entry, missing/untrusted first match, non-zero, empty, unparseable, unsupported, or expected-version mismatch fails closed (exit 1). SQL `SHOW`, Redis `INFO`, PgBouncer `SHOW VERSION`, cluster identity, listeners, data directories, bounded probe execution, and backup evidence are deferred. Inventory does not run without `--dry-run`. `--config` is never sourced.
3. **Safety gate** — in existing mode, require a fresh verified backup manifest before PostgreSQL/Redis configuration changes.
4. **Packages** — install approved repositories and exact release-pinned packages/images idempotently; record container digests. **This Partial (OPS-001 live fresh):** without `--dry-run`, `fresh-postgres` + `fresh` Redis on Ubuntu 24.04 (noble) and 26.04 (resolute) resolves each package's apt `Candidate` and installs `pkg=version` (`postgresql-17|18`, `docker.io`, `docker-compose-v2`, optional `pgbouncer`; not the `postgresql` metapackage). Already-installed packages are left in place (no `apt upgrade`). Redis images remain digest-pinned. This is host-suite pin-at-resolve, not a COMPATIBILITY.md §6 freeze of one patch across Ubuntu releases. Other Ubuntu codenames fail closed. Existing-mode live install still exits 2.
5. **Identity/filesystem** — create `redgres` system user/group and exact FHS paths/modes.
6. **Redis** — detect or install the selected supported series, validate capabilities, Compose and persistent mounts; never replace a named volume silently.
7. **PostgreSQL/PgBouncer** — apply independent existing/fresh lifecycles; fresh bootstrap only for the selected supported major; existing mode defaults to preserve; validate an optional extension plan, exact packages, preload merge and restart impact under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). **This Partial (OPS-001 live fresh):** after `pg_createcluster --start` and Redis Compose `up -d`, the live path waits for the `redgres-redis` container to be running, `pg_isready -h 127.0.0.1 -p 5432`, and a Redis `PING`→`PONG` (AUTH on redis-cli stdin; password not logged). Host `redis.conf` is `0600` uid/gid `999` with Compose `SKIP_FIX_PERMS=1` so the official image can read the `:ro` mount (root-owned `0600` is `Permission denied` inside the container). On Redis health timeout the installer prints container status and secret-safe logs. Hosts under 2GiB RAM also get `maxmemory 64mb`. That is not `verify` Complete and not SQL `SHOW` / Redis `INFO`.
8. **TLS/DNS** — issue/validate raw-service certificates and renewal hooks; configure declared DNS records/routes only. **Dry-run Partial:** deferred to runtime Domain wizard; `deploy/lib/tls_inventory.sh` PATH-scans certbot presence only.
9. **Application release** — verify artifact checksum, install immutable release, migrate SQLite, configure systemd credentials/unit. **This Partial (OPS-005):** dry-run paths print skip matrices (`result=partial`) after adjacent `SHA256SUMS` verification (release unread beyond checksum). Live `update`/`rollback` without `--dry-run` install application binaries under `/opt/redgres` (or `REDGRES_OPT_ROOT`): checksum → extract → `releases/<version>` (`0755`, because installer `umask 077` would otherwise leave `0700` dirs that `User=redgres` cannot exec) → `current` → unit → systemd when managing `/opt/redgres` → healthz. Live `fresh-postgres` install also downloads GitHub `releases/latest` (tarball + `SHA256SUMS`) into `/var/lib/redgres-release` (`0700` root:root; not `/tmp`, whose world-writable ancestors fail the trusted-path check), verifies the adjacent checksum, and applies the same update path. Before that it enables loopback PostgreSQL TLS (snakeoil), creates `redgres_admin`, and writes password/URL files referenced from `redgres.env` (never logged) so production `serve` has a complete admin connection. Then TTY-safe owner bootstrap (`create-owner --generate --password-fifo`) and a boxed finish report (bootstrap URL, selected/package versions, loopback listeners, UFW on/off, owner username). The owner password is printed once in that box on a TTY and is never written to installer logs. Redis/Postgres secrets stay in `/etc/redgres` files. Signature/provenance unverified. Never reverse PostgreSQL/Redis/vault/credentials/DNS/schema. This Partial is not Complete.
10. **Cloudflare** — install/validate cloudflared and routes; token stays protected. **Dry-run Partial:** deferred to runtime Domain wizard; `deploy/lib/cloudflare_inventory.sh` PATH-scans `cloudflared` only.
11. **Firewall** — calculate intended rules, preserve SSH access, apply, then verify listeners externally and locally.
12. **End-to-end verify** — run [TESTING.md](TESTING.md) deployment checks. **This Partial (OPS-003):** `verify --non-interactive --dry-run --config PATH` only; PATH must pass the trusted regular-file policy and is never sourced, evaluated, or printed. Prints an explicit skip matrix (`result=partial`) for DNS, Cloudflare Tunnel/Access/routes, public TLS, GET `/api/v1/healthz` (curl not invoked), GET `/api/v1/status` auth boundaries, live bindings (intended redgres `127.0.0.1:8790`), cluster SHOW/INFO, and backup prerequisites (no named backup keys; OPS-004). Does not call inventory, curl, wget, cloudflared, or certbot. Live verify without `--dry-run` exits 2. Missing `--non-interactive`, missing/untrusted `--config`, or unknown flags (including `--mode`) exit 1. DNS/Cloudflare/public TLS remain skipped; this Partial is not Complete.
13. **Report** — produce a boxed redacted recap: source-restricted bootstrap UI URL ([ADR-012](decisions/ADR-012-ui-bootstrap.md)), selected/package versions, loopback listeners (API `127.0.0.1:8790`, PostgreSQL `127.0.0.1:5432`, Redis `127.0.0.1:6380`), UFW on/off plus the 8989 allow-from note, and the owner username. Fresh PgBouncer is reported as package-installed with listen not configured (do not claim `6432` until that lifecycle exists). The owner password may appear once in that box on a TTY. Never emit Redis, PostgreSQL, vault, or file-stored credentials.

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

## Uninstall

`uninstall.sh` (full purge, not `--app-only`) must finish without an interactive apt/needrestart prompt and without leaving the operator in a deleted working directory.

- Stop PostgreSQL clusters with `pg_ctlcluster … stop -m fast` (then immediate) before `pg_dropcluster`.
- Delete leftover `/etc/postgresql`, `/etc/postgresql-common`, `/var/log/postgresql`, and `/var/lib/postgresql` so dpkg does not warn that those directories are not empty.
- `cd /` before deleting a git checkout of this repository and before leftover `apt-get autoremove`. Maintainer scripts call `getcwd()`; a deleted cwd looks like a hang.
- Export `DEBIAN_FRONTEND=noninteractive`, `NEEDRESTART_SUSPEND=1`, `NEEDRESTART_MODE=l`, and `APT_LISTCHANGES_FRONTEND=none`, and pass `-o Dpkg::Use-Pty=0`. Uninstall reconnects stdin to `/dev/tty`, so needrestart would otherwise prompt.

This Partial is not live-host Complete.

## Rollback limits

The installer automatically rolls back a failed application symlink/unit deployment. It may restore a configuration file it changed after validation. It does not automatically undo package upgrades, SQL migrations, PostgreSQL roles/databases, Redis ACL mutations, credential rotations, DNS propagation, or production data.

**This Partial (OPS-005):** `rollback --non-interactive --dry-run --to VERSION` prints `data_reversal: skipped` and does not switch `current`. Live rollback without `--dry-run` switches `current` to an existing `releases/<VERSION>` and rewrites the unit; it never reverses PostgreSQL/Redis/vault/credentials/DNS/schema automatically. This Partial is not Complete.

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
