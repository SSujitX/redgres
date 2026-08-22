# Production acceptance checklist

This is a sign-off record, not a substitute for automated evidence. Every checked item must link to a report, CI run, commit, manifest, or operator record.

## Product parity

- [ ] Source baselines/provenance pinned.
- [ ] AUTH, PLAT, PG, REDIS, OPS, and NFR rows in `TRACEABILITY.md` have implementation/test evidence.
- [ ] PostgreSQL direct/pooled URL and project-role behavior accepted.
- [ ] Redis preset workloads and ACL isolation accepted.
- [ ] pgAdmin and RedisInsight remain correctly separated expert tools.

## Security

- [ ] External scan sees only approved public ports.
- [ ] UI origins bind loopback only and require Cloudflare Access plus Redgres auth.
- [ ] Credential responses are POST/no-store and absent from browser persistence/logs/audit/SQLite.
- [ ] Protected PostgreSQL and Redis resources reject every mutation.
- [ ] CSRF/origin/session/reauth/rate-limit suites pass.
- [ ] Dependency, secret, vulnerability, and SBOM checks pass or have approved exceptions.

## Data and recovery

- [ ] Legacy Fernet fixture and copied-record tests pass without source mutation.
- [ ] Current PostgreSQL/Redis/SQLite backup manifest and off-host copy verified.
- [ ] Isolated restore completed and report approved.
- [ ] Cross-store credential-rotation failure recovery tested.

## Deployment

- [ ] Fresh and existing-postgres installer modes rehearsed.
- [ ] Second-run idempotency and interrupted-run recovery pass.
- [ ] Application update and schema-compatible rollback pass.
- [ ] DNS, TLS, renewal hooks, tunnel routes, Access policies, listeners, and firewall observed live.
- [ ] Monitoring/alert ownership and incident contacts configured.

## Cutover and retirement

- [ ] Change window and rollback owner approved.
- [ ] Canary and observation window completed.
- [ ] Legacy fallbacks tested during observation.
- [ ] No unresolved critical/high defects.
- [ ] Legacy retirement plan preserves source/history and handles runtime secrets/artifacts safely.

Approvals:

| Role | Name | Date | Evidence |
|---|---|---|---|
| Product owner | TODO | TODO | TODO |
| Security reviewer | TODO | TODO | TODO |
| Operations owner | TODO | TODO | TODO |
| Release owner | TODO | TODO | TODO |
