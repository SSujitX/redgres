# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” The table is current status. Empty evidence means incomplete. Do not mark a row Complete from prose alone.

Slice-by-slice implementation, review, and verifier records through REDIS-004 evidence pin `5474720` remain in Git:

```bash
git show 5474720:docs/TRACEABILITY.md
```

New work appends **one** current-slice block at the bottom (requirement, files, commands actually run, limitations). Do not re-copy older slices or stack reviewer-only pins as full duplicates.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | AUTH-001–005 unit/HTTP/CLI tests; AUTH-005 Partial: fail-closed attempt persistence, loopback-only `CF-Connecting-IP` identity, reserve-before-Argon2, IP-wide spray only for non-loopback IPs; AUTH-006 Partial: in-handler `Reauthenticate` on `DELETE /api/v1/redis/users/{username}`, flagged `DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows`, flagged `POST /api/v1/postgres/databases/{db}/truncate`, and flagged `DELETE /api/v1/postgres/databases/{db}` (no `POST /api/v1/auth/reauth`; reauth failures persist in `login_attempts` under a reserved `client_ip` prefix, not the login stream) | Partial |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | `GET /api/v1/healthz`; authenticated `GET /api/v1/status` + Overview live cards (PLAT-001 Partial: Redis Ping + Overview metrics, PgBouncer `SHOW VERSION` Ping, optional tool-link session hrefs + status presence, no live matrix); PLAT-003 audit read API + history UI; PLAT-004 `GET /api/v1/search` + grouped palette (Partial: Redis ACL username hits, no docs corpus/deep links/command palette) | Partial |
| PG-001..012 | PRD, Source Systems, ADR-004 | `internal/postgresadmin`, `internal/secrets` | PG-001/002 unit+HTTP+UI; PG-007 table-list API+UI + row-browse API+UI; PG-008 Partial: GET `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` + flagged `DELETE …/rows` API (`postgres.destructive` + CSRF + AUTH-006) + inspector single-column PK checkboxes and danger Delete selected dialog (no live PG, no Playwright); PG-012 Partial: GET `/api/v1/postgres/security` cluster overview + Security overview page + vault existence (`missing_password_count` when ok) + `rotation_eligible` (diagnostic; POST rotate is PG-006); PG-005 Partial: in-process Fernet/KDF fixtures plus HTTP vault existence GET plus masked connection GET plus POST `/connection/reveal` (no Gate 4); PG-004 Partial: GET `/api/v1/postgres/databases/{db}/connection` masked URLs (no decrypt); PG-003 Partial: POST `/api/v1/postgres/databases` (`postgres.provision` + CSRF) + `secrets.Encrypt` + vault INSERT + compensation + Databases Create dialog + ticket-open nav/search guard + list GET 401 clears ticket (no live PG, no Gate 4); PG-006 Partial: POST `/api/v1/postgres/databases/{db}/credentials/rotate` (`postgres.credentials` + CSRF) + ALTER ROLE + vault upsert + inspector Rotate (no live PG, no Gate 4, no AUTH-006); PG-010 Partial: POST `/api/v1/postgres/databases/{db}/duplicate` (`postgres.provision` + CSRF) TEMPLATE clone + unique owner + vault INSERT + clone-only compensation + inspector Duplicate; clone transfer has no blanket grants/default-privilege expansion, preserves skipped-object ACLs, and uses transaction-scoped temporary membership; direct `deptype='e'` members are skipped, while subsidiary/internal extension behavior remains unproven (no live PG 17/18, no 202, no AUTH-006); PG-009 Partial: flagged `POST /api/v1/postgres/databases/{db}/truncate` (`postgres.destructive` + CSRF + AUTH-006, one `TRUNCATE … RESTART IDENTITY`) + inspector danger Truncate dialog (no live PG, no Playwright); PG-011 Partial: flagged `DELETE /api/v1/postgres/databases/{db}` (`postgres.destructive` + CSRF + AUTH-006, terminate excluding current backend then `DROP DATABASE`, optional `DROP ROLE` + vault DELETE; **BF-1** no backup HTTP gate) + inspector danger Drop dialog (no live PG, no Playwright) | Partial |
| REDIS-001..008 | PRD, Source Systems, ADR-006 | `internal/redisadmin` | REDIS-001 Partial: Ping on GET `/api/v1/status`; metrics + typed failures on GET `/api/v1/redis/status` + Overview; REDIS-002 Partial: ACL list/inspect GET + UI; REDIS-003/004 Partial: POST create `on` + named presets + GET `/api/v1/redis/presets` + Permission presets catalog page + one-time ticket; REDIS-005 Partial: custom PATCH + POST create custom through `AllowedCommands()` + GET `/api/v1/redis/commands` + Edit/Create Custom checklists (no categories); REDIS-006 Partial: PATCH named-preset prefix/grants (password preserved) + inspector Edit permissions; REDIS-007 Partial: POST enable/disable `on`/`off` plus rotate `resetpass` + `>password` and inspector UI; REDIS-008 Partial: `DELETE /api/v1/redis/users/{username}` (`ACL LIST` + one `ACL DELUSER`) + inspector Delete danger dialog (no live Redis, no Playwright, no CLIENT KILL, keys not deleted) | Partial |
| OPS-001..007 | Deployment, Installer, PostgreSQL Provisioning, Backup, Compatibility, ADR-008/009 | `deploy/`, `internal/platform` | OPS-001/006 Partial: fail-closed dispatcher + `--dry-run` stage print. OPS-002 Partial: PATH host `--version` inventory on `--dry-run` only (`deploy/lib/inventory.sh`); `db20ff6`/`cb7be09` reject `%` SQLite paths, open a filesystem DSN (no `file:` URI), walk ancestor directories with Lstat, open production state under a verified `/var/lib/redgres` jail, reject `O_TRUNC`, re-check sidecars before driver open, and validate non-root inventory ancestry (parent Git Bash `deploy/tests/run.sh` 66 passed; Linux symlink/mode/root jail skipped on Windows). Security Approve Partial and verifier Approve Partial / reject Complete on `203eae6`. OPS-003 Partial: fail-closed `verify --non-interactive --dry-run --config PATH` skip matrix (`result=partial`; never sources config). OPS-005 Partial: fail-closed `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` skip matrices (`result=partial`). OPS-004/007 Planned (`backup` still exit 2). No packages/host mutation. | Partial |
| NFR-001..012 | PRD, Architecture, Testing, Compatibility, UI Design System | cross-cutting | Wave 0 pins, headers, WAL, CGO-free build local; race/cross-compile CI-only | Partial |

