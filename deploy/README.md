# Deployment automation

OPS-001 / OPS-002 / OPS-003 / OPS-005 / OPS-006 Partial: `install.sh` is a **fail-closed dispatcher**. `--dry-run --non-interactive` prints the planned stage list and inventories PATH host `--version` for existing PostgreSQL, Redis, and PgBouncer. `verify --non-interactive --dry-run --config PATH` prints a skip matrix (`result=partial`); it never sources `--config`, never mutates, and does not probe DNS, Cloudflare, public TLS, or HTTP health. `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` print skip matrices (`result=partial`); they never source/extract the release, never switch `current`, never write `/opt/redgres`, and do not probe healthz. Fresh/disabled modes skip live detection. SQL `SHOW` / Redis `INFO` / PgBouncer `SHOW VERSION` are deferred.

```bash
bash deploy/tests/run.sh
bash deploy/install.sh --help
bash deploy/install.sh --non-interactive --dry-run \
  --mode existing-postgres --expect-postgres-major 17 \
  --redis-mode existing --expect-redis-series 8.2 \
  --pgbouncer-mode existing
bash deploy/install.sh verify --non-interactive --dry-run --config PATH
bash deploy/install.sh update --non-interactive --dry-run --release PATH
bash deploy/install.sh rollback --non-interactive --dry-run --to VERSION
```

`verify` without `--dry-run` exits 2 (live verify not implemented). `update`/`rollback` without `--dry-run` exit 2 (live mutation not implemented; no inventory, no extract). `backup` and mutation install without `--dry-run` exit 2 (not implemented; no inventory). `--config` is recognized on install/verify and never sourced; it is an unknown flag on update/rollback. DNS/Cloudflare/public TLS remain skipped; verify is not Complete. Update/rollback skip matrices are not Complete.

Do not paste legacy A-to-Z runbooks here. Exact package/image mutation pins, Cloudflare, DNS, and live Ubuntu rehearsal are later slices.
