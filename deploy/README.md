# Deployment automation

OPS-001 / OPS-002 / OPS-006 Partial: `install.sh` is a **fail-closed dispatcher**. `--dry-run --non-interactive` prints the planned stage list and inventories PATH host `--version` for existing PostgreSQL, Redis, and PgBouncer. It does not mutate the host, start servers, or source `--config`. Fresh/disabled modes skip live detection. SQL `SHOW` / Redis `INFO` / PgBouncer `SHOW VERSION` are deferred.

```bash
bash deploy/tests/run.sh
bash deploy/install.sh --help
bash deploy/install.sh --non-interactive --dry-run \
  --mode existing-postgres --expect-postgres-major 17 \
  --redis-mode existing --expect-redis-series 8.2 \
  --pgbouncer-mode existing
```

`verify`, `backup`, `update`, `rollback`, and mutation install without `--dry-run` exit 2 (not implemented; no inventory). `--config` is recognized and never sourced.

Do not paste legacy A-to-Z runbooks here. Exact package/image mutation pins, Cloudflare, DNS, and live Ubuntu rehearsal are later slices.
