# Installer and command-interface specification

The installer is one operator entry point backed by small testable modules. “One shell command” does not mean one unstructured shell file or one file containing every secret.

## Command interface

```bash
sudo ./deploy/install.sh --mode existing-postgres --config /root/redgres/install.env
sudo ./deploy/install.sh --mode fresh-postgres --config /root/redgres/install.env
sudo ./deploy/install.sh verify --config /root/redgres/install.env
sudo ./deploy/install.sh backup --config /root/redgres/install.env
sudo ./deploy/install.sh update --release /root/redgres/redgres_VERSION_linux_amd64.tar.gz
sudo ./deploy/install.sh rollback --to VERSION
```

`install.sh` dispatches to version-controlled modules. It must support `--dry-run` for safe checks where feasible and `--non-interactive` only when every required decision is explicit.

## Required configuration classes

- Deployment profile: public hostnames, loopback ports, release source/checksum.
- Existing services: PostgreSQL version/cluster, config paths, PgBouncer path, Redis Compose project/volume.
- Network: intended public listeners, allowed source CIDRs, SSH port.
- Cloudflare: zone/account/tunnel identifiers supplied explicitly; token file paths.
- TLS: certificate names, Certbot credentials file, deploy hooks.
- Backup: destination, retention, off-host target, encryption mechanism, restore-test target.
- Feature flags and protected databases/roles.

The installer does not guess zones, delete records, generate publicly trusted credentials, or print secrets.

## Stage contract

1. **Preflight** — root, supported OS/architecture, clock, DNS, RAM/disk/inodes, network, commands, port conflicts.
2. **Inventory** — record package/service versions, cluster identities, listeners, data directories, configs, DNS/TLS observations, and checksums without secrets.
3. **Safety gate** — in existing mode, require a fresh verified backup manifest before PostgreSQL/Redis configuration changes.
4. **Packages** — install/pin required repositories and packages idempotently.
5. **Identity/filesystem** — create `redgres` system user/group and exact FHS paths/modes.
6. **Redis** — create/validate Compose and persistent mounts; never replace a named volume silently.
7. **PostgreSQL/PgBouncer** — fresh bootstrap only in fresh mode; existing mode validates and proposes/minimally applies explicit changes.
8. **TLS/DNS** — issue/validate raw-service certificates and renewal hooks; configure declared DNS records/routes only.
9. **Application release** — verify artifact checksum, install immutable release, migrate SQLite, configure systemd credentials/unit.
10. **Cloudflare** — install/validate cloudflared and routes; token stays protected.
11. **Firewall** — calculate intended rules, preserve SSH access, apply, then verify listeners externally and locally.
12. **End-to-end verify** — run [TESTING.md](TESTING.md) deployment checks.
13. **Report** — produce a redacted manifest with changed/skipped items and exact follow-ups. Never emit credentials.

## Idempotency

Every step classifies state as `already-correct`, `change-required`, `blocked`, or `failed`. Re-running after interruption must converge without creating duplicate users, tunnels, DNS records, certificates, timers, or releases.

The script uses strict shell mode, explicit paths, traps, and temporary directories from `mktemp -d`; it never parses `ls`, evaluates config as shell, downloads and executes unverified scripts, or uses broad recursive deletion.

## Existing PostgreSQL protections

- Confirm cluster major version, cluster name, data directory, system identifier, port, and service unit.
- Refuse if expected data directory/system identifier changes during run.
- Never run `initdb`, `pg_dropcluster`, package purge, data-directory move, or destructive restore.
- Back up configuration and logical/global objects before modifications.
- Apply config fragments only after syntax checks; reload before restart when possible.
- Preserve local emergency access and validate a second SSH session before firewall changes.

## Fresh PostgreSQL protections

- Require explicit `fresh-postgres` mode.
- Refuse when a cluster/data directory already contains data unless a separate destructive workflow is approved outside the installer.
- Generate/administer credentials through protected files/stdin, not process arguments.
- Complete TLS/SCRAM/ACL baseline before public firewall rules open.

## Rollback limits

The installer automatically rolls back a failed application symlink/unit deployment. It may restore a configuration file it changed after validation. It does not automatically undo package upgrades, SQL migrations, PostgreSQL roles/databases, Redis ACL mutations, credential rotations, DNS propagation, or production data.

## Acceptance tests

- Two consecutive installs make no unintended second-run changes.
- Interrupted run at every stage safely resumes.
- Existing mode leaves PostgreSQL system identifier/data checksum unchanged.
- Port/firewall changes do not lock out the validation SSH path.
- Output and logs pass fake-secret scanning.
- Unavailable DNS/Cloudflare/apt/Docker dependencies fail with actionable state and safe retry.
