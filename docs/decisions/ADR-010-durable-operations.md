# ADR-010: Durable operations ledger

Status: Accepted
Date: 2026-08-26

## Context

PG-010 duplicate is in-request **201**. `CREATE DATABASE … TEMPLATE` cannot run inside a transaction and can exceed the 30s HTTP bound. User freeze: persist non-secret operations; long duplicate returns **202** + operation ID; restart reconciles interrupted work. Sibling `database-app` has no 202 ledger to copy.

## Decision

SQLite tables `operations` and `operation_locks` in `migrations/002_operations.sql`. IDs are 32 lowercase hex from `crypto/rand` (same construction as HTTP `request_id`). Do not add `github.com/google/uuid`.

This slice persists **only** `postgres.database.duplicate`. Other action names may exist on the type; no workers are added here.

### HTTP

- Duplicate **target** after enqueue is **always 202** `{ "operation": { "id", "status": "queued" }, "request_id" }`. Never wait for TEMPLATE. Never return a credential on that POST.
- Implemented duplicate POST is **202 after InsertQueued**. Worker `postgresadmin.RunQueuedDuplicates` runs under `operations.MaxRuntime` (15m). `cmd/redgres` Open → `Reconcile(live Probe, Compensator)` → 1s `ListQueued` poller.
- Persist intended `result_json` `{database,owner,source}` at `queued`. GET still omits `result` unless `succeeded`.
- After `succeeded`, the operator uses existing Reveal. GET operations never includes passwords or credential URLs.
- `GET /api/v1/operations/{id}` is session + `platform.read`, no CSRF. No collection GET, no cancel endpoint. Invalid id → 400; missing → 404; SQLite down → 503. `Cache-Control: no-store`.
- AUTH-006 stays in-handler owner password. This ADR does not add `POST /api/v1/auth/reauth` or a reauth cookie/TTL.

### State machine

`queued` → `running` → (`succeeded` | `failed` | `compensating` | `interrupted`).

`compensating` → (`failed` | `indeterminate`).

`interrupted` → (`succeeded` | `failed` | `compensating` | `indeterminate`).

`queued` → `canceled` only.

Never `interrupted` → `running` to retry `CREATE DATABASE` (not idempotent). `indeterminate` is terminal.

Idempotent writes: same `(id, from, to)` is a no-op success. Illegal edges leave the row unchanged and return a typed error. Claim `queued`→`running` is one `UPDATE … WHERE status='queued'`.

Worker bound: constant **15m** (`operations.MaxRuntime`). No new env key. No heartbeat. Single instance (ADR-005): startup Reconcile flips every `running` → `interrupted`.

Retention: delete **terminal** rows older than **30d**; never delete non-terminal; cap **10 000** terminal by oldest `finished_at`.

### Locks

PK `(resource_kind, resource_name)`. Duplicate holds `postgres.database/<source>`, `postgres.database/<new>`, `postgres.role/<new_owner>`. Insert locks in the same transaction as `queued`. Terminal status deletes locks in the same transaction as the status update. Unique violation maps to `operation_in_progress` in backend-integration. This slice does not replace postgresadmin in-process TryLock maps.

Allow-list `resource_kind`: `postgres.database`, `postgres.role`, `redis.user`.

### Restart Reconcile

1. `running` → `interrupted`
2. Resume `compensating` (compensation only). Non-nil `Compensator` drops leftover clone/role/vault **before** terminal + lock release. Compensator error → `indeterminate` with `KeepLocks` (locks kept). Nil Compensator keeps fail-and-release (allowed only before 202).
3. For each `interrupted` duplicate, a live `Probe` inspects clone/role/vault existence (`SavedRoleNames`, no decrypt).
4. Probe: nothing created → `failed`; clone+role+vault complete → `succeeded`; partial → `compensating` then Compensator; cannot tell → `indeterminate`
5. `queued` stays queued. `ListQueued` is the dispatcher seam (1s in-process poller after Open + live Reconcile).
6. Prune terminal rows

`Reconcile(ctx, probe, compensator, now)`. Do not ship 202 with a nil Probe.

### Secrets

Forbidden in SQLite `result_json` / `error_json` and in GET JSON: password, credential URL, ciphertext, session/CSRF/tunnel tokens, private keys, raw `err.Error()`, SQL with values. Duplicate result keys are exactly `database`, `owner`, `source`.

## Consequences

- GET existed before duplicate became 202.
- Duplicate POST is 202 after enqueue. Crash during TEMPLATE cannot safely retry CREATE DATABASE; interrupted work is inspected, not blindly retried.
- Multi-instance HA still requires a future ADR (leases/heartbeats).
- Audit `postgres.database.duplicate` stays in the worker (backend-integration), fail-closed after cluster+vault. Optional metadata key `operation_id`. Audit failure after worker success keeps `succeeded` and does not compensate a complete clone.