## Per-feature completion template

```text
Requirement:
Decision/ADR:
Implementation files:
Unit tests:
Known limitations:
Do not mark Complete.
```

## Current slice

### OPS-005 update/rollback skip matrices (2026-08-26)

Partial. `deploy/install.sh update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` print skip matrices (`result=partial`). Live without `--dry-run` exit 2. `backup` still exit 2. Git Bash `deploy/tests/run.sh` 63 passed on `4a8fa4d`. Security/evidence/verifier Approve Partial on `8bbbdbe` / `43353a7`. Not Complete.

### REDIS-004 Permission presets catalog UI (2026-08-26)

Partial. Nested Permission presets consumes GET `/api/v1/redis/presets` (no CSRF, no Redis). HTTP unchanged. Parent `npm --prefix web run test:run` 363 passed on `c48fac0`. UI/security/evidence Approve Partial on `fa733ea` / `89ea7e2`. Independent verifier Approve Partial / reject Complete on `5474720` (this-run `npm --prefix web run test:run` 363 passed; `npm --prefix web run build` succeeded). Not viewport/live Redis/§6. Not Complete.

### PG-010 duplicate privilege containment (2026-08-26)

Partial. `db40b10` removes blanket grants/default-privilege expansion, preserves skipped-object ACLs, and makes temporary transfer membership transaction-scoped; `3f14e6a` limits extension claims to direct `pg_depend.deptype='e'` membership. Parent `go test -count=1 ./...` and focused `go test -count=1 ./internal/postgresadmin` passed. Independent security review approved Partial and confirmed its prior Low claim-overstatement finding resolved on `3f14e6a`. Subsidiary/internal extension behavior requires disposable PostgreSQL 17/18 evidence. Not Complete.

### Security containment — filesystem/process handling (2026-08-26)

Partial. `43a50c0` confines production SQLite lexically under `/var/lib/redgres`, rejects final-component symlink/non-regular SQLite and secret files, and executes only validated absolute inventory binaries. Parent `go test -count=1 ./...`, `go vet ./...`, and Git Bash `deploy/tests/run.sh` 65 passed. Independent security review withheld approval: High SQLite `file:` URI percent-decoding (`%2f`) can open outside the secured path; Medium ancestor-directory symlinks can escape `OpenRoot(filepath.Dir(path))`. Independent verifier FAIL for merge/completion on the ancestor-symlink gap plus missing this evidence pin. Linux symlink/mode/root inventory assertions remain unexecuted locally. Not Complete.

### Security containment — sqlite URI and ancestor confinement (2026-08-26)

Partial. `db20ff6` (writer `07a93cd`) rejects `%`/`?`/`#`/NUL in SQLite paths without echoing them, opens `modernc.org/sqlite` v1.57.0 with a filesystem path (no concatenated `file:` URI, no `_pragma` query parameters), sets WAL/foreign_keys/busy_timeout via post-open PRAGMA, walks ancestor directories with Lstat before OpenRoot/mkdir, opens production state relative to a verified `/var/lib/redgres` jail, rejects OpenRegular `O_TRUNC`, and validates non-root inventory ancestry with re-validate before `--version`. Parent `cb7be09` restores sidecar regular-file checks before the driver open. Focused parent `go test -count=1 ./internal/database ./internal/securefile ./internal/config` passed; parent `go test -count=1 ./...` passed; parent Git Bash `deploy/tests/run.sh` 66 passed. Independent security review Approve Partial on `203eae6` (prior High URI percent-decode and Medium ancestor-symlink resolved; no new Critical/High/Medium). Independent verifier Approve Partial / reject Complete on `203eae6`. Windows skipped symlink tests and Unix-mode writable-parent inventory; `/var/lib/redgres` Root jail was not exercised. Residual: `database.Open` does not reject an operator `file:` prefix on development CLI paths that skip `config.Load`. Secret-file `0640` vs systemd credentials remains an unfrozen contract. Not Complete.

### AUTH-005 / AUTH-006 / PLAT-002 containment follow-up (2026-08-26)

Partial. Do not treat `d5246c7` (`eb217ad`) as the finished slice. Follow-up `bc08fef` (`902b8cb`) uses loopback-only `CF-Connecting-IP` for lockout/audit identity, skips IP-wide spray when identity is still loopback, reserves failures before Argon2id, serializes hashing in-process to 1, persists login/reauth success atomically, keeps empty-body reveal/rotate out of `decodeJSON`, and rejects nested secrets under allowed audit keys. AUTH-006 reauth failures persist in `login_attempts` under a reserved `client_ip` prefix. Parent `go test -count=1 ./internal/auth ./internal/audit ./internal/httpapi` passed. Independent verifier Approve Partial / reject Complete on `98a1720`. Security review of that pin is still outstanding. No `POST /api/v1/auth/reauth`. Not Playwright, not live PG/Redis, not COMPATIBILITY.md §6. Not Complete.
