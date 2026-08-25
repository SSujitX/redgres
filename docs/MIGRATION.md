# Migration and cutover plan

The migration preserves working services until Redgres proves parity. It does not combine source directories or deploy an untested rewrite directly to production.

## Phase 0 — establish baselines and close source-critical gaps

Deliverables:

- Pin both source snapshots/provenance in [SOURCE_BASELINE.md](SOURCE_BASELINE.md).
- Characterize both APIs, UI workflows, PostgreSQL SQL behavior, Redis command sets, and vault vectors.
- Correct or supersede with tests:
  - Redis credential responses missing no-store.
  - Redis custom-command deny-list replaced by explicit allow-list.
  - PostgreSQL drop protected database set and self-backend exclusion.
- Record live VPS/DNS/TLS/service inventory separately from source assumptions.

Gate: source tests pass and compatibility fixtures are immutable.

## Phase 1 — stabilize current production topology

- Detect the existing PostgreSQL/Redis/PgBouncer versions and verify that they are in [COMPATIBILITY.md](COMPATIBILITY.md); do not upgrade a service implicitly.
- Inventory existing PostgreSQL packages, available/installed extensions per database, preload libraries and restart constraints. Use `preserve` policy unless an operator approves a plan under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
- Deploy/verify the selected supported PostgreSQL + PgBouncer host-native and Redis with persistent Docker volumes and TLS/ACL.
- Keep FastAPI on loopback 6969 and Redact on loopback 8787.
- Publish legacy browser hosts through Cloudflare Tunnel + Access.
- Complete PostgreSQL, Redis, and SQLite/runtime backup procedures and one isolated restore.
- Rehearse PostgreSQL and PgBouncer existing/fresh paths independently and prove that extension package/preload/per-database state can be restored from the release manifest.

Gate: current system is recoverable before unification starts.

## Phase 2 — create Redgres foundation

- Establish Git repository/public module decision.
- Import only reviewed Go/React source from the Redis baseline; rename Redact identities to Redgres through reviewed commits.
- Implement configuration, auth, session, audit, SQLite migrations, embedded UI, status, versioned API, and secret redaction.
- Apply no-store and explicit Redis allow-list; add real Redis integration tests.
- Run Redgres on `127.0.0.1:8790` with `console.onelifeltd.xyz` in staging/Access.

Gate: Redis parity and all platform security tests pass; legacy Redact remains available.

## Phase 3 — port PostgreSQL read-only capabilities

- Add `pgxpool` direct admin adapter.
- Port list/details/security/table/row-browse and masked URL metadata.
- Compare API outputs against a safe clone of the current PostgreSQL cluster.
- Do not reveal or mutate vault credentials yet. Masked `GET /api/v1/postgres/databases/{db}/connection` is PG-004/PG-005 Partial (existence + masked URLs only). POST reveal remains Phase 5.

Gate: read-only parity, query bounds, permissions, and failure isolation pass.

## Phase 4 — prove vault compatibility

- Implement exact Fernet/KDF compatibility.
- Pass Python↔Go vectors and copied production ciphertext read-only dry run.
- Add explicit vault health and wrong-key diagnostics that reveal no secret.
- Back up vault and legacy secret independently.

Gate: every sampled/copied record decrypts correctly and no record is mutated.

## Phase 5 — port PostgreSQL mutations

Order from lowest to highest risk:

1. Create role/database and one-time URLs.
2. Reveal existing connection URL.
3. Rotate password with recoverable operation state.
4. Duplicate database with source-isolation tests.
5. Selected row delete.
6. Truncate.
7. Drop database/conditional role cleanup.

Destructive features remain disabled until their individual integration, backup, reauth, and protected-resource gates pass.

Gate: PRD parity matrix complete; fault-injection proves compensation/reporting.

## Phase 6 — staging and shadow observation

- Deploy release candidate alongside both legacy apps.
- Use a cloned/staging PostgreSQL database and dedicated Redis test ACL users for mutations.
- Compare inventories/status; do not double-execute mutations against production.
- Run load, browser, security, backup, restore, update, rollback, certificate, and tunnel tests.
- Train operator using only Redgres docs.

Gate: signed acceptance checklist and no unresolved critical/high issue.

## Phase 7 — controlled cutover

1. Announce change window; capture fresh verified backups.
2. Temporarily pause control-plane mutations in legacy apps.
3. Verify Redgres release/checksum/config/vault on loopback.
4. Publish/confirm `console.onelifeltd.xyz` Access route.
5. Execute canary reads and one approved low-risk provisioning workflow.
6. Observe errors/audit/dependencies.
7. Keep legacy hostnames available to operators as read-only/fallback where feasible.

Rollback changes the UI route/release; it does not undo successful credentials or data changes.

## Phase 8 — observation and retirement

Suggested minimum observation: 14 days and at least one backup/restore cycle, one credential rotation, and representative PostgreSQL/Redis provisioning.

Retire legacy apps only when:

- no missing workflows/parity defects;
- vault and backups are independently recoverable;
- operator accepts Redgres runbooks;
- legacy runtime artifacts/config/secrets are archived securely or removed through an approved recovery-aware plan;
- Cloudflare routes and loopback listeners are cleaned up deliberately.

Do not delete either source repository. Preserve history for audit and migration evidence.

## Rollback triggers

- Authentication/session regression.
- Secret appearing in logs/cache/audit/browser persistence.
- Vault decrypt mismatch or unexplained credential failure.
- Protected-resource bypass.
- Incorrect Redis ACL expansion or application breakage.
- Unrecoverable operation state or data integrity concern.
- Sustained dependency/error rate outside accepted threshold.

Any credential/data mutation already completed is handled as an incident/change record, not blindly reversed.
