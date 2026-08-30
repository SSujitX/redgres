# Operations runbook

This runbook defines operator intent. Exact commands must be generated/validated by deployment automation and checked against the live host; never assume service names, paths, or ports without inventory.

## Daily automated checks

- Redgres, PostgreSQL, PgBouncer, Redis, cloudflared service health (`cloudflared-redgres.service` — see [CLOUDFLARED.md](CLOUDFLARED.md)).
- Loopback/public listener conformance.
- Backup completion, checksums, off-host transfer, disk capacity.
- Certificate remaining lifetime.
- PostgreSQL connection saturation and storage growth.
- Redis memory, eviction, persistence errors, fragmentation, and rejected connections.
- Failed Redgres logins and destructive/credential audit events.

## Domain wizard bootstrap (OPS-009 Partial)

- First-run console may listen on `:8989` until the operator completes Domain & Network steps ([ADR-012](decisions/ADR-012-ui-bootstrap.md)).
- **Confirm reachable** is operator attestation only; Redgres does not HTTP-probe the console hostname.
- On confirm, Redgres closes bootstrap gracefully and optionally runs `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD` (absolute path, no shell). If the helper fails, check UFW manually — the API returns `bootstrap_ufw_removed: false`.
- db/redis TLS: apply with an API token writes `REDGRES_CERTBOT_DNS_TOKEN_FILE` from that token and queues `REDGRES_TLS_ISSUE_REQUEST_FILE`. The installer enables `redgres-tls-issue.path` and a certbot renew deploy hook that stages copies under `/etc/ssl/redgres` and installs postgres-owned files in each `/etc/postgresql/*/main/` (reload re-reads as `postgres`; `0640` `root:ssl-cert` is not readable to that user here). PgBouncer gets its own owned copies under `/etc/pgbouncer/`. `POST /api/v1/domain/tls/issue` retries if the helper has not issued yet. Redis public TLS enablement is not part of this Partial (loopback `6380` stays plaintext).

## Weekly operator checks

- Review failed audit events and unexpected source IPs.
- Verify Cloudflare Access membership and tunnel health.
- Test direct and pooled PostgreSQL using a non-production test role.
- Test Redis TLS and a scoped test ACL user, including denied commands/key prefixes.
- Confirm package/container/release vulnerability status.
- Review disk/inode growth and backup retention.

## Monthly/quarterly

- Restore latest backup set into an isolated environment.
- Rotate/validate selected operational credentials according to policy.
- Review protected database/role/user lists.
- Rehearse application release rollback.
- Review operator access, SSH keys, Cloudflare tokens, and off-host backup access.
- Capacity-plan using actual PostgreSQL/Redis metrics.

## Release/update procedure

1. Read release notes and schema compatibility.
2. Capture/verify a current backup.
3. Run preflight and configuration diff.
4. Install immutable release and verify checksum/signature.
5. Run migrations and local health on staging/new port.
6. Run integration smoke tests.
7. Switch `current`/route according to migration state.
8. Monitor logs/audit/latency/errors through observation window.
9. Retain prior compatible release.

PostgreSQL/PgBouncer/extension packages are not part of an ordinary Redgres application update. Apply those only through a separately approved plan from [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md), with exact package and extension release notes, database backups, preload/config diff, capacity review, maintenance window and restore evidence.

## PostgreSQL capability change

1. Run the read-only `postgres-plan` command and confirm cluster/system identity did not change.
2. Review exact package source/version, named databases, extension owner/schema/version, preload merge, restart requirement, capacity and backup evidence.
3. Reject plans that mention unrequested databases, `template1`, arbitrary packages/SQL, extension upgrades/drops or an implicit PostgreSQL major upgrade.
4. If restart is required, approve a maintenance window and PgBouncer drain/pause; do not rely on application rollback to undo it.
5. Apply with the reviewed plan digest, verify direct PostgreSQL first, then PgBouncer and representative pooled clients.
6. Verify every named database and capability smoke query, capture the redacted report and rerun the plan to prove convergence.

Running pg_repack, creating TimescaleDB hypertables/jobs, configuring pg_partman/pg_cron jobs, or broadening pgAudit classes are separate operational/database changes after installation; package/extension availability does not authorize those actions.

## Application rollback

1. Confirm failure is application/configuration related, not data corruption or external dependency failure.
2. Verify prior release supports current SQLite schema.
3. Switch `current` to exact prior release, restore compatible config if required, restart.
4. Verify login, PostgreSQL read-only status, Redis read-only status, and audit.
5. Do not reverse credentials, PostgreSQL/Redis state, or schema automatically.
6. Record incident and rollback evidence.

## Certificate incident

- Confirm affected hostname/service and actual served chain.
- If expired/missing, keep raw endpoint restricted; renew via DNS-01 using least-privilege token.
- Validate name, dates, permissions, and service config before reload.
- Test with hostname verification from an external client.
- Rotate private key/token if exposure is suspected.

## Credential incidents

### Redgres owner

Reset only through local root/CLI workflow, revoke all sessions, review login/audit/tunnel logs, and rotate downstream administrator secrets if control-plane access may have occurred.

### PostgreSQL project

Identify all consumers, rotate through guarded workflow, update secret stores/deployments, verify direct+pooled clients, revoke old connections as policy allows, review audit/database logs.

### PostgreSQL administrator/vault secret

Contain Redgres service, preserve encrypted vault and evidence, rotate admin credential with least privilege, update protected file, verify. If legacy vault secret is exposed, plan versioned vault re-encryption; changing it alone makes old records unreadable.

### Redis project/admin

Project: rotate user, update consumers, verify old password rejected and prefix/command restrictions remain. Admin: stop/contain Redgres, create/verify replacement admin ACL safely, update protected file, revoke old admin, test all ACL operations.

### Cloudflare tokens

Revoke/rotate in Cloudflare, update only the matching secret file/service, verify tunnel or Certbot independently, inspect account audit logs. Tunnel and DNS tokens are separate blast radii.

## Database destructive action

Before any database drop/truncate: identify owner/consumers, capture a fresh targeted backup, verify checksum/restore feasibility, record ticket/approval, confirm exact target and protection policy, execute through guarded workflow, verify post-state and dependent services. A same-host snapshot alone is insufficient.

## Common diagnostic order

1. Is the process/container running?
2. Is it listening on the intended interface/port?
3. Is local health successful?
4. Is dependency auth/TLS successful from the service identity?
5. Is firewall/security group correct?
6. Is DNS resolving as intended (tunnel vs DNS-only)?
7. Is Cloudflare route/Access correct for HTTP?
8. Are certificate name/chain/dates correct for raw services?
9. Correlate request ID with redacted logs/audit.

Avoid “fixing” by opening ports, disabling TLS/Access/ACL, using the PostgreSQL superuser, or logging full connection strings.
