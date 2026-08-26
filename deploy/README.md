# Deployment automation

OPS-001 / OPS-002 / OPS-003 / OPS-004 / OPS-005 / OPS-006 / OPS-007 Partial: `install.sh` is a **fail-closed dispatcher**. Execute it directly so the fixed `/bin/bash -p` bootstrap is applied; plain `bash deploy/install.sh` is rejected. Before sourcing modules it installs a fixed runtime PATH, validates and descriptor-pins the trusted source tree, and treats the caller PATH only as host-inventory search data. `--dry-run --non-interactive` prints the planned stage list and inventories host `--version` for existing PostgreSQL, Redis, and PgBouncer through a minimal environment. Operator file inputs must be absolute trusted regular files with no symlink or writable/untrusted ancestry; extension-plan parsing uses one identity-pinned bounded snapshot. `verify --non-interactive --dry-run --config PATH` prints a skip matrix (`result=partial`); it never sources `--config`, never mutates, and does not probe DNS, Cloudflare, public TLS, or HTTP health. `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` print skip matrices (`result=partial`); they never source/extract the release, never switch `current`, never write `/opt/redgres`, and do not probe healthz. `postgres-plan --config PATH --extension-plan PATH` is a read-only extension-plan validator: it checks policy, capability IDs, explicit non-empty databases, and scheduler rules against the release-owned registry, prints a plan preview and a skip matrix (`result=partial`), and never sources `--config` or mutates. Fresh/disabled modes skip live detection. SQL `SHOW` / Redis `INFO` / PgBouncer `SHOW VERSION` are deferred.

```bash
bash deploy/tests/run.sh
./deploy/install.sh --help
./deploy/install.sh --non-interactive --dry-run \
  --mode existing-postgres --expect-postgres-major 17 \
  --redis-mode existing --expect-redis-series 8.2 \
  --pgbouncer-mode existing
./deploy/install.sh verify --non-interactive --dry-run --config /absolute/PATH
./deploy/install.sh update --non-interactive --dry-run --release /absolute/PATH
./deploy/install.sh rollback --non-interactive --dry-run --to VERSION
./deploy/install.sh backup --non-interactive --dry-run --config /absolute/PATH
./deploy/install.sh postgres-plan --config /absolute/PATH --extension-plan /absolute/PATH
./deploy/install.sh postgres-extensions apply --non-interactive --dry-run --config /absolute/PATH --extension-plan /absolute/PATH
```

`verify` without `--dry-run` exits 2 (live verify not implemented). `update`/`rollback` without `--dry-run` exit 2 (live mutation not implemented; no inventory, no extract). `backup` without `--dry-run` exits 2 (live backup is installer-recovery; no dump/snapshot/checksum/off-host). `postgres-extensions` and mutation install without `--dry-run` exit 2 (not implemented; no inventory). `backup --non-interactive --dry-run --config PATH` prints a skip matrix (`result=partial`); it never sources `--config`, never invokes pg_dump/pg_restore/BGSAVE/SQLite backup, and never mutates. `postgres-plan` is read-only and requires trusted absolute regular `--config` and `--extension-plan` files (`--config` is never sourced); invalid plans, unknown capabilities, protected/empty databases, and scheduler misuse exit 1. `postgres-extensions apply --non-interactive --dry-run --config PATH --extension-plan PATH` validates the plan and prints an apply skip matrix (`result=partial`); live apply without `--dry-run` exits 2. A main install `--dry-run` also validates an optional `--extension-plan` with the same rules when supplied, and an optional `--config` must pass the same trust policy (never sourced). `--config` is recognized on install/verify and never sourced; it is an unknown flag on update/rollback. DNS/Cloudflare/public TLS remain skipped; verify is not Complete. Update/rollback skip matrices are not Complete. postgres-plan is not Complete. backup skip matrix is not Complete.

Do not paste legacy A-to-Z runbooks here. Exact package/image mutation pins, Cloudflare, DNS, and live Ubuntu rehearsal are later slices.
