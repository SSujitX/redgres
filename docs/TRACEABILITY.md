# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” Add code/test links as work lands. Empty evidence means incomplete.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | AUTH-001–005 unit/HTTP/CLI tests; AUTH-006 Partial: in-handler `Reauthenticate` on `DELETE /api/v1/redis/users/{username}`, flagged `DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows`, flagged `POST /api/v1/postgres/databases/{db}/truncate`, and flagged `DELETE /api/v1/postgres/databases/{db}` (no `POST /api/v1/auth/reauth`, no AUTH-005 `login_attempts` increment) | Partial |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | `GET /api/v1/healthz`; authenticated `GET /api/v1/status` + Overview live cards (PLAT-001 Partial: Redis Ping + Overview metrics, PgBouncer `SHOW VERSION` Ping, optional tool-link session hrefs + status presence, no live matrix); PLAT-003 audit read API + history UI; PLAT-004 `GET /api/v1/search` + grouped palette (Partial: Redis ACL username hits, no docs corpus/deep links/command palette) | Partial |
| PG-001..012 | PRD, Source Systems, ADR-004 | `internal/postgresadmin`, `internal/secrets` | PG-001/002 unit+HTTP+UI; PG-007 table-list API+UI + row-browse API+UI; PG-008 Partial: GET `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` + flagged `DELETE …/rows` API (`postgres.destructive` + CSRF + AUTH-006) + inspector single-column PK checkboxes and danger Delete selected dialog (no live PG, no Playwright); PG-012 Partial: GET `/api/v1/postgres/security` cluster overview + Security overview page + vault existence (`missing_password_count` when ok) + `rotation_eligible` (diagnostic; POST rotate is PG-006); PG-005 Partial: in-process Fernet/KDF fixtures plus HTTP vault existence GET plus masked connection GET plus POST `/connection/reveal` (no Gate 4); PG-004 Partial: GET `/api/v1/postgres/databases/{db}/connection` masked URLs (no decrypt); PG-003 Partial: POST `/api/v1/postgres/databases` (`postgres.provision` + CSRF) + `secrets.Encrypt` + vault INSERT + compensation + Databases Create dialog + ticket-open nav/search guard + list GET 401 clears ticket (no live PG, no Gate 4); PG-006 Partial: POST `/api/v1/postgres/databases/{db}/credentials/rotate` (`postgres.credentials` + CSRF) + ALTER ROLE + vault upsert + inspector Rotate (no live PG, no Gate 4, no AUTH-006); PG-010 Partial: POST `/api/v1/postgres/databases/{db}/duplicate` (`postgres.provision` + CSRF) TEMPLATE clone + unique owner + vault INSERT + clone-only compensation + inspector Duplicate (no live PG, no 202, no AUTH-006); PG-009 Partial: flagged `POST /api/v1/postgres/databases/{db}/truncate` (`postgres.destructive` + CSRF + AUTH-006, one `TRUNCATE … RESTART IDENTITY`) + inspector danger Truncate dialog (no live PG, no Playwright); PG-011 Partial: flagged `DELETE /api/v1/postgres/databases/{db}` (`postgres.destructive` + CSRF + AUTH-006, terminate excluding current backend then `DROP DATABASE`, optional `DROP ROLE` + vault DELETE; **BF-1** no backup HTTP gate) + inspector danger Drop dialog (no live PG, no Playwright) | Partial |
| REDIS-001..008 | PRD, Source Systems, ADR-006 | `internal/redisadmin` | REDIS-001 Partial: Ping on GET `/api/v1/status`; metrics + typed failures on GET `/api/v1/redis/status` + Overview; REDIS-002 Partial: ACL list/inspect GET + UI; REDIS-003/004 Partial: POST create `on` + named presets + GET `/api/v1/redis/presets` + one-time ticket; REDIS-005 Partial: custom PATCH + POST create custom through `AllowedCommands()` + GET `/api/v1/redis/commands` + Edit/Create Custom checklists (no categories); REDIS-006 Partial: PATCH named-preset prefix/grants (password preserved) + inspector Edit permissions; REDIS-007 Partial: POST enable/disable `on`/`off` plus rotate `resetpass` + `>password` and inspector UI; REDIS-008 Partial: `DELETE /api/v1/redis/users/{username}` (`ACL LIST` + one `ACL DELUSER`) + inspector Delete danger dialog (no live Redis, no Playwright, no CLIENT KILL, keys not deleted) | Partial |
| OPS-001..007 | Deployment, Installer, PostgreSQL Provisioning, Backup, Compatibility, ADR-008/009 | `deploy/`, `internal/platform` | OPS-001/006 Partial: fail-closed dispatcher + `--dry-run` stage print. OPS-002 Partial: PATH host `--version` inventory on `--dry-run` only (`deploy/lib/inventory.sh`). OPS-003 Partial: fail-closed `verify --non-interactive --dry-run --config PATH` skip matrix (`result=partial`; never sources config). OPS-005 Partial: fail-closed `update --non-interactive --dry-run --release PATH` and `rollback --non-interactive --dry-run --to VERSION` skip matrices (`result=partial`; Git Bash `deploy/tests/run.sh` 63 passed). OPS-004/007 Planned (`backup` still exit 2). No packages/host mutation. | Partial |
| NFR-001..012 | PRD, Architecture, Testing, Compatibility, UI Design System | cross-cutting | Wave 0 pins, headers, WAL, CGO-free build local; race/cross-compile CI-only | Partial |

## Per-feature completion template

```text
Requirement:
Decision/ADR:
Source characterization:
Implementation files:
Unit tests:
Integration tests:
Security tests:
Deployment/migration impact:
Known limitations:
Reviewer/date:
```

Do not mark a row complete using a manual statement alone. Include reproducible test output/artifact or reviewed deployment evidence.

## Wave 0 foundation (2026-08-23)

```text
Requirement: PLAT-001 (partial: Redgres state DB only), NFR-006/007/008/009/010 (partial)
Decision/ADR: ADR-001, ADR-005 (migration mechanism), ADR-003
Source characterization: docs/SOURCE_BASELINE.md — structure-only influence from redis-ui; no source copied
Implementation files: cmd/redgres/main.go; internal/config/{config,dotenv}.go;
 internal/database/{database,migrations}.go; migrations/{embed.go,001_initial.sql};
 internal/httpapi/{server,middleware,errors,system_routes}.go; internal/web/embed.go;
 web/**; Makefile; .github/workflows/ci.yml
Unit tests: internal/config/{config,dotenv}_test.go; internal/database/{database,migrations}_test.go;
 internal/httpapi/server_test.go; internal/web/embed_test.go; web/src/App.test.tsx
Integration tests: none — no PostgreSQL/Redis code exists; the COMPATIBILITY.md §6 matrix is not applicable yet
Security tests: exact security-header assertions (SECURITY.md §7); production fail-closed config cases;
 healthz 503 asserted to leak no path/version/driver text
Deployment/migration impact: none deployed. Schema version 1 introduced. 001_initial.sql editable until first tag
Known limitations: no auth, audit, PostgreSQL, Redis, /api/v1/status, /api/v1/search, SBOM, installer;
 go test -race and linux cross-compiles are CI-only evidence; GitHub Actions have not run because nothing has been pushed;
 local frontend tests ran on Node v25.3.0 (unsupported); pinned Node 24.19.0 evidence is CI-only
Commands executed locally (2026-08-23):
  go version → go1.27.0 windows/amd64
  go test ./... → ok (config, database, httpapi, web)
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o redgres.exe ./cmd/redgres → success with and without web/dist/app
  CGO_ENABLED=0 go build ./cmd/redgres → success (verifier)
  web: npm run test:run → 3 passed; npm run build → TypeScript 7.0.2 + Vite 8.2.2
  production serve with development defaults → exit 1, "REDGRES_BASE_URL: production origin must use https"
  development serve → GET /api/v1/healthz 200 {"status":"ok","request_id":...} Cache-Control no-store
Local commit: `912b5a431413e90eb8641d0a994a5e533286ad6d` on `master` (not pushed).
Reviewer/date: UI reviewer approved Wave 0 scaffolding only (2026-08-23). Security review found no Critical; H1 (production flag still read .env) and H2 (SQLite umask) plus M1–M3 and L1–L6 were corrected in this change (`TestProductionFlagIgnoresDotEnv`, `.env.*` gitignore, 0700/0600 state files). Verifier approved merge to master (2026-08-23) with conditions: exclude `.env.local` (now ignored by `.env.*`); treat race, linux cross-compile, Node 24.19.0, gitleaks, and govulncheck as unproven until the first CI run. Verifier residual that `-environment production` still loads `.env` was stale relative to the post-security-review tree. No functional PRD ID is complete.
```

## Owner auth (2026-08-23)

```text
Requirement: AUTH-001..005; PLAT-002 (owner.login / owner.logout only)
Decision/ADR: ADR-001, ADR-005 hash-encoding amendment
Source characterization: redis-ui create-owner / internal/auth / login-logout-session; not copied. 15-code-point policy from NIST SP 800-63B-4 and OWASP Authentication Cheat Sheet (no MFA). Argon2id API from golang.org/x/crypto@v0.55.0 argon2.go (RFC 9106 second option).
Implementation files: cmd/redgres/{main,create_owner}.go; internal/auth/{password,owner,session,security}.go;
 internal/audit/audit.go; internal/httpapi/{auth_routes,json}.go; internal/config LoadDevelopmentDotEnv
Unit tests: internal/auth/*_test.go; internal/audit/audit_test.go; internal/httpapi/auth_routes_test.go;
 cmd/redgres/create_owner_test.go
Integration tests: none
Security tests: hash-at-rest, generic login errors, canary audit redaction, origin/CSRF, no --password flag
Deployment/migration impact: none deployed. 001_initial.sql unchanged (PHC bytes in existing BLOB).
Known limitations: AUTH-006 not implemented; first lockout is 1s (inherited redis-ui);
 RemoteAddr-only IP (tunnel clients may share 127.0.0.1); no pwned-password corpus;
 go test -race / CI / Node 24.19.0 still unproven
Commands executed locally (2026-08-23):
  go list -m golang.org/x/crypto golang.org/x/term → v0.55.0 / v0.45.0 (proxy.golang.org @latest)
  go test -count=1 ./... → ok (cmd/redgres, audit, auth, config, database, httpapi, web)
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
Reviewer/date: Security review (2026-08-23) found no Critical/High; M1/L1/L2 corrected. Verifier approved (2026-08-23) AUTH-001–005 and narrow PLAT-002; local commits `1b54b01` and `c3f72c0` (not pushed). Residual gaps are non-blocking (no HTTP assertion that success audit rows exist; first lockout 1s; race/CI unproven).
```

## Login and shell chrome (2026-08-23)

```text
Requirement: AUTH-003–005 (UI), NFR-004/012 (partial), PLAT-004 (navigation filter only)
Decision/ADR: ADR-001, ADR-003 (exact origin; Vite BaseURL documented, SameOrigin unchanged)
Source characterization: UI_DESIGN_SYSTEM.md / UX.md; redis-ui not copied
Implementation files: web/src/App.tsx; web/src/api/{client,auth}.ts; web/src/nav.ts;
 web/src/features/auth/LoginPage.tsx; web/src/features/pages/Placeholders.tsx;
 web/src/components/shell/AppShell.tsx; web/src/components/search/NavigationSearch.tsx;
 web/src/components/icons.tsx; web/src/hooks/useFocusTrap.ts;
 web/src/styles/{tokens,globals,login,shell}.css; web/index.html
Unit tests: web/src/App.test.tsx; web/src/nav.test.ts
Integration tests: none
Security tests: login never calls healthz; generic 401; 429 Retry-After; CSRF header on logout;
 password not left in the form; no style={{}}; login catch is generic (no control-plane text)
Deployment/migration impact: none. Vite login requires REDGRES_BASE_URL=http://127.0.0.1:5173
Known limitations: no URL routes; no /status; no server search; jsdom cannot prove viewports
 or 200% zoom; Node 25.x local frontend evidence; AUTH-006 / Redis still absent
Commands executed locally (2026-08-23):
  web: npm run test:run → 15 passed (App.test 13, nav.test 2); Node v25.3.0
  web: npm run build → TypeScript 7.0.2 + Vite 8.2.2; built index.html has
  viewport-fit=cover and external script/link only (no inline style/script)
Reviewer/date: UI reviewer rejected (2026-08-23) on H1/H2; remediations re-reviewed
 and approved (2026-08-23). Residual Medium (rail tooltip under workspace) corrected
 by stacking `.app-sidebar` above `.app-main` at 768–1023. Not viewport sign-off:
 360/768/1280/1600 and 200% zoom were not opened.
```

## PostgreSQL inventory (2026-08-23)

```text
Requirement: PG-001, PG-002 (no vault)
Decision/ADR: ADR-001, ADR-003, ADR-008 (majors 17/18 in code); ADR-004 deferred
Source characterization: database-app list_databases / get_database_info / get_security_overview
 at 1c3e8e2; Redgres deny set is stricter (SOURCE_SYSTEMS.md)
Implementation files: internal/postgresadmin/*; internal/config/postgres.go;
 internal/httpapi/postgres_routes.go; cmd/redgres/main.go; go.mod (pgx v5.10.0)
Unit tests: internal/postgresadmin/*_test.go; internal/httpapi/postgres_routes_test.go;
 internal/config postgres cases
Integration tests: none run (no live PostgreSQL claimed)
Security tests: protected/missing collapse to 404; canary DSN not returned; no-store;
 no vault query; password file only
Deployment/migration impact: none. 001_initial.sql unchanged. Production serve now
 requires a complete admin PostgreSQL connection.
Known limitations: PG-003–012 / AUTH-006 / Redis not started; live PG 17/18 and race/CI unproven
Commands executed locally (2026-08-23):
  go list -m -versions github.com/jackc/pgx/v5 → newest stable v5.10.0
  go list -m -json github.com/jackc/pgx/v5@v5.10.0 → 2026-06-03, Go 1.25.0, MIT pin
  go test -count=1 ./... → ok (cmd/redgres, audit, auth, config, database, httpapi, postgresadmin, web)
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
  Not run: go test -race, live PostgreSQL 17/18, CI, gitleaks, govulncheck
Reviewer/date: Verifier approved (2026-08-23) PG-001/002 at `52930bd` + pin `260e45a`
 (not pushed). Security review approved merge; Medium M1 (SSLROOTCERT libpq keyword
 injection) corrected with tab/CR/LF/`=` reject + quoted sslrootcert
 (`TestLoadRejectsSSLRootCertKeywordInjection`). Residual Lows are owner-static
 capabilities, case-sensitive deny names, and unpinned search_path. Do not treat
 this as COMPATIBILITY.md §6 evidence.
```

## PostgreSQL inventory UI (2026-08-23)

```text
Requirement: PG-001/002 (UI only)
Decision/ADR: ADR-001, ADR-003
Source characterization: UI_DESIGN_SYSTEM ledger/inspector; consumes /api/v1/postgres/databases
Implementation files: web/src/api/postgres.ts; web/src/features/postgres/DatabasesPage.tsx;
 web/src/features/pages/Placeholders.tsx; web/src/styles/globals.css; web/src/App.test.tsx
Unit tests: App.test.tsx list + unavailable + stale-selection + security flags
Integration tests: none
Security tests: no healthz; 503 is not an empty healthy cluster; saved credential
 always shown as Not available; no style={{}}; no localStorage
Deployment/migration impact: none
Known limitations: no create/URLs/tables; jsdom cannot prove viewports
Commands executed locally (2026-08-23):
  web: npm run test:run → 18 passed; npm run build → TypeScript 7.0.2 + Vite 8.2.2
Reviewer/date: UI reviewer rejected (2026-08-23) on details race, long-name overflow,
 incomplete security facts, and silent details load. Remediations: AbortController
 isolation + name match; `.identifier` wrap; all security flags; aria-busy + status.
 Re-review approved (2026-08-23) at `aff68d8` (not pushed). Not viewport
 sign-off: 360/768/1280/1600 and 200% zoom were not opened.
```

## PostgreSQL table list API (2026-08-23)

```text
Requirement: PG-007 (table list only; no rows, search, truncate, or UI)
Decision/ADR: ADR-001, ADR-003, ADR-008 (majors already gated on inventory Open)
Source characterization: database-app list_tables at 1c3e8e2
  (information_schema BASE TABLE, exclude pg_catalog/information_schema)
Implementation files: internal/postgresadmin/{types,memory,service,adapter}.go;
 internal/httpapi/{server,postgres_routes}.go
Unit tests: internal/postgresadmin/service_test.go (manageable/protected/cap/empty/canary);
 internal/httpapi/postgres_routes_test.go (session/503/200/404 collapse/400/405/canary)
Integration tests: none run (no live PostgreSQL claimed)
Security tests: protected/missing collapse to 404 before ListTables; canary DSN not returned;
 no-store; identifier 400 before query; empty 200 only after successful policy+lookup
Deployment/migration impact: none. 001_initial.sql unchanged. go.mod/config unchanged.
Known limitations: no table UI, row browse, identifier quoting of schema/table names,
 vault, mutations; live PG 17/18 and race/CI unproven
Commands executed locally (2026-08-23):
  go test -count=1 ./... → ok (cmd/redgres, audit, auth, config, database, httpapi, postgresadmin, web)
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
  Not run: go test -race, live PostgreSQL 17/18, CI, gitleaks, govulncheck, frontend (no UI change)
Reviewer/date: Verifier approved (2026-08-23) table-list API only (not full PG-007)
 at `4aea8e5` (not pushed). Security review approved merge (2026-08-23): no
 Critical/High/Medium. Lows L1–L3 corrected in `f13296b` (startup search_path including pg_temp,
 LIMIT 501, adapter ValidateIdentifier). Do not treat as COMPATIBILITY.md §6
 evidence.
```

## PostgreSQL table list UI (2026-08-23)

```text
Requirement: PG-007 (inspector list only; no rows, search, truncate)
Decision/ADR: ADR-001, ADR-003
Source characterization: consumes GET /api/v1/postgres/databases/{db}/tables;
 database-app list_tables UI at 1c3e8e2 inspected, not copied
Implementation files: web/src/api/postgres.ts; web/src/features/postgres/DatabasesPage.tsx;
 web/src/styles/globals.css; web/src/App.test.tsx
Unit tests: App.test.tsx happy/empty/503/truncated/stale tables + prior inventory cases
Integration tests: none
Security tests: no /rows fetch; table names are not buttons; 503 ≠ “No tables.”;
 no localStorage; no style={{}}
Deployment/migration impact: none
Known limitations: no row browse; jsdom cannot prove viewports
Commands executed locally (2026-08-23):
  web: npm run test:run → 22 passed; npm run build → TypeScript 7.0.2 + Vite 8.2.2
Reviewer/date: UI reviewer approved (2026-08-23) inspector list only; Lows L1–L5
 optional (card chrome, empty copy, truncation live region, schema/table spacing,
 503 wording). Verifier approved (2026-08-23) at `ce7cf8b` (not pushed). Not
 viewport sign-off. Not full PG-007.
```

## PostgreSQL row browse API (2026-08-23)

```text
Requirement: PG-007 (bounded row GET only; no UI, DELETE, truncate)
Decision/ADR: ADR-001, ADR-003, ADR-008 (majors already gated on Open)
Source characterization: database-app fetch_table_data at 1c3e8e2; Redgres 404s
 missing/zero-column tables instead of empty 200; explicit quoted SELECT list
Implementation files: internal/postgresadmin/{types,service,memory,encode,rows,adapter}.go;
 internal/httpapi/{server,postgres_routes,errors}.go
Unit tests: service/encode + HTTP session/503/200 clamp/404/400/405/canary
Integration tests: none run (no live PostgreSQL claimed)
Security tests: identifier 400 before query; protected db no ListRows; canary
 redacted; no-store; marshal-first writeJSON
Deployment/migration impact: none. 001_initial.sql and go.mod unchanged.
Known limitations: no row UI; COUNT/OFFSET on huge tables; q LIKE wildcards;
 citext via data_type only; huge TOAST cells; live PG 17/18 unproven
Assumptions recorded: q max 128 code points; offset unbounded; GET not audited
Commands executed locally (2026-08-23):
  go test -count=1 ./... → ok
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
  Verifier also ran: go test -race -count=1 ./internal/postgresadmin/ ./internal/httpapi/ → ok
  Not run: full ./... race, live PostgreSQL 17/18, CI, frontend
Reviewer/date: Verifier approved (2026-08-23) row-browse API only (not full PG-007)
 at `2dfaec7` (not pushed). Security review approved merge (2026-08-23): no
 Critical/High/Medium. Accepted Lows: L1 LIKE `%`/`_` remain wildcards (documented);
 L2 adapter does not re-clamp limit/q (HTTP+service clamp today); L3 no HTTP test
 that marshal failure never emits 200 (writeJSON is fail-closed). Do not treat as
 COMPATIBILITY.md §6 evidence.
```

## Embedded assets without a frontend build (2026-08-25)

```text
Requirement: No functional PRD ID; NFR-009 maintainability / release-gate
 correctness. Fixes a pre-existing defect that made the CI backend job red.
Decision/ADR: ADR-001. No new or superseded ADR: no contract, endpoint, or
 configuration key changes, and the HTTP allow-list is untouched.
Source characterization: none.
Defect: web.Assets() was fs.Sub(dist, "dist/app"). Only internal/web/dist/.gitkeep
 is tracked, so on a clean checkout dist/app does not exist and fs.Sub returns a
 lazy subFS with no StatFS; fs.Stat(assets, ".") then returns ErrNotExist. The
 pre-existing internal/web/embed_test.go:14 assertion — in a test literally named
 TestAssetsAvailableWithoutFrontendBuild — therefore failed, and
 .github/workflows/ci.yml runs go test ./... with no frontend build, so the
 backend job was red. Discovered by redgres-verifier while checking the
 single-port dev runtime slice and reproduced independently at HEAD.
Implementation files: internal/web/embed.go — Assets() falls back to an emptyFS
 when dist/app is absent. emptyFS implements Open/Stat/ReadDir and exposes only a
 statable, readable, empty root; it serves no file, so the tracked dist/.gitkeep
 never becomes reachable and no partial application can be served.
Unit tests: internal/web/embed_test.go — existing
 TestAssetsAvailableWithoutFrontendBuild now passes on a clean checkout;
 new TestAssetsPlaceholderTreeIsEmptyButUsable pins the placeholder contract
 (root is a directory, ReadDir returns zero entries, and index.html, .gitkeep and
 assets/app.js are all unopenable).
Integration tests: none
Security tests: the placeholder tree exposes no file, so the frontend-unavailable
 path cannot be turned into a partial or unexpected asset response. Verified end to
 end on a clean checkout with no frontend build: GET / → 503 and
 GET /api/v1/healthz → 200, so the dependency_unavailable behavior in
 internal/httpapi/server.go:72 is preserved rather than replaced by a fallback.
Deployment/migration impact: none. A release build still embeds the real
 dist/app and takes the unchanged path. 001_initial.sql, go.mod, go.sum, and
 web/package-lock.json unchanged.
Known limitations: none outstanding for this defect. CI has since run green on this
 commit (see the CI evidence entry below), which is the first time the backend job
 has passed.
Commands executed locally (2026-08-25):
  reproduction at HEAD (no slice files): git archive -o head.zip HEAD;
   Expand-Archive; go test -count=1 -v -run TestAssetsAvailableWithoutFrontendBuild
   ./internal/web/ → FAIL embed_test.go:14 "embed root: open .: file does not exist"
  clean checkout with the fix (dist contains only .gitkeep):
   go test -count=1 ./... → all packages ok, EXIT=0
   go build -o ci-redgres.exe ./cmd/redgres; serve on 127.0.0.1:8991 →
   GET / → 503, GET /api/v1/healthz → 200
  working tree: go test -count=1 ./... → ok; go vet ./... → no findings;
   gofmt -l cmd internal migrations → empty; go build -o NUL ./cmd/redgres → success
  Not run locally: live PostgreSQL. CI evidence is recorded below.
Reviewer/date: pending independent review. CI green on `f10eb19`.
```

## First green CI run (2026-08-25)

```text
Requirement: NFR-007/NFR-010 release-gate evidence. No functional PRD ID.
Purpose: replace the "CI has never run / unproven" caveats carried by every
 earlier entry in this file with one authoritative record.
Evidence: GitHub Actions run 32764658544 on `f10eb19` (branch `master`, event
 push, ubuntu-latest) completed with conclusion success. All six jobs passed:
  backend (2m54s)        — go mod verify; gofmt -l cmd internal migrations;
                           go vet ./...; go test ./...; go test -race ./...;
                           go build ./cmd/redgres
  cross-compile (1m41s)  — CGO_ENABLED=0 GOOS=linux GOARCH=amd64 and arm64 builds
  frontend (21s)         — Node from web/.nvmrc (24.19.0, the pinned runtime);
                           npm ci; npm run test:run; npm run build; the inline
                           script/style rejection check on the built index.html;
                           npm audit --omit=dev --audit-level=high
  embedded-build (58s)   — frontend build + go build, then a healthz smoke test
                           asserting {"status":"ok"} on 127.0.0.1:8790
  secret-scan (9s)       — gitleaks-action v3.0.0 over full history (fetch-depth 0)
  vulnerability (26s)    — govulncheck v1.7.0 ./...
What this resolves: full `go test -race ./...`, linux/amd64 and linux/arm64
 cross-compiles, frontend tests and build on pinned Node 24.19.0, gitleaks,
 govulncheck, and npm audit were previously recorded as CI-only and unproven in the
 Wave 0, owner auth, login/shell, PostgreSQL inventory, table-list, row-browse, and
 single-port dev runtime entries. They are now proven for the tree at `f10eb19`.
Prior state: run 32627397991 on `da59ada` failed in the backend job at
 `go test ./...` with "--- FAIL: TestAssetsAvailableWithoutFrontendBuild /
 embed_test.go:14: embed root: open .: file does not exist" — the same failure the
 parent reproduced locally and fixed in `f10eb19`. The Linux CI log confirms the
 diagnosis independently of the Windows reproduction.
Known limitations: this proves the checks in .github/workflows/ci.yml only. It is
 not COMPATIBILITY.md §6 evidence: no live PostgreSQL 17/18, Redis, or PgBouncer
 job exists yet, and no CI job exercises the dev:full workflow. Evidence is
 commit-scoped; later commits require their own run.
Commands executed locally (2026-08-25):
  gh run list --limit 5
  gh run view 32627397991 --log-failed  → the embed_test.go:14 failure above
  gh run view 32764658544               → all six jobs green, conclusion success
Note: `f10eb19` and `410ba4c` were pushed by the user, not by an agent.
Reviewer/date: CI is machine evidence; no reviewer sign-off required.
```

## Local single-port development runtime (2026-08-25)

```text
Requirement: No functional PRD ID; developer tooling + one configuration key
 (NFR-007/NFR-009 partial). Claims no PG-*/AUTH-*/PLAT-*/REDIS-*/OPS-* progress
 and no COMPATIBILITY.md §6 evidence.
Decision/ADR: ADR-001 (module boundary is why internal/web reads no environment);
 ADR-003 exact-origin preserved (dev script sets ADDRESS and BASE_URL to the same
 http://127.0.0.1:8989, so no proxy and no Origin rewrite). No new/superseded ADR:
 the filesystem asset source is development-only, production-rejected, adds no
 dependency and no network surface, and does not change the transport allow-list.
Source characterization: none. No source-system behavior ported.
Implementation files: internal/config/config.go (DevAssetDir field +
 loadDevAssetDir fail-closed validation); internal/web/assets.go
 (Open(devAssetDir string) (fs.FS, func(), error) using os.OpenRoot + Root.FS();
 no os.Getenv, no environment concept); cmd/redgres/main.go
 (web.Open(cfg.DevAssetDir) + defer closeAssets()); web/scripts/dev-full.mjs;
 web/package.json (dev:full); web/vite.config.ts (dev host 127.0.0.1 + strictPort
 for the two-port workflow only); .gitignore (*.db-*); .env.example; README.md
 (dev workflows, rebuild window, orphaned-process cleanup)
Unit tests: internal/config/config_test.go — DevAssetDir default empty,
 development absolute-path resolution, production rejection naming the variable
 without echoing the path, missing directory, regular file, URI-reserved
 characters, and production-unset stays empty.
 internal/web/assets_test.go — Open("") delegates to the embedded assets (asserted
 by agreeing with Assets() on index.html readability, so it holds on a clean
 checkout), Open(dir) reads the directory, Open("") still uses the embed while
 REDGRES_DEV_ASSET_DIR is set (with a positive control proving the marker is
 reachable via the argument, so the negative cannot pass vacuously), Open rejects a
 missing directory, and link escape is contained: a junction out of the root is
 blocked (runs and passes on Windows) and a symlink out of the root is blocked
 (skips on this host for lack of the Windows symlink privilege; runs on Linux CI).
Integration tests: none
Security tests: internal/httpapi/server_test.go
 TestStaticFromDirectoryKeepsAllowList proves the index.html + assets/* allow-list
 bounds a real os.DirFS (different traversal/separator behavior than fstest.MapFS):
 index no-store, hashed asset immutable, non-allow-listed secret.env falls back to
 index without leaking its canary, missing asset 404, and nine traversal forms
 never return the outside-the-root canary or non-index content: ../.., %2e%2e,
 ..%2f, backslash, %5c, an NTFS alternate-data-stream colon, %00, and the reserved
 device names NUL.js and CON. /assets/ returns the SPA index fallback and never a
 directory listing.
 Note: fs.ValidPath in allowedStaticName is the only path guard in Redgres code
 for static requests. chi middleware.CleanPath writes only rctx.RoutePath and does
 not rewrite r.URL.Path, which serveStatic reads, so CleanPath must not be counted
 as a static-path bound. fs.ValidPath is strictly stronger here than path.Clean
 would be, because it rejects traversal outright instead of normalizing it.
 TestStaticFromEmptyDirectoryIsUnavailable keeps 503 dependency_unavailable +
 no-store for a directory without index.html (no fallback added).
 Existing server_test.go and internal/web/embed_test.go cases were not modified,
 relaxed, or deleted; internal/httpapi/server.go was not touched.
Deployment/migration impact: none deployed. 001_initial.sql, go.mod, go.sum, and
 web/package-lock.json unchanged (zero new Go modules and zero new npm packages).
 REDGRES_ADDRESS default remains 127.0.0.1:8790; port 8989 is set only by the dev
 script. No CLI flag was added for REDGRES_DEV_ASSET_DIR.
Known limitations: the httpapi allow-list bounds served names, not link targets;
 target containment comes from os.Root, and production rejection remains the
 outer control. os.Root holds an open directory handle, so Open returns a release
 function that cmd/redgres defers. A TOCTOU window remains between config
 validation and use; the Stat is a startup usability check producing a named error
 instead of a later 503, not a security boundary. config.Load now performs one Stat
 (Load already read .env from disk); vite build --watch sets emptyOutDir, so a
 refresh mid-rebuild returns 503 (documented in README, deliberately no retry or
 fallback); the NUL-byte guard is not reachable from the t.Setenv seam on Windows;
 no CI job exercises dev:full, so this workflow is only ever locally proven;
 graceful Ctrl+C shutdown could not be exercised from the agent harness (no
 console signal delivery) — only the force-kill path was observed, which orphans
 the Go and Vite children and keeps port 8989 bound (documented in README).
 Config.DevAssetDir is an exported field, so code building a config.Config literal
 outside Load bypasses loadDevAssetDir; internal/httpapi/server_test.go does this
 deliberately, and no production path constructs a Config outside Load.
 Development mode still permits a non-loopback REDGRES_ADDRESS (validateAddress
 enforces loopback only in production), so a manually widened bind plus a dev asset
 dir would expose the asset server on the LAN. Pre-existing; dev:full itself always
 pins 127.0.0.1 and its explicit keys override any inherited environment.
 .gitignore *.db-* does not match SQLite etilqs_* temporaries or sidecars of a
 -sqlite-path without a .db extension (both pre-existing gaps), and it would
 silently ignore a future tracked fixture such as testdata/legacy.db-wal.
 web/scripts/dev-full.mjs passes only static literal args with shell:true on
 Windows (required since Node refuses to spawn npm.cmd without it); the repository
 path reaches children through the cwd option, not string concatenation, so it is
 not an injection sink today but would become one if a path were interpolated.
 Node prints DEP0190 on every dev:full start for that shell:true call.
 dev:full type-checks only in the initial npm run build; the vite build --watch
 rebuilds skip tsc, so type errors introduced after startup do not surface until
 the next full build.
 emptyOutDir is set in web/vite.config.ts, not by the --watch flag.
Assumptions recorded: 8989 chosen for the unified dev workflow to avoid colliding
 with 8790 (serve) and 5173 (Vite); dev:full always runs a full npm run build
 before starting Go because the new validation requires the directory to exist.
Commands executed locally (2026-08-25):
  go test -count=1 ./... → ok (cmd/redgres, audit, auth, config, database, httpapi,
   postgresadmin, web)
  go vet ./... → no findings
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
  go test -race -count=1 ./internal/config/ ./internal/web/ ./internal/httpapi/ → ok
  web: npm run test:run → 32 passed; npm run build → TypeScript 7.0.2 + Vite 8.2.2
   (Node v25.3.0 locally; web/.nvmrc pins 24.19.0, so pinned-runtime frontend
   evidence remains CI-only)
  clean-checkout run (git archive HEAD + this slice's files, no frontend build):
   go test -count=1 ./... → every package ok except the pre-existing
   internal/web TestAssetsAvailableWithoutFrontendBuild failure recorded below;
   go test -run TestOpen ./internal/web/ → 5 pass, 1 skip (Windows symlink
   privilege), including the junction containment test
  production fail-closed: REDGRES_ENVIRONMENT=production with REDGRES_DEV_ASSET_DIR
   set → exit 1, stderr "REDGRES_DEV_ASSET_DIR: not allowed in production"
   (variable named, path not echoed)
  npm --prefix web run dev:full → "listening" at 127.0.0.1:8989;
   GET /api/v1/healthz → 200; GET / → 200
  live-reload proof: overwrote internal/web/dist/app/index.html while the server ran
   → GET / returned the new bytes with no Go restart; real build restored afterwards
  os.Root vs emptyOutDir: edited and reverted web/src/App.tsx while dev:full ran →
   two "build started ... built in 93ms/91ms" cycles succeeded and GET / stayed 200,
   so the open os.Root handle does not block Vite rebuilds on Windows (emptyOutDir
   removes files inside the directory, not the directory itself)
  go test -race -count=1 ./internal/config/ ./internal/web/ ./internal/httpapi/ → ok
   (re-run after the os.Root change)
  git check-ignore -v redgres.db-x-owners-1-password_hash.bin →
   ".gitignore:15:*.db-*  redgres.db-x-owners-1-password_hash.bin"
  Not run: go test -race ./... in full, CI, gitleaks, govulncheck, live PostgreSQL
Secret hygiene: untracked redgres.db-x-owners-1-password_hash.bin is a BLOB export
 of owners.password_hash (Argon2id). Prior .gitignore patterns (*.db, *.db-shm,
 *.db-wal) did not match it; *.db-* now does. It was never staged or committed and
 never left the machine. Its producing tool is unverified — most likely a local
 SQLite GUI/editor BLOB export, not modernc.org/sqlite. Deletion is a user action.
Pre-existing defect discovered by review, NOT fixed in this slice: on a clean
 checkout `go test ./...` fails because web.Assets() is fs.Sub(dist, "dist/app")
 and only dist/.gitkeep is tracked, so fs.Stat(assets, ".") returns ErrNotExist.
 Reproduced by the parent at HEAD with no slice files applied:
   git archive -o head.zip HEAD; Expand-Archive; go test -count=1 -v
    -run TestAssetsAvailableWithoutFrontendBuild ./internal/web/
   → FAIL internal/web/embed_test.go:14 "embed root: open .: file does not exist"
 The .github/workflows/ci.yml backend job runs go test ./... with no frontend
 build, so that job is red on master today and was before this slice. This slice
 does not fix it (internal/web/embed.go is out of scope) and no longer duplicates
 it: the slice's own clean-tree run shows every package ok except that one
 pre-existing failure. Fixing Assets() clean-tree behavior is the next slice.
 Correction: an earlier draft of this entry claimed the new tests "pass with only
 the dist/.gitkeep placeholder" while relying on a locally built dist/app. That
 claim was false and has been replaced by the delegation assertion above.
Reviewer/date: Security review (2026-08-25) approve-with-fixes: no Critical/High.
 Mediums corrected in this change — (a) Windows backslash/%5c/ADS-colon/%00 and
 reserved-device traversal forms were untested and are now asserted; (b) the
 symlink-containment overclaim in docs/ARCHITECTURE.md and this entry was fixed by
 switching internal/web/assets.go from os.DirFS to os.OpenRoot + Root.FS(), making
 target containment real rather than reworded, with a Windows junction test that
 passes locally; (c) the CleanPath misconception is recorded above. The reviewer
 could not execute commands in its session and read the tree statically; every
 command in this entry was executed in the parent session. Accepted Lows: exported
 DevAssetDir field, .gitignore breadth, shell:true forward-looking risk, TOCTOU.
 Outstanding user action: delete redgres.db-x-owners-1-password_hash.bin —
 .gitignore prevents accidental commit but not `git add -f`, and the file remains
 offline-crackable Argon2id material on disk.
 Verifier (2026-08-25) returned FAIL on exactly one item: the clean-checkout claim
 for internal/web tests. The parent reproduced the failure at HEAD, confirmed it is
 pre-existing, removed the duplicate dependency from the new tests, and corrected
 this entry. All other verified items passed: no functional PRD claim, internal/web
 free of os.Getenv, config ownership with no CLI flag, existing httpapi/embed tests
 preserved byte-identical, forbidden files untouched, the full local gate, the
 gitignore resolution, production fail-closed, and documentation consistency.
 Local commit `410ba4c` on `master` (not pushed).
```

## PostgreSQL row browse UI (2026-08-23)

```text
Requirement: PG-007 (inspector row grid/search/pager only; no DELETE)
Decision/ADR: ADR-001, ADR-003
Source characterization: consumes GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows;
 database-app table view at 1c3e8e2 inspected, not copied. Submit-only q (not live search).
 200-empty vs 404 distinguished. No PK/delete/edit/title dump.
Implementation files: web/src/api/{client,postgres}.ts;
 web/src/features/postgres/DatabasesPage.tsx; web/src/styles/globals.css;
 web/src/App.test.tsx; docs/UX.md
Unit tests: App.test.tsx happy/empty/503/404/stale table/db-clear/q client+400/pager/XSS-text + prior cases
Integration tests: none
Security tests: no localStorage writes; q not placed in location; markup cell is a text node;
 no /rows until a table is activated; 503/404 ≠ “No rows.”; search autoComplete=off
Deployment/migration impact: none
Known limitations: jsdom cannot prove viewports; no DELETE; live PG 17/18 unproven
Assumptions recorded: changing table resets q/offset; Null label; omit empty q and offset 0;
 Next offset is response.offset+limit; Next disabled when offset+rows.length>=total
Commands executed locally (2026-08-23):
  web: npm run test:run → 32 passed; npm run build → TypeScript 7.0.2 + Vite 8.2.2
Reviewer/date: Security review approved merge (2026-08-23): no Critical/High/Medium.
 Verifier approved remaining UI only (not full PG-007). UI reviewer requested
 M1/M2 then approved remediations (2026-08-23): bounded sticky pane; rows after
 selected table; Back to tables; hide other tables below 1024px. L1–L3 applied;
 L4/L5 accepted residuals. Local commit `095ae11` (not pushed). Not viewport
 sign-off. Not COMPATIBILITY.md §6.
```

## Paginated audit history read API (2026-08-25)

```text
Requirement: PLAT-003 (backend only). Also closes the PLAT-002 residual recorded
 at the owner-auth entry above: "no HTTP assertion that success audit rows exist".
Decision/ADR: ADR-001 (audit read stays behind internal/audit, transport in
 internal/httpapi), ADR-005 (SQLite control state). No new ADR and none superseded:
 the endpoint contract belongs in docs/API.md and the module graph is unchanged,
 so docs/ARCHITECTURE.md needed no edit (line 49 already routes audit endpoints to
 the audit service and the control-state repository).
Source characterization: no source-system parity involved. docs/API.md already
 reserved GET /api/v1/audit?cursor=&limit= and the audit.read capability but
 defined no response shape, so the contract was written in this slice.
Ordering-key finding (the load-bearing decision): created_at is NOT usable as the
 paging key, for two independent reasons proven locally rather than assumed.
 (a) It is not unique. (b) It is not lexicographically chronological: Store.Record
 writes time.RFC3339Nano, whose fractional-second directive drops trailing zeros,
 so the TEXT column has variable-length fractions and SQLite's string comparison
 disagrees with time order wherever one fraction is a prefix of another, because
 'Z' (0x5A) sorts above every digit (0x30-0x39). Measured with a throwaway Go
 program (Go 1.27.0, deleted after use):
   "2026-08-25T04:11:05Z"   vs "2026-08-25T04:11:05.5Z"  → time_before=true  string_less=false
   "2026-08-25T04:11:05.1Z" vs "2026-08-25T04:11:05.12Z" → time_before=true  string_less=false
   "…05.000000001Z"         vs "…05.00000001Z"           → time_before=true  string_less=true
 (c) created_at is also sampled by time.Now() before the INSERT, so it can
 disagree with commit order. Paging therefore orders by id, the SQLite rowid
 alias. TestCreatedAtStringOrderIsNotChronological pins case (a) inside the real
 schema so the hazard cannot silently change.
 Primary-source verification for id (https://www.sqlite.org/autoinc.html,
 page last updated 2024-02-22): "a column with type INTEGER PRIMARY KEY is an
 alias for the ROWID"; on INSERT without an explicit value it is "filled
 automatically with an unused integer, usually one more than the largest ROWID
 currently in use"; and the algorithm "will generate monotonically increasing
 unique ROWIDs as long as you never use the maximum ROWID value and you never
 delete the entry in the table with the largest ROWID". AUTOINCREMENT was NOT
 added; it is not required for keyset paging and imposes overhead.
Migration impact: NONE. migrations/001_initial.sql is unchanged and no 002_*.sql
 was added. ORDER BY id DESC needs no new index because it does not sort:
 TestListPlanUsesNoTemporaryBTree asserts EXPLAIN QUERY PLAN for both statements
 is non-empty and contains no TEMP B-TREE, converting that from assertion to
 evidence. Observed plans are "SCAN audit_events" and "SEARCH audit_events USING
 INTEGER PRIMARY KEY (rowid<?)", with no sorter. Scan DIRECTION is deliberately
 not claimed: EXPLAIN QUERY PLAN does not annotate it, so "reverse b-tree
 traversal" would be an inference. The load-bearing fact — no sorter, therefore
 no index and no migration — is directly proven. The existing
 audit_events_created_at_idx is simply unused here and was left alone.
 go.mod/go.sum unchanged; no new dependency. No configuration key — the page
 bounds are code constants, as defaultRowLimit already is.
Implementation files: internal/audit/list.go (EventSummary, Page, ClampListLimit,
 Store.List); internal/httpapi/audit_routes.go (handleAuditEvents, opaque cursor
 codec, limit clamp, error mapping); internal/httpapi/server.go (one route line,
 gated by requireSession + requireCapability("audit.read")); docs/API.md.
Contract: GET /api/v1/audit → {events,has_more,next_cursor?,limit,request_id},
 newest first. cursor is opaque base64url-unpadded of a versioned "a1:<id>"
 payload, exclusive. Default limit 50; limit<=0 or >500 clamps to 50 and the
 response echoes the effective limit; non-integer limit is 400. events is always
 an array, never null. next_cursor is present only when has_more is true.
Metadata exclusion (deliberate): the metadata column is neither returned nor
 SELECTed. redactMetadata is a substring heuristic that does not recurse into
 nested maps/slices and deliberately keeps a key named "session" (the existing
 fixture documents this), so historical rows may hold whatever it allowed.
 Excluding the column keeps that content out of process memory by construction
 instead of relying on a filter staying correct. Named follow-up: a later slice
 may project an explicit allow-list of metadata keys, never the raw column.
Unit tests: internal/audit/list_test.go — newest-first ordering; three-page walk
 with no skip/repeat; stability across an insert between two page reads; tied
 created_at strings paged deterministically; created_at string-order hazard;
 empty slice not nil; cursor below the oldest id; NULL actor/target/client_ip read
 as ""; ClampListLimit table; clamped limits honored; EXPLAIN QUERY PLAN;
 query failure surfaced; context cancellation honored.
 internal/httpapi/audit_routes_test.go — 14 tests: login success visible (the
 PLAT-002 gap closer); failure and success both visible; stable HTTP paging with
 an interleaved write and 5 distinct ids; limit clamp echo for 0/-3/501;
 "events":[] literal on an empty table; well-formed cursor below the oldest id is
 200-empty not 400/404; 401 for absent and unknown cookies; 12 malformed cursor
 forms; 4 malformed limit forms; 405 for POST/DELETE/PUT; storage failure; the
 metadata canary; the capability gate; cursor round trip and URL safety.
Integration tests: none. This slice touches no PostgreSQL or Redis adapter, so
 the COMPATIBILITY.md §6 matrix is not applicable and no §6 evidence is claimed.
Security tests: TestAuditListNeverReturnsMetadata inserts a row with raw SQL,
 bypassing Store.Record and therefore the write-time redactor, containing a
 postgresql:// DSN with a password, a "password" pair, a csrf_token and a
 session_token; it first asserts the canary really is stored and that the canary
 event is present in the returned page (so the test cannot pass vacuously), then
 asserts audit.ContainsSecret(body) is false and that "canary", "secret",
 "csrf_token", "postgresql://" and "metadata" are all absent. Malformed-cursor
 cases assert the submitted value is never echoed back. Cache-Control: no-store
 asserted on 200, 400, 401 and 503. Storage-failure test asserts 503
 dependency_unavailable with the fixed storageUnavailable message and no db path,
 "sqlite", "modernc", "audit_events" or "no such table" text, and no "events" key.
 401 responses asserted to expose no "events" key. All SQL is parameterized; the
 cursor never reaches a statement as text.
Storage-failure seam: the audit table is DROPped rather than the connection
 closed, because closing srv.db makes requireSession fail first and the resulting
 503 would not exercise the handler's own error mapping.
Known limitations: metadata is not exposed at all (deliberate, above). A
 route-level 403 is unreachable because hasCapability tests the package-level
 defaultCapabilities list which contains audit.read — the same pre-existing
 residual already recorded for postgres.read; the gate is instead proven by a
 hasCapability table test and a middleware-level 403 for an unknown capability,
 and the capability model was not changed. No total count (the table grows without
 bound, so NFR-002 forbids a COUNT(*)), and no UI — audit history is not yet
 reachable from the browser. Rowid reuse is possible only if the highest rows are
 deleted, which a future oldest-first retention policy would not do; the cursor is
 opaque so its encoding can change without a contract break.
 go test -race ./... in full, gitleaks, govulncheck, cross-compiles and the
 frontend jobs remain CI-only and are not claimed here.
SECURITY RESIDUAL — unauthenticated audit-row injection and flooding (Medium,
 pre-existing in the AUTH/PLAT-002 login path, NOT introduced by this slice, and
 deliberately NOT fixed here because internal/auth and internal/httpapi/
 auth_routes.go login behavior is outside PLAT-003). Both login failure branches
 let an unauthenticated caller add audit rows without bound, verified by reading
 the code in this session:
   (a) internal/httpapi/auth_routes.go:117-121 (invalid username) records an audit
       row whose actor/target are the fixed literal "invalid_username" and never
       calls store.Record, so it contributes nothing to lockout at all;
   (b) internal/httpapi/auth_routes.go:136-147 (well-formed but unknown username)
       records an audit row whose actor/target are the CALLER'S OWN string, and
       although it does call store.Record, AttemptStore.LockoutRemaining keys on
       (username, client_ip) (internal/auth/security.go:72-79), so a caller
       varying the username never accumulates lockoutThreshold=5 failures for any
       single key and is never locked out.
 The practical throttle in both cases is only the argon2id cost of VerifyUnknown
 (m=65536 KiB, t=3, p=4 — internal/auth/password.go:18-20, pinned by
 password_test.go:13). Injected content is bounded by ValidateUsername to 64
 runes with no Cc control characters and is lowercased, and JSON encoding makes it
 inert in a response, so there is no injection or log-forging risk today (this
 route logs nothing).
 What THIS slice changes is the consequence, which is why it is recorded here:
 audit history is now readable only as unfiltered newest-first pages, with no
 filtering by action/outcome/actor/date and no retention policy (NFR-006 open).
 An attacker can therefore push genuine security events off the operator's early
 pages and grow the SQLite control-plane file without authenticating. That
 degrades PLAT-003's own criterion that "failure events are visible". The
 follow-up filtering slice is therefore a SECURITY requirement, not a
 convenience feature, and should be scoped with retention (NFR-006) and with
 making the invalid-username branch contribute to rate limiting.
FUTURE-UI CONSTRAINT — bidirectional/format characters survive into actor/target.
 ValidateUsername rejects only unicode.IsControl (category Cc,
 internal/auth/security.go:40-44), so U+202E RIGHT-TO-LEFT OVERRIDE, other Cf
 characters, and homoglyphs pass. Nothing is exploitable today: encoding/json
 escapes safely and there is no UI consumer of this endpoint. The audit-history UI
 slice MUST neutralize bidi/format controls at render time to prevent a
 Trojan-Source-style display spoof against the operator reading the log.
 ValidateUsername was deliberately not changed here; that would alter AUTH
 behavior outside PLAT-003.
Accepted residual: the 503 path discards the storage error with no server-side
 log (internal/httpapi/audit_routes.go:44-47). Response hygiene is the point, and
 this matches the existing writePostgresError pattern, so adding logging was left
 out of this slice rather than diverging from the codebase-wide convention.
Commands executed locally (2026-08-25):
  go test -count=1 ./internal/audit/ → ok 1.028s
  go test -count=1 ./internal/httpapi/ → ok 8.088s
  go test -count=1 -run TestAudit ./internal/httpapi/ -v → 14 PASS, ok 2.350s
  go test -count=1 ./... → ok (cmd/redgres, audit, auth, config, database,
   httpapi, postgresadmin, web; migrations no test files)
  go test -race -count=1 ./internal/audit/ ./internal/httpapi/ → ok 3.247s / 11.477s
  go vet ./... → no findings (exit 0)
  gofmt -l cmd internal migrations → empty
  go build -o NUL ./cmd/redgres → success
  git status --porcelain → exactly three modified (docs/API.md,
   docs/TRACEABILITY.md, internal/httpapi/server.go) and the four new audit
   files; no *.db, *.db-*, .env*, binary or exported BLOB is stageable
  Not run: go test -race ./... in full, CI, gitleaks, govulncheck, live PostgreSQL,
   any frontend command (web/** untouched)
Reviewer/date: Security review (2026-08-25) approve-with-required-changes, both
 documentation-only, no code change required: no Critical or High, and no
 vulnerability introduced by this diff. Confirmed by the reviewer against the
 code: metadata is unselectable on this path (a repository-wide grep finds it in
 production code only in the write path) and the eight-destination scan would fail
 loudly rather than leak if a column were added; the canary test is non-vacuous on
 all four counts; the cursor is never echoed and this route logs nothing (the
 router installs no request logger and recoverer logs only %T); the cursor decoder
 bounds length before decoding and is parameterized so injection is impossible;
 the 503 discards the error and emits a fixed constant; no-store holds
 structurally on every status because writeJSON sets it unconditionally and all
 error paths delegate to it; SameSite=Strict with no CORS blocks cross-origin
 theft of this first authenticated historical-data GET. Required changes applied
 in this entry: the M1 unauthenticated audit-row injection/flooding residual and
 the L1 bidi-character UI constraint, both recorded above. The reviewer's
 non-blocking docs/API.md point was also applied — the endpoint section now states
 that the capability set is a static single-owner grant, so the contract is
 literally true today. The parent independently verified and SHARPENED M1 before
 recording it: the reviewer attributed the unbounded path to the invalid-username
 branch, but reading internal/auth/security.go:72-79 shows lockout is keyed on
 (username, client_ip), so the well-formed-unknown-username branch is ALSO
 unbounded and it is the one carrying attacker-chosen actor/target. Accepted
 Lows: L2 no server-side log on the 503 (codebase-wide convention), L3 the
 unreachable "PostgreSQL is unavailable" marshal-failure fallback in
 errors.go:39-41, and 405 preceding authentication on every route.
 Reviewer environment caveat: shell execution was unavailable in the reviewer's
 session, so it could not run git diff, git status, or go test and read the tree
 statically; every command in this entry was executed in the parent session and
 remains for the verifier to confirm.
Verifier (2026-08-25) PASS on all ten items, recommending merge. It re-executed
 every command in the block above (Go 1.27.0 windows/amd64) and found the recorded
 results truthful, with only wall-clock differences: 14 PASS / 0 FAIL / 0 SKIP for
 -run TestAudit, race green, vet clean, gofmt empty, build exit 0. Confirmed
 byte-identical to 1e5d4c3: migrations/001_initial.sql, migrations/embed.go,
 go.mod, go.sum, web/package-lock.json, and the five existing test files
 (internal/audit/audit_test.go, internal/httpapi/{auth_routes,postgres_routes,
 server}_test.go, internal/web/embed_test.go); no t.Skip or testing.Short() added.
 All forbidden paths untouched and server.go is exactly one route line. It
 reproduced the created_at ordering hazard INDEPENDENTLY and more strongly than
 this entry did, performing the string comparison inside SQLite rather than only
 in Go (modernc.org/sqlite v1.57.0, SQLite 3.53.3): all three recorded pairs
 reproduce. It checked all 20 docs/API.md claims against the code and found no
 overclaim, including computing that the documented cursor YTE6MTQyMQ is exactly
 base64url-unpadded "a1:1421" matching the example id. It confirmed the parent's
 sharpened M1 lockout-keying correction is right on every cited line. For item 10
 it did not accept reading alone: in a scratch copy it mutated the code to select
 metadata and observed TestAuditListNeverReturnsMetadata fail loudly printing the
 canary DSN, and separately observed that adding the column without a scan
 destination yields the fixed 503 with no leak — proving both the canary test's
 load-bearing role and the "fail loudly rather than leak silently" claim.
 Three verifier findings, all corrected in this entry: (a) the recorded
 git status line omitted docs/TRACEABILITY.md and now lists all three modified
 files; (b) TestListPlanUsesNoTemporaryBTree could have passed vacuously if
 EXPLAIN returned zero rows, so it now also asserts the plan text is non-empty
 and matches the expected access path; (c) "reverse b-tree traversal" was an
 unprovable inference and has been replaced with the observed plans and an
 explicit statement that scan direction is not claimed.
 It also independently flagged the pre-existing untracked
 redgres.db-x-owners-1-password_hash.bin owner password-hash BLOB export: ignored
 by *.db-* and not stageable, so this slice's hygiene claim stands, but it
 recommends deletion. That remains an outstanding user action from the
 single-port slice above.
Reviewer/date: Security review 2026-08-25 (approve, docs-only changes applied);
 verifier 2026-08-25 (PASS, three corrections applied).
 Local commit `47b3e17` on `master` (not pushed).
```

## Audit history UI (2026-08-25)

```text
Requirement: PLAT-003 (frontend). Consumes GET /api/v1/audit from commit 47b3e17.
 Does not claim NFR-006 (retention/export). Respects NFR-002 by showing one page
 at a time with no total count. NFR-012 responsive contract is implemented in
 markup/CSS; jsdom cannot prove viewports.
Decision/ADR: ADR-001 (UI is a client of the existing audit HTTP contract).
 No new ADR. docs/API.md, docs/PRD.md, docs/SECURITY.md, and every Go file are
 unchanged. Cursor encoding, limit, and metadata exclusion stay server-owned.
Source characterization: no source-system parity. The page follows DatabasesPage
 abort/error-vs-empty patterns. API contract is F-9 / docs/API.md as shipped.
Bidi findings (load-bearing; CSS is not sufficient):
 F-1 CSS cannot neutralize a bidi override. CSS Writing Modes Level 4 §2.4.2
  (https://www.w3.org/TR/css-writing-modes-4/#bidi-control, “CSS–Unicode Bidi
  Control Translation, Text Reordering”): “bidi control codes in the source
  text are still honored”. UAX #9 Unicode 17.0.0 revision 51 §2.2
  (https://www.unicode.org/reports/tr9/tr9-51.html#Explicit_Directional_Overrides):
  directional overrides nest. unicode-bidi: isolate/plaintext therefore cannot
  disarm U+202E already present in the cell. Mandatory mechanism is character
  replacement at render time; CSS is defense in depth only.
 F-2 Twelve Bidi_Control code points, UAX #9 §2
  (https://www.unicode.org/reports/tr9/tr9-51.html#Bidirectional_Character_Types):
  implicit U+200E LRM, U+200F RLM, U+061C ALM; embeddings/overrides U+202A LRE,
  U+202B RLE, U+202C PDF, U+202D LRO, U+202E RLO; isolates U+2066 LRI, U+2067
  RLI, U+2068 FSI, U+2069 PDI. Enumerated explicitly in displayText.ts. The
  regex does not use \p{Bidi_Control} (unverified in this runtime).
 F-3 unicode-bidi: isolate with direction: ltr, not plaintext. CSS Writing Modes
  Level 4 §2.2 (https://www.w3.org/TR/css-writing-modes-4/#unicode-bidi):
  isolate takes the box’s direction property as base direction; plaintext
  “behaves as isolate except that … base directionality … is determined by
  following the heuristic in rules P2 and P3 of the Unicode bidirectional
  algorithm (rather than by using the direction property of the box)”. The
  plaintext suggestion was overridden because actor/target/action are
  attacker-influenced (login-failure actor is the caller’s own string; see the
  PLAT-003 API residual). isolate + direction:ltr keeps the base direction on
  the property, not the payload.
 F-5 Matches are replaced with U+FFFD, not deleted, so a spoof attempt remains
  visible. After the twelve explicit points, \p{Cf}|\p{Cc} is a general
  invisible-character net (U+200B, U+200D, U+FEFF, U+00AD covered). Applied to
  every rendered field: actor, action, target, outcome, request_id, client_ip,
  created_at.
 F-8 outcome is an opaque string; no status-color map.
 F-9 API contract is fixed: {events, has_more, next_cursor?, limit, request_id};
  newest first; events always an array; next_cursor only when has_more; cursor
  opaque exclusive; nullable fields ""; created_at verbatim RFC3339Nano UTC;
  no total; no metadata. First UI request is exactly /api/v1/audit.
D-1 byte-exactness trade-off: displayText() is not a byte-preserving view of
 stored text. Replacing bidi/format controls with U+FFFD is a deliberate
 display transform so the operator can see that a spoof was attempted. The
 stored row and the HTTP JSON remain unchanged. Homoglyphs (Cyrillic/Latin
 lookalikes) are an explicit non-goal and are rendered as stored.
jsdom limit: jsdom does not implement the Unicode Bidirectional Algorithm
 layout, so tests prove code-point replacement, U+FFFD presence, absence of
 the twelve F-2 points in textContent, and the isolate class. They do not
 prove visual reordering was prevented on a real renderer.
SECURITY RESIDUAL — unauthenticated audit-row injection and flooding (Medium,
 pre-existing on AUTH/PLAT-002 login, recorded on the API slice, NOT mitigated
 here). This UI makes that residual operator-visible: the history is unfiltered
 newest-first pages, so injected login-failure rows occupy the first pages the
 owner sees. No filtering, retention (NFR-006), or login-path rate-limit change
 is in this slice. ValidateUsername was not changed.
Implementation files: web/src/api/audit.ts; web/src/features/audit/AuditPage.tsx;
 web/src/text/displayText.ts; web/src/text/displayText.test.ts;
 web/src/features/pages/Placeholders.tsx (audit branch only);
 web/src/styles/globals.css (audit table/stack + .bidi-isolate);
 web/src/App.test.tsx (additive helpers/tests only);
 docs/UX.md (Audit history workflow; navigation tree unchanged);
 docs/UI_DESIGN_SYSTEM.md (§6 stored/attacker-influenced text);
 docs/REPOSITORY_STRUCTURE.md (web/src/text/); docs/TRACEABILITY.md.
Unit tests: web/src/text/displayText.test.ts — twelve F-2 points + U+200B,
 U+200D, U+FEFF, U+00AD replaced with U+FFFD, ordinary text unchanged.
 web/src/App.test.tsx AC-1 heading+table/list, placeholder absent; AC-2 first
 URL exactly /api/v1/audit; AC-3 verbatim cursor + Older disabled on has_more
 false / missing / empty next_cursor; AC-4 three-page forward, two Newer replay
 cursors; AC-5 array order ids 9,3,7; AC-6 bidi replacement + isolate class + created_at/dateTime poison
 (U+202E in the timestamp; time[dateTime] contains U+FFFD, not U+202E) +
 markup actor is a text node; AC-7 verbatim RFC3339Nano + UTC, no AM/PM;
 AC-8 em dash + Not recorded, Null absent; AC-9 source-address disclosure,
 column not Client IP; AC-10 401/400/503/TypeError; AC-11 200 empty page;
 AC-12 no Storage.setItem, location.search has no cursor, logout clears actor;
 AC-14 bounded-scroll wrapper, no service-rail on the page, 64-rune actor and
 32-hex request_id use .identifier.
Integration tests: none. No PostgreSQL/Redis adapter change; COMPATIBILITY.md
 §6 is not applicable and is not claimed.
Security tests: bidi/format replacement; markup payload remains a text node
 (document.querySelector("img") is null); 401 does not auto-redirect; 400 does
 not echo the submitted cursor; no localStorage writes; cursor not placed in
 location.search; logout removes previously rendered actor. Audit fields are
 React text nodes, not HTML.
Deployment/migration impact: none. No Go, migration, API, or configuration
 change. Application rollback is binary/config only, as already documented.
Known limitations: jsdom cannot prove visual bidi order or 360/768/1280/1600
 viewports; homoglyphs undetected; unauthenticated flooding now operator-visible
 and unmitigated; no metadata column; no filtering; no NFR-006 retention;
 local Node is v25.3.0 so pinned Node 24.19.0 frontend evidence remains CI-only.
Assumptions recorded: client cursor stack stores already-consumed next_cursor
 values and never decodes them; has_more true without a non-empty next_cursor
 is an error page, not a rendered page; Refresh and Newest both return to
 /api/v1/audit and clear the stack; empty actor/target/client_ip use em dash
 plus visually-hidden “Not recorded”, never the SQL Null token.
Commands executed locally (2026-08-25), worktree
 D:\code\github\Redgres-worktrees\plat-003-audit-ui, Node v25.3.0 (web/.nvmrc
 pins 24.19.0):
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 54 passed (54);
   Duration 34.70s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 29 modules;
   ../internal/web/dist/app/{index.html, assets/index--h2CFBMA.css 12.44 kB,
   assets/index-g_T8wCh4.js 222.64 kB}; built in 1.06s
  go build -o NUL ./cmd/redgres → success (no stdout)
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, postgresadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  gofmt -l cmd internal migrations → empty
  git diff --name-only HEAD -- *.go go.mod go.sum web/package.json
   web/package-lock.json web/vite.config.ts web/tsconfig.json docs/API.md
   docs/PRD.md web/src/nav.ts → empty
  git status --porcelain internal/web/dist → empty (build output not stageable)
  Not run: go test -race ./..., gitleaks, govulncheck, CI, live PostgreSQL or
   Redis, browser viewports 360×800 / 768×1024 / 1280×800 / 1600×1000, 200%
   zoom, Playwright, frontend jobs on pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve, no Critical/High/Medium
 introduced. Confirmed UAX #9 twelve Bidi_Control points enumerated (not
 \p{Bidi_Control}), U+FFFD not deletion, every painted field including dateTime
 through displayText, React text nodes, 400 uses a fixed string, no metadata
 in the client type, no storage/URL persistence. Homoglyphs and login-path
 flooding recorded as residuals, not claimed closed. One Low: AC-6 originally
 omitted created_at; parent applied that in `7e84e6d` (timestamp now carries
 U+202E and dateTime is asserted). Reviewer did not run tests or git diff.
 UI review (2026-08-25) approve, no required code changes. Confirmed shared
 tokens, no service rail, UTC-verbatim, distinct states, keyboard pager 44px,
 displayText on every field including dateTime. Explicitly NOT viewport,
 a11y-tree, or visual-bidi sign-off: no browser tools, app not opened. Optional
 Lows (disabled text-button opacity, VoiceOver ol mapping, empty-state next
 action, 401 copy pointing at Log out, empty created_at) accepted unfixed —
 they are polish, not contract misses. Parent also applied `1adfb89` before
 review: UX.md bidi bullet the packet required, and dateTime uses the same
 sanitized string as visible text.
 Verifier (2026-08-25) PASS on worktree HEAD `3402960`. Re-ran npm test:run
 (54 passed), npm run build, go test ./..., vet, gofmt empty; forbidden paths
 byte-identical to `1631d2e`; App.test.tsx additive only. Not viewport sign-off.
 Fast-forwarded `master` to `3402960` (not pushed).
 Local commits: `81b47b3` (feature), `1adfb89`, `7e84e6d`, `3402960` (docs).
```

## PLAT-001 authenticated status + Overview cards (2026-08-25)

```text
Requirement: PLAT-001 (Partial). Independent component health for Redgres state
 and PostgreSQL direct; PgBouncer, Redis, and tool links remain honest
 not_implemented / not_configured. NFR secret-safe payload (no host/DSN/password).
Decision/ADR: ADR-001 modular monolith; platform.Collect is the dashboard
 aggregator. No new module, migration, dependency, or COMPATIBILITY §6 claim.
Source characterization: none required for this slice (no Redis/PgBouncer probe
 parity). postgresadmin Ping uses existing pgxpool.Ping; list/details are not health.
Implementation files: internal/platform/{status.go,status_test.go};
 internal/postgresadmin/{types,errors,service,adapter,memory}.go + ping_test.go;
 internal/httpapi/{server.go,status_routes.go,status_routes_test.go};
 web/src/api/status.ts; web/src/features/overview/OverviewPage.tsx;
 web/src/features/pages/Placeholders.tsx; web/src/App.test.tsx;
 web/src/styles/shell.css; docs/{API,ARCHITECTURE,UX,TRACEABILITY}.md
Unit tests:
 platform: TestCollectMixedStateOKPostgresUnavailable;
 TestCollectReverseMixedStateUnavailablePostgresOK;
 TestCollectNilPostgresPingIsNotConfigured;
 TestCollectPostgresNotConfiguredError;
 TestCollectReturnsFixedFiveComponents;
 TestCollectOmitsCanaryHostAndPassword
 postgresadmin: TestServicePingNilCatalogIsNotConfigured;
 TestServicePingMapsMemoryCatalogPingErr; existing list/details/tables/rows still pass
 httpapi: TestStatusRequiresSession (401, no components key);
 TestStatusDefaultPostgresNotConfigured (sqlite ok, postgres not_configured, five ids);
 TestStatusPostgresUnavailableKeepsRedis;
 TestStatusHealthyPostgresKeepsRedisNotImplemented;
 TestStatusRejectsMutatingMethods (POST/PUT/PATCH/DELETE 405);
 TestStatusOmitsCanarySecrets;
 TestHealthzUnchangedWithoutComponents (+ existing TestHealthzOK)
 frontend App.test.tsx: login never calls /api/v1/status or /healthz and has no
 /reachable/i; mixed payload Reachable/Unavailable/Not connected with Redis visible;
 401 alert no cards; network throw generic alert no cards; missing redis id still
 renders Redis Unavailable; loading then replacement; Refresh refetches
 /api/v1/status with no query; logout clears cards; unknown state → Unavailable;
 postgres unavailable uses status-card-postgres + status-unavailable, not
 status-card-redis / service-rail-redis; malformed 200 JSON alert no cards.
 Unknown-URL stubs gained a disconnected /api/v1/status 200 so Overview does not
 leak a stray alert into unrelated tests.
Integration tests: none. COMPATIBILITY.md §6 is not applicable and is not claimed.
 No live PostgreSQL/Redis/PgBouncer ping in this slice.
Security tests: canary password=canary-secret and host=10.0.0.1 absent from
 Collect fields and HTTP body; 401 has no components key; healthz unchanged
 (no components, no session); login path never fetches /status or /healthz;
 GET is not audited.
Deployment/migration impact: none. No go.mod/go.sum, package.json/lockfile,
 migrations, REDGRES_* keys, or COMPATIBILITY.md change. Application rollback
 is binary/config only.
Known limitations / residuals: Redis INFO/ACL health, PgBouncer SHOW VERSION,
 tool-link hrefs, backups card, Overview audit widget, quick actions, AUTH-005,
 sidebar footer health/version, polling, version/uptime in payload, server search
 (PLAT-004), COMPATIBILITY.md §6. jsdom cannot prove 360/768/1280/1600 viewports
 or 200% zoom. Local Node is v25.3.0 so pinned Node 24.19.0 frontend evidence
 remains CI-only. Status TRACEABILITY stays Partial; do not mark Complete.
Commands executed locally (2026-08-25), worktree
 D:\code\github\Redgres-worktrees\plat-001-status, branch plat-001-status,
 baseline b2f155f, go1.27.0 windows/amd64, Node v25.3.0 (web/.nvmrc pins 24.19.0):
  gofmt -l internal/platform internal/postgresadmin internal/httpapi → empty
  go vet ./internal/platform ./internal/postgresadmin ./internal/httpapi → no findings
  go test -count=1 ./internal/platform ./internal/postgresadmin ./internal/httpapi
   → ok platform 0.494s; postgresadmin 0.567s; httpapi 8.388s
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 64 passed (64);
   Duration 10.78s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 31 modules;
   ../internal/web/dist/app/{index.html, assets/index-soMggF2I.css 12.71 kB,
   assets/index-CD7LBgYE.js 224.68 kB}; built in 190ms
  go build -o NUL ./cmd/redgres → success (no stdout)
  git diff --name-only HEAD -- go.mod go.sum web/package.json web/package-lock.json
   docs/PRD.md docs/COMPATIBILITY.md web/src/nav.ts → empty
  git status --porcelain → owned source/docs only (dist not stageable)
  Not run: go test -race ./..., gitleaks, govulncheck, CI, live PostgreSQL or
   Redis or PgBouncer, browser viewports 360×800 / 768×1024 / 1280×800 /
   1600×1000, 200% zoom, Playwright, frontend jobs on pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve; no Critical/High/Medium.
 Confirmed canary discard at adapter/service/Collect, 401 has no components,
 no-store, session gate, 200 on postgres down, platform does not import
 postgresadmin after `832fea5`, Overview does not persist. Reviewer could not
 run tests. UI review (2026-08-25) approve; no required changes. Confirmed
 shared tokens, independent rails, envelope alerts with no cards, login never
 fetches status. Explicitly NOT viewport/zoom sign-off. Optional Lows
 (aria-busy unobserved while list unmounted; Refresh layout jump) accepted.
 Parent `832fea5` moved ErrNotConfigured into platform so Collect does not
 import postgresadmin.
 Verifier (2026-08-25) PASS on worktree HEAD `2e9c344`. Re-ran go test ./...,
 vet, gofmt empty, npm test:run 64 passed, npm run build; forbidden paths
 byte-identical to `b2f155f`. Keep PLAT-001 Partial.
 Fast-forwarded `master` to `b767c38` (not pushed).
 Local commits: `6d06290` (feature), `832fea5`, `2e9c344`, `b767c38`.
```

## PLAT-004 authenticated bounded global search (2026-08-25)

```text
Requirement: PLAT-004 (Partial). Authenticated GET /api/v1/search returns
 postgres_databases + redis_acl_users; UI groups those with client filterNav
 navigation/documentation. Redis ACL hits stay not_implemented / empty.
Decision/ADR: ADR-001 modular monolith; platform.ResourceGroups is the search
 aggregator and does not import postgresadmin/redisadmin. No new module,
 migration, dependency, or COMPATIBILITY §6 claim.
Source characterization: none for Redis ACL search (adapter not implemented).
 Postgres matching reuses List manageability; no new catalog SQL.
Implementation files: internal/platform/{search.go,search_test.go};
 internal/postgresadmin/{types,service,service_test}.go (Search);
 internal/httpapi/{server.go,search_routes.go,search_routes_test.go};
 web/src/api/search.ts; web/src/components/search/NavigationSearch.tsx;
 web/src/components/shell/AppShell.tsx; web/src/features/pages/Placeholders.tsx;
 web/src/features/postgres/DatabasesPage.tsx (focusDatabase);
 web/src/App.test.tsx; web/src/styles/shell.css;
 docs/{API,ARCHITECTURE,UX,TRACEABILITY}.md
Unit tests:
 postgresadmin: TestServiceSearchOmitsProtectedNames;
 TestServiceSearchIsCaseInsensitiveOnName;
 TestServiceSearchRespectsLimitAndTruncation;
 TestServiceSearchUnavailableWithoutCatalog;
 TestServiceSearchMapsCanaryErrors
 platform: TestResourceGroupsAlwaysTwoWithEmptyRedis;
 TestResourceGroupsPostgresUnavailableKeepsRedis;
 TestResourceGroupsHitFieldsOnlyAndOmitsCanary
 httpapi: TestSearchRequiresSession;
 TestSearchRejectsMissingQueryWithoutListing;
 TestSearchRejectsLongQueryWithoutListing;
 TestSearchRejectsNonIntegerLimit; TestSearchClampsLimit;
 TestSearchNilPostgresIsNotConfigured;
 TestSearchOmitsProtectedAndReturnsManageableHit;
 TestSearchUnavailablePostgresKeepsRedisGroup;
 TestSearchRejectsMutatingMethods; TestSearchOmitsForbiddenKeys
 frontend App.test.tsx: login never calls /search /status /healthz;
 typing audit calls GET /api/v1/search?q=audit, Audit from local nav, no
 /api/v1/audit from the palette, Redis not-available copy;
 postgres hit project_a opens Databases inspector; extra password/URL fields
 ignored and canary-secret not rendered; drop/truncate only search GET;
 ArrowDown+Enter; abort on close; too-short/too-long no fetch;
 401 clears hits; postgres unavailable keeps Audit nav; logout clears
 search UI and project_a.
Integration tests: none. COMPATIBILITY.md §6 is not applicable and is not claimed.
 No live PostgreSQL/Redis in this slice.
Security tests: protected postgres omitted; canary secret/host absent from HTTP
 body; 401 has no groups key; q not echoed; no-store; mutations 405; hits only
 id/type/label; Redis never fabricated; login path never fetches /search;
 GET is not audited.
Deployment/migration impact: none. No go.mod/go.sum, package.json/lockfile,
 migrations, REDGRES_* keys, or COMPATIBILITY.md change. Application rollback
 is binary/config only.
Known limitations / residuals: Redis ACL user hits, URL deep links, docs corpus,
 command-palette actions, AUTH-005, COMPATIBILITY.md §6. jsdom cannot prove
 360/768/1280/1600 viewports or 200% zoom. Local Node is v25.3.0 so pinned
 Node 24.19.0 frontend evidence remains CI-only. Search TRACEABILITY stays
 Partial; do not mark Complete.
Commands executed locally (2026-08-25), worktree
 D:\code\github\Redgres-worktrees\plat-004-search, branch feat/plat-004-global-search,
 baseline 5f92059, go1.27.0 windows/amd64, Node v25.3.0 (web/.nvmrc pins 24.19.0):
  gofmt -l internal/platform internal/postgresadmin internal/httpapi → empty
  go vet ./internal/platform ./internal/postgresadmin ./internal/httpapi → no findings
  go test -count=1 ./internal/platform ./internal/postgresadmin ./internal/httpapi
   → ok platform 0.409s; postgresadmin 0.542s; httpapi 7.454s
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  go build -o NUL ./cmd/redgres → success (no stdout)
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 72 passed (72);
   Duration 10.90s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 32 modules;
   ../internal/web/dist/app/{index.html, assets/index-CKrIdaLU.css 12.89 kB,
   assets/index-C4h8VCwz.js 228.79 kB}; built in 168ms
  After UI reject remediations (status counts postgres hits; dialog overflow;
   focusNonce; stale-hit clear; not-connected/unavailable tones):
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 74 passed (74);
   Duration 11.17s
  git diff --name-only HEAD -- go.mod go.sum web/package.json web/package-lock.json
   docs/PRD.md docs/COMPATIBILITY.md → empty
  git status --porcelain → owned source/docs only (dist not stageable)
  Not run: go test -race ./..., gitleaks, govulncheck, CI, live PostgreSQL or
   Redis, browser viewports 360×800 / 768×1024 / 1280×800 / 1600×1000,
   200% zoom, Playwright, frontend jobs on pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve; no Critical/High/Medium.
 Confirmed session gate, no-store, q not echoed/slogged/audited, protected names
 omitted, hits id/type/label only, canary discarded, 200 on postgres down,
 Redis never fabricated, platform does not import postgresadmin, in-memory
 focusDatabase, login never fetches /search. Reviewer did not run tests.
 UI review (2026-08-25) first pass rejected H1 (status ignored postgres hits)
 and H2 (palette not a scroll container). Parent `c671569` closed those plus
 same-name refocus, stale hits, and degraded tones. Re-review approved H1/H2
 with remaining Medium M1 (in-flight empty-page copy). Parent `0993613` closed
 M1 (status is only “Searching.” while pending with no page hits). Final UI
 pass approve; no remaining High/Medium. Explicitly NOT viewport, zoom, or
 visual sign-off: no browser tools, app not opened. Lows (empty group headers,
 nested nav in palette, non-sticky input) accepted unfixed.
 Verifier (2026-08-25) PASS on worktree HEAD `28d2313`. Re-ran go test ./...,
 vet, gofmt empty, npm test:run 74 passed, npm run build; forbidden paths
 empty vs HEAD and vs `5f92059`. Keep PLAT-004 Partial.
 Fast-forwarded `master` to `6ae0c2e` (not pushed).
 Local commits: `3e983b1` (feature), `c671569`, `0993613`, `28d2313`, `6ae0c2e`.
```

## REDIS-001 ping-only connectivity on GET /status (2026-08-25)

```text
Requirement: REDIS-001 (Partial: Ping-only connectivity). PLAT-001 (Partial:
 Redis live Ping on Overview; PgBouncer still not_implemented). Do not mark
 REDIS-001 or PLAT-001 Complete.
Decision/ADR: ADR-001 modular monolith; platform.Collect stays the dashboard
 aggregator and does not import redisadmin or go-redis. ADR-006 ACL allow-list
 is not in this slice. No COMPATIBILITY.md §6 claim.
Source characterization: redis-ui Open/ParseURL/SetLogger/MaxRetries=1 and
 loopback plaintext rules as behavioral reference; not copied. go-redis v9.22.0
 official README ParseURL + NewClient (lazy; no startup Ping).
Implementation files: go.mod, go.sum;
 internal/redisadmin/{errors,service,memory,adapter,service_test}.go;
 internal/config/{config.go, redis.go, redis_test.go};
 internal/platform/{status.go,status_test.go};
 internal/httpapi/{server.go,server_test.go,status_routes.go,status_routes_test.go};
 cmd/redgres/main.go; web/src/App.test.tsx; .env.example;
 docs/{API,ARCHITECTURE,UX,CONFIGURATION,TRACEABILITY}.md; AGENTS.md
 OverviewPage.tsx / api/status.ts unchanged: presentation() already maps
 ok/unavailable/not_configured/not_implemented.
Unit tests:
 config: TestLoadNoRedisKeysDevelopment (RedisConfigured false);
 TestLoadIncompleteRedisFileEnv (names REDGRES_REDIS_ADMIN_URL_FILE);
 TestLoadRedisAllowPlaintextWithoutFile; TestLoadRejectsRawRedisURLAsFileEnv;
 TestLoadRejectsPlaintextNonLoopbackWithoutAllow; TestLoadAcceptsLoopbackRedisURL;
 TestLoadAcceptsRedissURL; TestLoadAcceptsNonLoopbackRedisWithAllowPlaintext;
 TestLoadProductionWithoutRedisURLFileFailClosed;
 TestLoadProductionWorldReadableRedisFileFailClosed;
 TestLoadRedisCanaryURLAbsentFromErrors; TestLoadRepositoryEnvExample still loads
 redisadmin: TestServicePingNilIsNotConfigured;
 TestServicePingMapsMemoryPingErr (canary absent);
 TestOpenNotConfiguredDevelopmentReturnsNilService;
 TestOpenProductionWithoutURLFileFailClosed;
 TestOpenValidURLUnusedHighPortSucceedsWithoutPing (Ping → ErrUnavailable, no leak);
 TestOpenCanaryURLAbsentFromErrors
 platform: TestCollectNilRedisPingIsNotConfigured;
 TestCollectRedisNotConfiguredError; TestCollectRedisUnavailable;
 TestCollectRedisOK; TestCollectPostgresAndRedisIndependent;
 TestCollectReturnsFixedFiveComponents (redis not_configured when ping nil,
 pgbouncer still not_implemented); TestCollectOmitsCanaryHostAndPassword
 (redis ping included); existing postgres Collect tests updated to third PingFunc
 httpapi: TestStatusDefaultPostgresNotConfigured (redis not_configured);
 TestStatusPostgresUnavailableKeepsRedis (postgres unavailable, redis
 not_configured); TestStatusHealthyPostgresKeepsRedisNotConfigured (rewrite of
 TestStatusHealthyPostgresKeepsRedisNotImplemented);
 TestStatusRedisPingErrIndependentOfPostgres; TestStatusRedisPingOK;
 TestStatusRequiresSession (401 no components); TestHealthzUnchangedWithoutComponents
 frontend App.test.tsx: disconnectedStatus/mixedStatus no longer use redis
 not_implemented; mixed postgres Unavailable + Redis Reachable, Redis card
 visible; Redis Unavailable + status-card-redis; Redis Not configured; Redis
 Reachable; PgBouncer remains Not connected; unknown-state leftover still uses
 redis not_implemented; search redis_acl_users stays not_implemented
Integration tests: none. COMPATIBILITY.md §6 is not applicable and is not claimed.
 No live Redis INFO/DBSIZE/latency.
Security tests: canary rediss://:canary-secret@10.0.0.1:6379/0 absent from
 Load/Open errors; Memory PingErr canary absent from ErrUnavailable; Collect
 and HTTP status omit host/password; 401 has no components; GET is not audited;
 no-store unchanged.
Deployment/migration impact: new go.mod pin github.com/redis/go-redis/v9
 v9.22.0 (BSD-2-Clause, https://github.com/redis/go-redis). Production serve
 now fails closed without a usable Redis URL file (mirror postgres Open).
 Development may start without Redis keys. Application rollback is
 binary/config only. No migrations, package.json/lock, PRD, or COMPATIBILITY
 change.
Known limitations / residuals: REDIS-001 INFO/DBSIZE/latency, distinct
 auth vs NOPERM reasons, COMPATIBILITY §6, REDIS-002–008 ACL, search Redis
 hits, EXPECTED_SERIES, AUTH-005/006, PgBouncer probe, tool-link hrefs.
 Do not mark REDIS-001 or PLAT-001 Complete.
go-redis pin evidence (2026-08-25), worktree
 D:\code\github\Redgres-worktrees\redis-001-ping-status, branch
 redis-001-ping-status, baseline 921af37, go1.27.0 windows/amd64,
 Node v25.3.0 (web/.nvmrc pins 24.19.0):
  go list -m -versions github.com/redis/go-redis/v9
   → newest non-prerelease v9.22.0 (v9.22.0-beta.1 is prerelease)
  go list -m -json github.com/redis/go-redis/v9@v9.22.0
   → Version v9.22.0, Time 2026-08-03T17:39:49Z, Origin URL
   https://github.com/redis/go-redis, Hash c7f59a2a950eb5131cc27bfff716d6d3382e4490,
   Ref refs/tags/v9.22.0, GoVersion 1.24. LICENSE is BSD-2-Clause.
Commands executed locally (2026-08-25):
  gofmt -l internal/redisadmin internal/config internal/platform
   internal/httpapi cmd/redgres → empty
  go vet ./internal/redisadmin ./internal/config ./internal/platform
   ./internal/httpapi ./cmd/redgres → no findings
  go test -count=1 ./internal/redisadmin ./internal/config ./internal/platform
   ./internal/httpapi ./cmd/redgres
   → ok redisadmin 1.714s; config 0.569s; platform 0.451s; httpapi 7.696s;
   cmd/redgres 1.538s
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, redisadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  go build -o NUL ./cmd/redgres → success (no stdout)
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 77 passed (77);
   Duration 12.35s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 32 modules;
   ../internal/web/dist/app/{index.html, assets/index-Dn9WU1Ry.css 12.95 kB,
   assets/index-XMbfSLtm.js 229.13 kB}; built in 241ms
  git diff --name-only HEAD -- docs/PRD.md docs/COMPATIBILITY.md
   web/package.json web/package-lock.json → empty
  git status --porcelain → owned source/docs only (dist not stageable)
  Not run: go test -race ./..., gitleaks, govulncheck, CI, live Redis or
   PostgreSQL or PgBouncer, browser viewports, Playwright, frontend jobs on
   pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve; no Critical/High/Medium.
 Confirmed session + platform.read, 401 has no components, no-store, GET not
 audited, independent Redis Ping, platform does not import redisadmin/go-redis,
 Open does not Ping, logger discarded, file-path-only URL, production mode
 0o077, canary absent, payload has no version/uptime/host/DSN, search Redis
 group stays not_implemented. Low residual: production rediss:// may honor
 go-redis skip_verify; later reject. Reviewer did not run tests.
 UI review (2026-08-25) approve Overview Redis card; no High/Medium. Copy is
 Reachable / Unavailable / Not configured; PgBouncer stays Not connected;
 redis rail on status-card-redis; login never fetches /status; search Redis
 ACL stays not_implemented. Explicitly NOT viewport, zoom, or visual sign-off:
 no browser tools, app not opened. Lows (CSS class not-connected vs copy;
 rail assert only on Unavailable; leftover unknown-state redis
 not_implemented fixture) accepted unfixed.
 Verifier (2026-08-25) PASS on worktree HEAD `08529f4`. Re-ran gofmt empty,
 focused + ./... tests, vet, go build, npm test:run 77 passed, npm run build;
 forbidden paths empty vs `921af37`. Keep REDIS-001 Partial and PLAT-001
 Partial.
 Fast-forwarded `master` to `08529f4` (not pushed).
 Local commits: `08529f4` (feature).
```

## REDIS-001 metrics on GET /redis/status + Overview (2026-08-25)

```text
Requirement: REDIS-001 (Partial: Ping on GET /api/v1/status unchanged;
 metrics + distinct auth/permission/connectivity failures on GET
 /api/v1/redis/status + Overview). PLAT-001 stays Partial (PgBouncer
 not_implemented, tool_links not_configured). Do not mark Complete.
 Do not claim COMPATIBILITY.md §6.
Decision/ADR: ADR-001; platform.Collect and GET /status JSON unchanged.
 ADR-006: only Ping, INFO sections server/clients/memory/stats, DBSIZE.
 No ACL, no LATENCY command.
Source: redis-ui Adapter.Status Ping RTT + Info + DBSize; PublicRedisError
 not copied. go-redis v9.22.0 ParseURL skip_verify (options.go). Official
 INFO field names + DBSIZE of the selected DB.
Implementation files: internal/redisadmin/{errors,service,memory,adapter,
 service_test}.go; internal/config/{redis.go,redis_test.go};
 internal/httpapi/{server.go,redis_status_routes.go,redis_status_routes_test.go};
 web/src/api/redis.ts; web/src/features/overview/OverviewPage.tsx;
 web/src/App.test.tsx; web/src/styles/shell.css; docs/{API,ARCHITECTURE,
 CONFIGURATION,SECURITY,UX,UI_DESIGN_SYSTEM,TRACEABILITY}.md; AGENTS.md
Unit/HTTP tests: redisadmin Status all-or-nothing INFO/DBSIZE; classify
 NOAUTH/WRONGPASS → ErrAuthFailed, NOPERM → ErrPermissionDenied; skip_verify
 Open/Load reject; HTTP 401 no state/metrics/reason; 200 not_configured;
 200 ok eight metric keys including max_memory_bytes=0; three reasons;
 canary absent; no-store; 405; existing /status and healthz unchanged
Frontend App.test.tsx: both-ok metrics; three reasons; Reachable + Metrics
 unavailable without fake zeros; not_configured omits rows; Refresh hits
 both URLs with no query; isStatusUrl does not match /redis/status; login
 never fetches /status or /redis/status; PgBouncer Not connected; postgres
 Unavailable + Redis Reachable independent; /redis/status failure does not
 blank PostgreSQL cards; canary-secret not rendered; search Redis
 not_implemented
Integration tests: none. COMPATIBILITY.md §6 is not applicable and is not
 claimed. No live Redis.
Security tests: canary rediss://:canary-secret@10.0.0.1:6379/0?skip_verify=true
 absent from Load/Open/HTTP; Ping still maps client errors to ErrUnavailable;
 Status classifies internally then returns sentinels; 401 has no metrics;
 GET not audited; no-store unchanged.
Deployment/migration impact: skip_verify rejected in every environment
 (config URL-parse true|1 and adapter TLSConfig.InsecureSkipVerify after
 ParseURL). Application rollback is binary/config only. No migrations,
 package.json/lock, PRD, or COMPATIBILITY change.
Known limitations / residuals: REDIS-002–008 ACL, search Redis hits,
 EXPECTED_SERIES, AUTH-005/006, PgBouncer probe, tool-link hrefs.
 Latency is Ping RTT. db_size is selected-DB DBSIZE. jsdom cannot prove
 viewports or 200% zoom.
Commands executed locally (2026-08-25) after merging redis-001-metrics-api
 `6d3afc3` then redis-001-metrics-ui `83826dc` onto master `17e61e1`
 (merge commits `aa0a044`, `a5d88e2`), go1.27.0 windows/amd64, Node v25.3.0
 (web/.nvmrc pins 24.19.0):
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, redisadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 86 passed (86);
   Duration 14.93s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 33 modules;
   ../internal/web/dist/app/{index.html, assets/index-Cg5Btti4.css 13.52 kB,
   assets/index-CvMHoMLo.js 232.59 kB}; built in 264ms
  Not run: go test -race ./..., gitleaks, govulncheck, CI, live Redis or
   PostgreSQL or PgBouncer, browser viewports, Playwright, frontend jobs on
   pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve; no Critical/High/Medium.
 Confirmed session + redis.read, 401 has no state/metrics/reason, no-store,
 GET not audited, no CSRF, all-or-nothing metrics, typed reasons, skip_verify
 fail-closed (config true|1 and Open InsecureSkipVerify), platform does not
 import redisadmin/go-redis, Open does not Ping, search Redis stays
 not_implemented, login never fetches /status or /redis/status. Lows: config
 skip_verify parser is true|1 (matches go-redis v9.22.0; Open still
 fail-closes when InsecureSkipVerify is set); HTTP tests omit a dedicated
 WRONGPASS case (service covers it). Reviewer did not run tests.
 UI review (2026-08-25) approve Overview Redis metrics card; no High/Medium.
 Headline from /status; metrics/reasons from /redis/status; Reachable +
 Metrics unavailable without fake zeros; not_configured omits rows; Refresh
 both URLs; /redis/status failure does not blank PostgreSQL cards. Explicitly
 NOT viewport, zoom, or visual sign-off: no browser tools, app not opened.
 Lows (headline-only aria-label; degraded metrics no role=status; unknown
 reason suppression untested) accepted unfixed.
 Verifier (2026-08-25) PASS on master `2dc4d02`. Re-ran gofmt empty, focused
 + ./... tests, vet, go build, npm test:run 86 passed, npm run build
 (33 modules); forbidden paths empty vs `17e61e1`. Keep REDIS-001 Partial
 and PLAT-001 Partial.
 Local commits: `6d3afc3` (API), `83826dc` (UI), `aa0a044` (merge API),
 `a5d88e2` (merge UI), `2dc4d02` (docs). Not pushed.
```

## REDIS-002 ACL list and inspect (2026-08-25)

```text
Requirement: REDIS-002 (Partial: ACL list/inspect GET + UI). Do not mark
 Complete. PLAT-004 Redis ACL username hits landed later as a residual
 (2026-08-25). REDIS-001/PLAT-001 stay Partial. Do not claim
 COMPATIBILITY.md §6.
Decision/ADR: ADR-001; ADR-006 inspect-only (ACL LIST; no SETUSER; no
 deny-list grants). platform.Collect and GET /status unchanged.
Source characterization: redis-ui ListUsers/parseACLLine/InferPreset/
 IsProtectedUsername inspected read-only; silent skip of bad lines not
 copied. Official ACL LIST hash-in-line; ACL GETUSER not called (passwords
 field). go-redis v9.22.0 ACLList → ACL LIST.
Implementation files: internal/redisadmin/{acl.go,acl_test.go,presets.go,
 adapter.go,service.go,memory.go,errors.go,service_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go};
 web/src/api/redis.ts; web/src/features/redis/AclUsersPage.tsx;
 web/src/features/pages/Placeholders.tsx; web/src/App.test.tsx;
 web/src/styles/globals.css; docs/{API,ARCHITECTURE,UX,TRACEABILITY}.md;
 AGENTS.md
Unit/HTTP tests: official LIST lines; hash/> canary absent; +@read →
 custom+limited; exact v1 cache-read-write; protected visible; 501st
 truncated; classify NOAUTH/WRONGPASS/NOPERM; HTTP 401 no users/user;
 not_configured; ok list; detail 200; 404 Not found; 400 no echo; 405;
 no-store; /status and /redis/status unchanged
Frontend App.test.tsx: placeholder gone; list URL exactly /api/v1/redis/users;
 select /users/project_a; protected+limited badge; not_configured/unavailable/empty
 distinct; truncated role=alert; inspect-one + Back to users + inspector focus;
 detail unavailable auth_failed copy; 401; 404; canary not rendered; login does
 not fetch users; search Redis copy unchanged; no Storage.setItem; logout
 clears inspector
Integration tests: none. COMPATIBILITY.md §6 not claimed. No live Redis.
Security tests: canary #hash / >password absent; 401 omits users/user;
 protected visible not 404; GET not audited; no-store; no ACL GETUSER.
Deployment/migration impact: none. go.mod unchanged. Application rollback
 binary/config only.
Known limitations: inspect-only preset v1; limited ≠ expanded categories;
 list cap 500; no viewport/zoom sign-off; no create/rotate/delete;
 redis-presets placeholder. Redis ACL search hits landed later as PLAT-004
 residual (2026-08-25).
Commands executed locally (2026-08-25) after fast-forward API `46637d0`
 then merge UI `18fa511` as `f61be71`, remediations `26286e3`/`9ac5757`/`338d88d`,
 go1.27.0 windows/amd64, Node v25.3.0 (web/.nvmrc pins 24.19.0), verifier rerun
 on `338d88d`:
  gofmt -l redisadmin + redis users HTTP files → empty
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, redisadmin, web; migrations no test files
  go vet ./... → no findings (exit 0)
  go build -o NUL ./cmd/redgres → success
  go test -race -count=1 ./... → ok, same packages
  npm --prefix web run test:run → first run 1 failed / 104 passed (unrelated
   search-dialog focus flake: two role=status nodes); retry Test Files 3
   passed (3); Tests 105 passed (105); Duration 19.82s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 34 modules;
   ../internal/web/dist/app/{index.html, assets/index-FsGibsca.css 14.75 kB,
   assets/index-BgawEhPf.js 239.45 kB}; built in 826ms
  Not run: gitleaks, govulncheck, CI, live Redis, browser viewports,
   Playwright, frontend jobs on pinned Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve inspect-only list/GET +
 UI; no Critical/High/Medium. Confirmed session + redis.read, no CSRF,
 no-store, GET not audited, 401 has no state/users/user/reason, typed
 unavailable reasons, protected visible not 404, 400 no echo, password
 material stripped, ACLList only, platform does not import redisadmin/
 go-redis, login never fetches users, /status and /redis/status unchanged.
 Lows: HTTP canary omits !… / Redis <password removal form (parser skips;
 not a demonstrated leak); redis.read is a static owner grant.
 UI review (2026-08-25) approve Partial UI at `338d88d`; no Critical/High/
 Medium. Inspect-one below 1024px, 2-col 768–1023 / 4-col 1024+, Protected
 badge, truncation alert, typed detail unavailable. Explicitly NOT viewport,
 zoom, or visual sign-off. Optional polish (Back to users focus restore;
 inspector list max-height; detail not_configured copy) accepted unfixed.
 Verifier (2026-08-25) PASS on master `338d88d`. Re-ran gofmt empty, ./...
 tests, race, vet, go build, npm test:run 105 passed (after unrelated flake
 retry), npm run build (34 modules); forbidden paths empty vs `68d19fe`.
 Keep REDIS-002 Partial.
 Local commits: `46637d0` (API), `18fa511` (UI), `f61be71` (merge UI),
 `5d754f5` (docs), `26286e3`/`9ac5757`/`338d88d` (inspect-one remediations).
 Not pushed.
```

## PLAT-004 Redis ACL search hits (2026-08-25)

```text
Requirement: PLAT-004 (Partial residual). Authenticated GET /api/v1/search
 redis_acl_users maps redisadmin.Search (ACL LIST via ListUsers). Status is
 not_configured | ok | unavailable — never not_implemented. Hits only when
 ok: {id:"redis_acl_user:<username>", type:"redis_acl_user", label:"<username>"}.
 Empty hits are []. Protected usernames omitted from search even though
 GET /api/v1/redis/users still lists them. UI opens ACL inspect-one in memory.
 Keep PLAT-004 Partial (no docs corpus / deep links / command palette).
 REDIS-002 remains inspect-only. Do not mark Complete. Do not claim
 COMPATIBILITY.md §6.
Decision/ADR: ADR-001; ADR-006 inspect-only (ACL LIST only).
 platform.ResourceGroups(pg, redis) does not import postgresadmin/redisadmin.
 HTTP maps adapter sentinels; capability remains platform.read; GET not
 audited; no CSRF. No go.mod / go-redis bump.
Source characterization: redis-ui has no global search API (UserLedger is a
 local filter). Not edited.
Implementation files: internal/redisadmin/{service.go,service_test.go};
 internal/platform/{search.go,search_test.go};
 internal/httpapi/{server.go,search_routes.go,search_routes_test.go};
 web/src/components/search/NavigationSearch.tsx;
 web/src/components/shell/AppShell.tsx;
 web/src/features/pages/Placeholders.tsx;
 web/src/features/redis/AclUsersPage.tsx; web/src/App.test.tsx;
 docs/{API,ARCHITECTURE,UX,TRACEABILITY}.md; AGENTS.md
Unit/HTTP tests: redisadmin Search omits protected, case-insensitive,
 limit/truncation, nil not_configured, ACL errors without canary;
 platform Redis ok hits, postgres-down keeps redis, redis-unavailable keeps
 postgres, never not_implemented, hit fields only;
 httpapi nil redis not_configured, nil postgres still probes redis,
 Memory project_a hit, q=default omits protected while list still shows
 default, auth_failed keeps postgres hits, #hash/>password canaries absent,
 GET /status and /redis/users unchanged
Frontend App.test.tsx: redis hit inspects project_a; extra secret fields
 ignored; Unavailable ≠ not available yet; postgres unavailable still shows
 redis hits; count “1 matching ACL user.”; logout clears inspector; no
 Storage.setItem; Create absent; disconnectedSearch redis not_configured
Integration tests: none. COMPATIBILITY.md §6 not claimed. No live Redis.
Security tests: protected Redis usernames omitted from search; ACL canaries
 absent; 401 has no groups; no reason on search groups; hits only
 id/type/label; GET not audited; login never fetches /search
Deployment/migration impact: none. go.mod unchanged. Application rollback
 binary/config only. go-redis remains v9.22.0.
Known limitations: no docs corpus, URL deep links, or command-palette
 actions; ListUsers cap 500 applies to search; no viewport/zoom sign-off;
 REDIS-003–008 not started.
Commands executed locally (2026-08-25) after FF API `07303be` then merge UI
 `0756f87` as `7c3fa0e`, go1.27.0 windows/amd64, Node v25.3.0 (web/.nvmrc
 pins 24.19.0):
  go test -count=1 ./internal/platform ./internal/redisadmin ./internal/httpapi
   → PASS (parent after API FF)
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, redisadmin, web; migrations no test files
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 111 passed
   (111); Duration 15.95s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 34 modules;
   ../internal/web/dist/app/{index.html, assets/index-FsGibsca.css 14.75 kB,
   assets/index-B9gZve1k.js 240.65 kB}; built in 169ms
  Writer also ran gofmt/vet/build on API worktree and npm test 111 / build
   on UI worktree.
  Verifier (2026-08-25) on `b73a52c` additionally ran:
  gofmt -l touched Go → empty
  go test -count=1 ./internal/platform ./internal/redisadmin ./internal/httpapi
   → 173 pass / 0 fail
  go vet ./... → no findings
  go test -count=1 ./... → 298 pass / 0 fail / 1 skip
   (internal/web.TestOpenDoesNotFollowSymlinkOutOfRoot, pre-existing Windows)
  go build -o NUL ./cmd/redgres → success
  go test -race -count=1 ./... → ok
  npm --prefix web run test:run → Test Files 3; Tests 111 passed (111);
   Duration 17.93s; search-dialog role=status flake did not occur
  npm --prefix web run build → 34 modules; index-FsGibsca.css 14.75 kB;
   index-B9gZve1k.js 240.65 kB; built in 759ms
  git diff --name-only 546fffd -- go.mod go.sum web/package.json
   web/package-lock.json docs/PRD.md docs/COMPATIBILITY.md → empty
  Not run: gitleaks, govulncheck, CI, live Redis, browser viewports,
   Playwright, frontend jobs on Node 24.19.0
Reviewer/date: Security review (2026-08-25) approve; no Critical/High/Medium.
 Confirmed session + platform.read, 401 has no groups, no-store, GET not
 audited, no CSRF, hits id/type/label, protected omitted from search, canaries
 absent, ACL LIST only, platform does not import redisadmin. Lows accepted
 unfixed: future platform.read vs redis.read; search HTTP audit-count;
 HTTP adminUser omit.
 UI review (2026-08-25) approve Partial search hits UI; no blocking defects.
 Redis group mirrors postgres; inspect-one in memory; count includes ACL
 users; login never /search; secrets not rendered; Create absent. Explicitly
 NOT viewport, zoom, or visual sign-off.
 Evidence review (2026-08-25) keep Partial; REDIS-002 stale requirement
 line corrected in `b73a52c`.
 Verifier (2026-08-25) PASS on master `b73a52c`. Keep PLAT-004 Partial and
 REDIS-002 Partial.
 Local commits: `07303be` (API), `0756f87` (UI), `7c3fa0e` (merge UI),
 `9af4b46` (docs record), `b73a52c` (REDIS-002 residual wording).
 Not pushed.
```

## REDIS-003 create isolated ACL user (2026-08-25)

```text
Requirement: REDIS-003 (Partial: POST /api/v1/redis/users creates one
 isolated ACL user, always on + cache-read-write). Keep REDIS-003 Partial.
 REDIS-002 GET list/inspect unchanged (no CSRF, no audit). Do not mark
 Complete. Do not claim REDIS-004–008, live Redis, or COMPATIBILITY.md §6.
 No go-redis bump (stay v9.22.0).
Decision/ADR: ADR-001; ADR-006 allow-list (`-@all` + explicit +CMD from
 inspectCacheReadWrite). Production serve does not newly require
 REDGRES_REDIS_PUBLIC_HOST / REDGRES_REDIS_PUBLIC_PORT.
Source characterization: redis-ui CreateUser/BuildACLRules/NormalizePrefix/
 GeneratePassword/handleCreateUser inspected read-only at
 D:\code\github\redis-ui. Official Redis 8.2.2 / 8.8.0 acl.c + redis.io
 ACL SETUSER: SETUSER upserts (LIST first); resetchannels after reset is
 required; SETUSER modifier errors can include >password. go-redis v9.22.0
 ACLSetUser is NewStatusCmd(ctx, "acl", "setuser", username, rules...);
 local acl_commands.go SHA-256
 5F0E99517F9179DAF3F9B4395854F26E853532F0C02EE31CD9917133EEA94F5F.
 ACL GETUSER, ACL USERS, Do, ACLGenPass not used.
Implementation files: internal/redisadmin/{validate.go,validate_test.go,
 credentials.go,credentials_test.go,create_test.go,presets.go,errors.go,
 memory.go,adapter.go,service.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go};
 internal/config/{config.go,redis.go,redis_test.go};
 web/src/{App.tsx,App.test.tsx,api/redis.ts,
  components/shell/AppShell.tsx,features/pages/Placeholders.tsx,
  features/redis/{AclUsersPage,CreateAclUserForm,CredentialTicket}.tsx,
  styles/{globals,shell}.css};
 docs/{API,ARCHITECTURE,CONFIGURATION,SECURITY,DATA_AND_SECRETS,UX,
  TRACEABILITY}.md; AGENTS.md
Unit/HTTP tests: username/prefix validation; password 32 chars unique;
 rules contain reset/on/>/~:* /resetchannels/-@all/+PING or +GET;
 never +@all/ACL/CONFIG/FLUSHALL; protected; duplicate; ACLSetUser once;
 HTTP 201 no-store; CSRF; 401/403/400/409/503; unknown fields; GET still
 no audit; canary/password absent from audit JSON; GET after create has
 no password; public URL only when both host and port set; audit-fail
 after SETUSER returns 503 without credential; SETUSER modifier
 ERR containing >canary-secret is typed 503 with no canary in body
 or audit.
Frontend App.test.tsx: POST {username,key_pattern} + CSRF, no password;
 ticket shows password and ignores extra secrets; URL copy only when
 urls.primary present; dismiss clears password then inspects; ticket
 clears on logout/section/other inspect; Create hidden when
 not_configured/unavailable; login never POST /redis/users; no
 Storage.setItem.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node
 v25.3.0 (web/.nvmrc pins 24.19.0):
 Writer API worktree feat/redis-003-create-api `6a93136`:
  gofmt -w touched Go → success
  go test -count=1 ./internal/redisadmin ./internal/httpapi ./internal/config
   → ok redisadmin 1.591s; httpapi 11.530s; config 0.464s
  go test -count=1 ./... → ok; go vet ./... → no findings;
  go build -o NUL ./cmd/redgres → success
 Parent after API FF `6a93136` then UI merge `a79f67d`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi ./internal/config
   → ok redisadmin 1.850s; httpapi 11.839s; config 0.526s
  go test -count=1 ./... → ok cmd/redgres, audit, auth, config, database,
   httpapi, platform, postgresadmin, redisadmin, web; migrations no test files
  go vet ./... → no findings
  npm --prefix web run test:run → Test Files 3 passed (3); Tests 122 passed
   (122); Duration 19.93s (vitest 4.1.11)
  npm --prefix web run build → tsc --noEmit + vite v8.2.2; 36 modules;
   ../internal/web/dist/app/{index.html, assets/index-BmRbBVSX.css 15.65 kB,
   assets/index-BJ4gHUk2.js 245.33 kB}; built in 534ms
 Not run: live Redis, COMPATIBILITY §6, gitleaks, govulncheck, CI,
 browser viewports, Playwright, frontend jobs on Node 24.19.0
Known limitations: SETUSER-then-audit-fail residual (user exists, no
 credential returned); concurrent ACL LIST/SETUSER race (upsert);
 ticket alertdialog has no focus trap; 201 does not clear a prior
 inspector; no viewport/zoom sign-off; PRD on/off is on-only.
Reviewer/date: Security review (2026-08-25) approve Partial; no
 Critical/High/Medium. L1 SETUSER modifier HTTP canary added in
 `c6e70e6`. L2 MemoryClient plaintext ACLLines is test-double only.
 UI review (2026-08-25) approve Partial UI; no Critical/High. Medium
 ticket alertdialog without focus trap accepted unfixed. Medium leftover
 inspector after 201 accepted unfixed. Explicitly NOT viewport/zoom
 sign-off.
 Evidence review (2026-08-25) keep-Partial; no docs-correction-required.
 Verifier (2026-08-25) PASS Partial on master `c6e70e6`. Re-ran gofmt
 empty, focused + ./... tests, race, vet, go build, npm test:run 122
 passed, npm run build (36 modules; index-BJ4gHUk2.js 245.33 kB).
 Forbidden paths empty vs `c322826`. Keep REDIS-003 Partial.
 Local commits: `6a93136` (API), `9652adf` (UI), `a79f67d` (merge UI),
 `016bae3` (docs record), `c6e70e6` (SETUSER canary). Not pushed.
```

## REDIS-007 enable/disable ACL users (2026-08-25)

```text
Requirement: REDIS-007 (Partial: POST enable/disable only; no rotate).
 Keep REDIS-007 Partial. REDIS-003 create and REDIS-002 GET unchanged.
 Do not mark Complete. Do not claim REDIS-004–006/008, live Redis,
 or COMPATIBILITY.md §6. go-redis stays v9.22.0.
Decision/ADR: ADR-001; ADR-006 unused for grants (on/off only).
 AUTH-006 does not apply (delete only).
Source: redis-ui SetEnabled / handleEnableUser / handleDisableUser
 at D:\code\github\redis-ui (read-only). Official Redis 8.2.2 / 8.8.0
 ACL SETUSER on/off are flag-only (no reset). go-redis v9.22.0
 ACLSetUser unchanged. SETUSER upserts: GetUser/LIST first.
Implementation files: internal/redisadmin/{service.go,memory.go,
 enable_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,
 redis_users_routes_test.go};
 web/src/{api/redis.ts,features/redis/AclUsersPage.tsx,App.test.tsx};
 docs/{API,ARCHITECTURE,SECURITY,UX,TRACEABILITY}.md; AGENTS.md
Unit/HTTP: on/off-only SETUSER; permissions preserved in MemoryClient
 merge; protected; missing; limited/custom toggle; canary; CSRF; 401/
 400/403/404/503; audit username only; audit-fail after SETUSER;
 GET/PATCH 405 on enable paths; POST /users/{username} still 405;
 create/GET regressions.
Frontend: Enable/Disable inspector; CSRF empty body; no storage;
 protected hidden; session/404/503; create/ticket unchanged.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API worktree feat/redis-007-enable-api `beaa606`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi → ok
  go test -count=1 ./... → ok; go vet ./... → no findings;
  go build -o NUL ./cmd/redgres → success
 Parent after API FF `beaa606`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi → ok
 Writer UI `8158c6a`: npm --prefix web run test:run → 138 passed
 Parent after UI merge `6ecd075` + docs:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.371s; httpapi 13.250s
  go vet ./... → no findings
  npm --prefix web run test:run → Tests 138 passed (138)
Not run: live Redis, §6, gitleaks, govulncheck, CI, Playwright,
 viewport/zoom, Node 24.19.0
Known limitations: SETUSER-then-audit-fail leftover toggle;
 GetUser/SETUSER recreate race; disable does not kill connections;
 no rotate; no viewport sign-off; REDIS-003 residuals unchanged.
Reviewer/date: Security review (2026-08-25) approve Partial; no
 Critical/High/Medium. L1 GetUser/SETUSER recreate race accepted
 unfixed. UI review (2026-08-25) approve Partial UI; no Critical/High.
 Explicitly NOT viewport/zoom sign-off. Evidence review (2026-08-25)
 docs-correction-required then keep-Partial (`014e0dc`).
 Verifier (2026-08-25) PASS Partial on master `85586b8`. Re-ran gofmt
 empty, focused + ./... tests, race, vet, go build, npm test:run 138
 passed, npm run build. go-redis stays v9.22.0. Forbidden paths empty
 vs `66c4053`. Keep REDIS-007 Partial (rotate missing).
 Local commits: `beaa606` (API), `8158c6a` (UI), `6ecd075` (merge UI),
 `8eac4eb` (docs record), `74af2c4` (parent test results), `014e0dc`
 (catalog implemented), `c45f05f` (security approve), `85586b8`
 (UI approve). Not pushed.
```

## REDIS-007 rotate ACL password (2026-08-25)

```text
Requirement: REDIS-007 (Partial: POST enable/disable + rotate; no delete/PATCH).
 Keep REDIS-007 Partial until live Redis / §6 / viewport exist. Do not mark
 Complete. REDIS-003 create and REDIS-002 GET unchanged. REDIS-004–006/008
 not started. go-redis stays v9.22.0.
Decision/ADR: ADR-001; ADR-006 unused for rotate grants (resetpass +
 >password only). AUTH-006 does not apply (delete only). Capability is
 redis.credentials, not redis.destructive.
Source: redis-ui Adapter.Rotate / handleRotateUser at D:\code\github\redis-ui
 (read-only). Redgres generates GeneratePassword() in the service, not HTTP.
 Official Redis ACL SETUSER: resetpass clears passwords/nopass; >password adds
 a new secret; SETUSER upserts so GetUser/LIST is mandatory
 (https://redis.io/commands/acl-setuser). go-redis v9.22.0 ACLSetUser unchanged.
Implementation files: internal/redisadmin/{service.go,memory.go,rotate_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go};
 web/src/{api/redis.ts,features/redis/AclUsersPage.tsx,
 features/redis/RotatePasswordDialog.tsx,App.test.tsx};
 docs/{API,ARCHITECTURE,SECURITY,UX,TRACEABILITY}.md; AGENTS.md
 credentials.go reused (GeneratePassword, ProjectConnectionURL).
Unit/HTTP: resetpass+> only; grants/on-off/prefix/channels preserved via
 MemoryClient merge (strip #/>/!/nopass, do not replace the line);
 protected/missing no SETUSER; limited/custom/disabled rotatable; inspect
 omits >canary; SETUSER errors classified without ERR/>password; 200 envelope
 + one_time; optional urls.primary; CSRF/401/400/403/404/503; GET/PATCH/PUT/
 DELETE 405; no POST .../rotate alias; extra body ignored; audit username
 only; audit-fail after SETUSER returns no credential/password/user;
 create/enable/GET regressions; Cache-Control no-store.
Frontend: inspector Rotate text-button (not danger); confirm “Rotate password?”;
 CSRF empty body encodeURIComponent; existing CredentialTicket via
 parseCredential; extra secret fields ignored; hidden for protected /
 degraded / loading; disabled in flight or while ticket open; 401/404/403/503
 copy; dismiss stays on user and refreshes list+detail; no storage.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer rotate API worktree feat/redis-007-rotate-api `711e0f3`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.558s; httpapi 13.020s
  go test -count=1 ./... → ok; go vet ./... → no findings;
  go build -o NUL ./cmd/redgres → success
 Parent review of API then FF onto master `711e0f3`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.644s; httpapi 14.604s
 Writer UI feat/redis-007-rotate-ui `74cd219` (writer-attributed, Node v25.3.0;
 web/.nvmrc pins 24.19.0):
  npm --prefix web test -- --run → Tests 154 passed (154)
  npm --prefix web ci → 113 packages (writer)
  npm --prefix web run build → tsc + vite 8.2.2 (writer; dist gitignored)
 Parent after UI merge `2b1fca5` + docs `c150289`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.045s; httpapi 16.661s
  npm --prefix web test -- --run → Tests 154 passed (154)
  go vet ./... → no findings; go build -o NUL ./cmd/redgres → success
Not run by parent: live Redis, COMPATIBILITY.md §6, gitleaks, govulncheck,
 CI, Playwright, npm production build, viewport/zoom, Node 24.19.0
Known limitations: SETUSER-then-audit-fail leftover password (not returned);
 GetUser/SETUSER recreate race; MemoryClient stores >password (real Redis
 hashes); no viewport sign-off; REDIS-003 residuals unchanged.
 UI Medium residuals (not reject): confirm sheet does not name the ACL user;
 focus does not return to Rotate; Escape/search can cover the confirm;
 ticket focus-trap still absent (REDIS-003).
Reviewer/date: Security review (2026-08-25) approve Partial; no
 Critical/High/Medium. Reject gates absent. UI review (2026-08-25) approve
 Partial UI; no Critical/High. Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) keep-Partial; no required corrections.
 Writer-only ./... / vet / build and UI npm ci/build are not parent-after-merge
 re-runs.
 Verifier (2026-08-25) PASS Partial on master `85c75af`. Independent re-runs
 go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.636s; httpapi 14.803s
  go test -count=1 ./... → ok
  go test -race -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 3.126s; httpapi 20.332s
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Tests 154 passed (154)
  npm --prefix web run build → tsc + vite 8.2.2
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Forbidden paths empty vs `b4a57ba` (no go.mod/go.sum, siblings, secrets).
 Not executed: live Redis, §6, full-tree race, viewport/Playwright, gitleaks,
 govulncheck, CI, npm ci, Node 24.19.0.
 Local commits: `711e0f3` (API), `74cd219` (UI), `2b1fca5` (merge UI),
 `c150289` (docs record), `85c75af` (review pin). Not pushed.
 Keep REDIS-007 Partial.
```

## REDIS-004 named-preset create + catalog (2026-08-25)

```text
Requirement: REDIS-004 (Partial: named-preset create + GET /api/v1/redis/presets).
 Keep REDIS-004 Partial until live Redis representative workloads /
 COMPATIBILITY.md §6 / viewport exist. Do not mark Complete.
 REDIS-003 create path now accepts preset; omitted/empty defaults to
 cache-read-write. REDIS-005 custom, REDIS-006 PATCH, REDIS-008 delete
 not started. REDIS-007 stays Partial. go-redis stays v9.22.0.
Decision/ADR: ADR-001; ADR-006 (-@all + explicit +CMD from inspect* sets).
 AUTH-006 does not apply. GET presets is static NamedPresets(); no Redis.
Source: redis-ui CommandsForPreset / BuildACLRules / CreateUser /
 handleCreateUser / UserForm.tsx at D:\code\github\redis-ui (read-only).
 Custom deny-list + arbitrary commands were not copied. inspect* membership
 unchanged. Compatibility research (2026-08-25): all 115 inspect* names exist
 on Redis 8.2.2 and 8.8.0 official src/commands JSON and are ACL-grantable;
 eight deprecated-but-present commands stay in the frozen sets;
 XNACK (8.8), XDELEX (8.2), and XACKDEL (8.2) were not added.
Implementation files: internal/redisadmin/{service.go,presets.go,errors.go,
 create_test.go,presets_catalog_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go,
 redis_presets_routes_test.go};
 web/src/{api/redis.ts,features/redis/CreateAclUserForm.tsx,
 features/redis/AclUsersPage.tsx,App.test.tsx,styles/globals.css};
 docs/{API,ARCHITECTURE,SECURITY,UX,TRACEABILITY}.md; AGENTS.md
Unit/HTTP: five named SETUSER +CMD sets equal inspect slices; omitted
 preset = cache-read-write; custom/unknown preset; queue_kind mismatch;
 extra fields/password; GET presets 401/405/catalog equality/nil adapter
 200/no Redis client; CSRF/401/403/409/503; canary SETUSER ERR; audit-fail
 after SETUSER; create/enable/rotate/GET regressions.
Frontend: Preset select defaults Cache read/write; Queue type only for
 queue-worker; POST {username, key_pattern, preset}; queue_kind only when
 queue-worker; never password/commands/custom; ticket unchanged; create
 hidden when degraded. Create does not fetch GET /presets. Nested
 Permission presets nav item remains the Wave 0 placeholder (does not
 call the catalog).
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API worktree feat/redis-004-presets-api `8846d7f`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.604s; httpapi 16.260s
  go test -count=1 ./... → ok; go vet ./... → no findings;
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent review of API then FF onto master `8846d7f`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.617s; httpapi 15.040s
 Writer UI feat/redis-004-presets-ui `b7f905f` (Node v25.3.0):
  npm --prefix web run test:run → Tests 155 passed (155)
  npm --prefix web run build → tsc + vite 8.2.2 (writer; dist gitignored)
 Parent after UI merge `7ca8c93` + this docs commit:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.223s; httpapi 17.176s
  npm --prefix web test -- --run → Tests 155 passed (155)
  go vet ./... → no findings; go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
Not run by parent: live Redis, COMPATIBILITY.md §6, gitleaks, govulncheck,
 CI, Playwright, npm production build, viewport/zoom, Node 24.19.0
Known limitations: SETUSER-then-audit-fail leftover user; LIST/SETUSER
 race; no representative workload tests; no viewport sign-off; REDIS-003/007
 UI Medium residuals unchanged.
 UI Medium residuals this slice (not reject): new selects lack shared
 --focus ring; / and Ctrl/Cmd+K still fire on a focused select.
Reviewer/date: Security review (2026-08-25) approve Partial; no
 Critical/High/Medium. Reject gates held. UI review (2026-08-25) approve
 Partial UI; no Critical/High. Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) keep-Partial; no required corrections.
 Writer-only ./... / vet / build and UI npm build are not parent-after-merge
 re-runs of those exact commands.
 Verifier (2026-08-25) PASS Partial on master `651ecb1`. Independent re-runs
 go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.067s; httpapi 15.470s
  go test -count=1 ./... → ok (serial after dist settled)
  go test -race -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 3.102s; httpapi 21.570s
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 155 passed (155)
  npm --prefix web run build → tsc + vite 8.2.2
 inspect* membership unchanged vs `343f807`; no XNACK/XDELEX/XACKDEL.
 Forbidden paths empty vs `343f807` (no go.mod, siblings, secrets).
 Not executed: live Redis, §6, representative workloads, viewport/Playwright,
 gitleaks, govulncheck, CI, Node 24.19.0.
 Local commits: `8846d7f` (API), `b7f905f` (UI), `7ca8c93` (merge UI),
 `1d48e8f` (docs record), `651ecb1` (review pin). Not pushed.
 Keep REDIS-004 Partial.
```

## REDIS-006 named-preset PATCH (2026-08-25)

```text
Requirement: REDIS-006 (Partial: PATCH named-preset prefix/grants; password
 preserved). Keep REDIS-006 Partial until live Redis / COMPATIBILITY.md §6 /
 viewport exist. Do not mark Complete. Custom PATCH is REDIS-005. REDIS-008 /
 AUTH-006 not started. REDIS-003/004/007 stay Partial. go-redis stays v9.22.0.
 inspect* unchanged.
Decision/ADR: ADR-001; ADR-006 (-@all + explicit +CMD). AUTH-006 does not apply.
 SETUSER: resetkeys ~pattern resetchannels nocommands -@all +CMD; no
 reset/resetpass/>/on/off. Capability redis.provision, not redis.destructive.
Source: redis-ui UpdatePermissions / handleUpdateUser / UserForm.tsx at
 D:\code\github\redis-ui (read-only). Custom deny-list + commands/categories
 were not copied.
Implementation files: internal/redisadmin/{service.go,memory.go,update_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go};
 web/src/{api/redis.ts,features/redis/AclUsersPage.tsx,
 features/redis/EditPermissionsDialog.tsx,App.test.tsx};
 docs/{API,ARCHITECTURE,SECURITY,UX,TRACEABILITY}.md; AGENTS.md
Unit: five named SETUSER vectors = inspect slices; no reset/resetpass/>/
 on/off (assertUpdateRules); enabled+hash preserved; custom/limited/disabled
 updatable; protected/missing no SETUSER; empty preset does not default.
 HTTP: +CMD set equals NamedPresets(); CSRF/401/403/400/404/503; unknown
 fields; PATCH collection 405; PUT/DELETE username 405; audit-fail after
 SETUSER no user; create/enable/rotate/GET/presets regressions. HTTP named-
 preset tests do not fail if reset/resetpass/> were added to SETUSER.
Frontend: Edit permissions text-button (not danger); same visibility as
 Enable/Rotate; no Custom; custom inspect defaults cache-read-write;
 queue_kind only for queue-worker; PATCH CSRF encodeURIComponent
 {key_pattern, preset [, queue_kind]}; 200 applies inspector+row, no ticket;
 401/403/404/503 copy; no storage; login never PATCH; never GET /presets.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API worktree feat/redis-006-patch-api `0a8d9b2`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.693s; httpapi 17.019s
  go test -count=1 ./... → ok; go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent review of API then FF onto master `0a8d9b2`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.609s; httpapi 16.375s
 Writer UI feat/redis-006-patch-ui `418c473` (Node v25.3.0):
  npm --prefix web run test:run → Tests 173 passed (173)
  npm --prefix web run build → tsc + vite 8.2.2 (writer; dist gitignored)
 Parent after UI merge `d410034` + this docs commit:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.918s; httpapi 18.378s
  npm --prefix web test -- --run → Tests 173 passed (173)
  go vet ./... → no findings; go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
Not run by parent: live Redis, COMPATIBILITY.md §6, gitleaks, govulncheck,
 CI, Playwright, npm production build, viewport/zoom, Node 24.19.0.
 Writer API ./... / vet / build are writer-attributed, not parent-after-merge
 re-runs of those exact commands.
Known limitations: SETUSER-then-audit-fail leftover grants; GetUser/SETUSER
 race; MemoryClient plaintext > residual unchanged; REDIS-003/004/007 UI
 Medium residuals unchanged; no representative workloads.
 Security Medium (not reject): Redis 7+ ACL selectors survive named-preset
 PATCH because SETUSER omits clearselectors (root-only resetkeys/nocommands;
 same as redis-ui). Later slice: clearselectors or refuse SETUSER when LIST
 shows selector parentheses. Do not use reset.
 UI Medium (inherited, not reject): focus does not return to Edit permissions
 (same as create/rotate trap).
Reviewer/date: Security review (2026-08-25) approve Partial; no Critical/High.
 One Medium (selectors). UI review (2026-08-25) approve Partial UI; no
 Critical/High. Explicitly NOT viewport/zoom sign-off. Evidence review
 (2026-08-25) keep-Partial; no required corrections. Token-forbid evidence
 is unit-only.
 Verifier (2026-08-25) PASS Partial on master `782e998`. Independent re-runs
 go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.612s; httpapi 15.203s
  go test -count=1 ./... → ok
  go test -race -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 3.147s; httpapi 25.827s
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success (after npm build settled dist)
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 173 passed (173)
  npm --prefix web run build → tsc + vite 8.2.2
 inspect* membership unchanged vs `74e41a2`; go.mod unchanged.
 Forbidden paths empty vs `74e41a2` (no siblings, secrets).
 Not executed: live Redis, §6, representative workloads, viewport/Playwright,
 gitleaks, govulncheck, CI, Node 24.19.0, clearselectors.
 Local commits: `0a8d9b2` (API), `418c473` (UI), `d410034` (merge UI),
 `421a522` (docs record), `782e998` (review pin). Not pushed.
 Keep REDIS-006 Partial.
```

## REDIS-005 custom PATCH allow-list (2026-08-25)

```text
Requirement: REDIS-005 (Partial: custom PATCH through AllowedCommands() +
 GET /api/v1/redis/commands + inspector Custom checklist). Keep REDIS-005
 Partial until live Redis / COMPATIBILITY.md §6 / viewport exist. Do not
 mark Complete. No POST create custom. No categories. No REDIS-008 /
 AUTH-006. Named PATCH (REDIS-006) unchanged except commands is a known
 field (omitted/empty OK; non-empty on a named preset → 400
 fields.commands). POST create custom still 400. go-redis stays v9.22.0.
 inspect* unchanged vs 74e41a2.
Decision/ADR: ADR-001; ADR-006 (fail-closed allow-list, not deny-list).
 SETUSER: resetkeys ~pattern resetchannels nocommands -@all +CMD; no
 reset/resetpass/>/on/off/clearselectors. Capability redis.provision.
 AllowedCommands() = unique-sorted union of NamedPresets()[].Commands.
 Audit redis.user.update: username, preset, key_pattern only — not the
 command list. GET /commands: session + redis.read, no CSRF, no Redis,
 no audit.
Source: redis-ui UpdatePermissions SETUSER shape at
 D:\code\github\redis-ui\internal\redisadmin\adapter.go (~203–228),
 read-only. Do not port deniedAlways / AssertSafeCommands / custom
 CommandsForPreset (deny-list + arbitrary names).
Implementation files: internal/redisadmin/{presets.go,service.go,errors.go,
 update_test.go,allowlist_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go,
 redis_commands_routes_test.go};
 web/src/{api/redis.ts,features/redis/AclUsersPage.tsx,
 features/redis/EditPermissionsDialog.tsx,App.test.tsx,
 styles/globals.css};
 docs/{API,ARCHITECTURE,SECURITY,UX,UI_DESIGN_SYSTEM,TRACEABILITY}.md;
 AGENTS.md
 TRACEABILITY.md / AGENTS.md owned by parent after writer commits.
Unit: AllowedCommands == NamedPresets union; disjoint from test-only
 dangerous fixture (acl, config, debug, module, shutdown, flushall,
 flushdb, eval, evalsha, script, keys, @all). Custom vectors: CRW
 subset, connection-safe only, queue-lists subset. Named+commands and
 custom invalid (@all/+/flushall/acl/empty/unknown) reject before Redis
 (ACLListErr still 400, no SETUSER). inspect* == 74e41a2.
 HTTP: GET /commands 200 byte-equal AllowedCommands, never null, no
 state/reason/preset, no Redis, no audit; 401 has no commands key;
 other methods 405; Cache-Control no-store. PATCH custom 200; flushall/
 @all/acl/empty/named+commands 400 fields.commands; POST create custom
 still 400; CSRF/401/403/404/503; SETUSER-then-audit-fail 503 no user;
 captured SETUSER has no reset/resetpass/>/on/off. Named PATCH /
 enable / rotate / GET / presets regressions.
Frontend: Edit dialog Custom only (create stays named-only). GET
 /commands when dialog opens / preset becomes custom; no CSRF. Failure
 disables Save and invents no fallback list. Checklist from catalog.
 Prefill inspect ∩ allow-list; unknown names dropped. namedPreset
 returns custom (does not default custom inspect to cache-read-write).
 PATCH {key_pattern, preset: custom, commands}; no ticket, no password,
 no queue_kind. Login never GET /commands. Create never sends commands.
 Stacked `.command-checklist` (one row, 44px hit area, `--line` /
 `--radius-surface`); catalog errors do not mark Key prefix invalid.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API worktree feat/redis-005-allowlist-api `00549ec`:
  gofmt -w <touched Go>
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.580s; httpapi 19.529s
  go test -count=1 ./... → ok; go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent review of API then FF onto master `00549ec`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.588s; httpapi 19.749s
 Writer UI feat/redis-005-allowlist-ui `b07c0de` (Node v25.3.0):
  npm --prefix web run test:run → Tests 177 passed (177) (writer worktree)
  npm --prefix web run build → tsc + vite 8.2.2 (writer; dist gitignored)
 Parent after UI merge `76299b0` + docs `ed64500`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.205s; httpapi 20.405s
  go test -count=1 ./... → ok
  npm --prefix web run test:run → Tests 177 passed (177)
  go vet ./... → no findings; go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent after UI High/Medium/Low correction `9f81257`:
  npm --prefix web run test:run → Tests 178 passed (178)
  (Go tests not re-run; CSS/dialog/docs only.)
Not run by parent: live Redis, COMPATIBILITY.md §6, gitleaks, govulncheck,
 CI, Playwright, npm production build, viewport/zoom, Node 24.19.0,
 go test -race.
 Writer API ./... / vet / build and writer UI npm build are
 writer-attributed, not parent-after-merge re-runs of those exact
 commands (parent did re-run Go ./... / vet / build after UI merge).
Known limitations: SETUSER-then-audit-fail leftover grants; GetUser/SETUSER
 race; MemoryClient plaintext > residual unchanged; Redis 7+ ACL selectors
 survive PATCH (no clearselectors; inherited Medium); no POST custom;
 if custom set equals a named inspect set, inferPreset may return that
 named preset (expected). CreateUser arity unchanged.
Reviewer/date: Security review (2026-08-25) on `ed64500` approve Partial;
 no Critical/High/Medium/Low this-slice; no deny-list mistake; no
 allow-list hole. Inherited selector leftover and SETUSER-then-audit-fail
 stay out of scope.
 UI review (2026-08-25) on `ed64500` approve Partial UI; High wrapping
 catalog labels, Medium 44px hit area, Low UA fieldset chrome + prefix
 aria-invalid on catalog errors. Parent closed those on `9f81257`.
 UI re-check (2026-08-25) on `9f81257` approve Partial UI; High/Medium/Low
 closed; no remaining Critical/High. Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) on `ed64500` keep-Partial; nine mapped
 claims supported; no required corrections. API.md unique-sort vs raw
 len>256 sentence aligned in `d6a5b69`.
 Verifier (2026-08-25) PASS Partial on master `e595b1c`. Independent
 re-runs go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l internal/redisadmin internal/httpapi → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.591s; httpapi 16.409s
  go test -count=1 ./... → ok
  go test -race -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 3.371s; httpapi 26.705s
  go vet ./... → no findings (after npm build dist settled)
  go build -o NUL ./cmd/redgres → success (after dist settled)
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 178 passed (178)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 inspect* membership unchanged vs `74e41a2`; go.mod unchanged vs
 `6ac634f`. Forbidden paths empty (21 files, +1380/−111; no siblings,
 secrets, .env).
 Not executed: live Redis, §6, representative workloads, viewport/
 Playwright, gitleaks, govulncheck, CI, Node 24.19.0, clearselectors.
 Local commits: `00549ec` (API), `b07c0de` (UI), `76299b0` (merge UI),
 `ed64500` (docs record), `9f81257` (checklist layout), `d6a5b69`
 (review pin), `e595b1c` (UI re-check pin), this verifier record.
 Not pushed.
 Keep REDIS-005 Partial.
```

## REDIS-005 POST create custom allow-list (2026-08-25)

```text
Requirement: REDIS-005 (Partial residual: POST /api/v1/redis/users
 accepts preset=custom plus commands ⊆ AllowedCommands(); Create
 dialog Custom checklist). Keep REDIS-005 Partial until live Redis /
 COMPATIBILITY.md §6 / viewport exist. Do not mark Complete. No
 categories. No POST enabled/password. Create stays on-only.
 REDIS-008 / AUTH-006 not started. Named create (REDIS-003/004)
 unchanged except commands is a known field (omitted/empty OK;
 non-empty on named/defaulted preset → 400 fields.commands).
 go-redis stays v9.22.0. inspect* unchanged vs 74e41a2.
Decision/ADR: ADR-001; ADR-006 (fail-closed allow-list, not deny-list).
 Create SETUSER: reset on >password ~prefix:* resetchannels -@all +CMD;
 no nocommands (PATCH-only); no +@all; no clearselectors. Capability
 redis.provision. AllowedCommands() unchanged. Audit redis.user.create:
 username, preset (actual/inferred), key_pattern, plus queue_kind only
 when actual preset is queue-worker — never the command list.
 GET /commands unchanged. GET /presets still has no custom row.
Source: redis-ui CommandsForPreset / AssertSafeCommands / deniedAlways
 at D:\code\github\redis-ui (read-only ANTI-PATTERN). Not ported.
Implementation files: internal/redisadmin/{service.go,create_test.go,
 allowlist_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,
 redis_users_routes_test.go,redis_commands_routes_test.go};
 web/src/{api/redis.ts,features/redis/CreateAclUserForm.tsx,
 features/redis/AclUsersPage.tsx,App.test.tsx};
 docs/{API,ARCHITECTURE,SECURITY,UX,UI_DESIGN_SYSTEM,TRACEABILITY}.md;
 AGENTS.md
 TRACEABILITY.md / AGENTS.md owned by parent after writer commits.
Unit/HTTP: custom 201 SETUSER reset/on/>/~/resetchannels/-@all/+CMD,
 no nocommands/flushall/acl/@all; named + non-empty commands 400
 fields.commands before Redis (ACLListErr unused); custom
 empty/omitted/[]/flushall/@all/acl 400 fields.commands before Redis;
 named omitted/[] and {username,key_pattern} still 201; inferPreset
 may label custom grants as a named preset; audit success/failure omit
 command list/password/>; SETUSER-then-audit-fail 503, no credential;
 CSRF/401/403/409/503 plus PATCH custom + GET /commands + named PATCH
 + enable + rotate regressions. inspect* == 74e41a2.
Frontend: Create dialog Custom option; GET /commands (cookie, no CSRF)
 when preset is Custom; no inspect prefill; empty/catalog-fail disables
 Create, no invented list; POST {username,key_pattern,preset:custom,
 commands} CSRF, no queue_kind/password; named POST omits commands;
 201 still ticket; login never GET /commands; Edit Custom unchanged.
 stacked .command-checklist reused.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API feat/redis-005-create-custom-api `c333c00`:
  gofmt -l touched Go → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.643s; httpapi 22.397s
  go test -count=1 ./... → ok; go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  TestInspectSetsEqual74e41a2 → ok
 Parent review of API then FF onto master `c333c00`:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 1.599s; httpapi 18.479s
 Writer UI feat/redis-005-create-custom-ui `0f947d2` (Node v25.3.0):
  npm --prefix web run test:run → Tests 184 passed (184)
  npm --prefix web run build → tsc + vite (writer; dist gitignored)
 Parent after UI merge `e2a494b` + this docs commit:
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.041s; httpapi 21.361s
  go test -count=1 ./... → ok
  npm --prefix web run test:run → Tests 184 passed (184)
  go vet ./... → no findings; go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
Not run by parent: live Redis, COMPATIBILITY.md §6, gitleaks, govulncheck,
 CI, Playwright, npm production build, viewport/zoom, Node 24.19.0,
 go test -race.
Known limitations: SETUSER-then-audit-fail leftover user; GetUser/SETUSER
 race; MemoryClient plaintext > residual; Redis 7+ PATCH selectors
 (inherited Medium); no categories; if custom set equals a named inspect
 set, inferPreset may return that named preset (expected).
Reviewer/date: Security review (2026-08-25) on `f9457de` approve Partial;
 no Critical/High/Medium/Low this-slice; no deny-list mistake; no
 allow-list hole. Inherited SETUSER-then-audit-fail leftover user,
 GetUser race, MemoryClient >, PATCH selector leftover stay out of
 scope.
 UI review (2026-08-25) on `f9457de` approve Partial UI; no
 Critical/High/Medium. Optional Low: catalog 401/503 alert can remain
 after switching Custom → named (named Create still enabled; not
 required). Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) on `f9457de` keep-Partial; nine mapped
 claims supported; no required corrections. PRD category expansion
 still out of slice.
 Verifier (2026-08-25) PASS Partial on master `feaba38`. Independent
 re-runs go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 2.602s; httpapi 18.731s
  go test -count=1 ./... → ok (re-run after npm build dist settled;
  first ./... and vet collided with hashed embed)
  go test -race -count=1 ./internal/redisadmin ./internal/httpapi
   → ok redisadmin 3.144s; httpapi 26.823s
  go vet ./... → no findings (after dist settled)
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 184 passed (184)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 go.mod/go.sum unchanged vs `ed3c682`. Forbidden paths empty (18 files;
 no siblings, secrets, .env).
 Not executed: live Redis, §6, representative workloads, viewport/
 Playwright, gitleaks, govulncheck, CI, Node 24.19.0, categories,
 REDIS-008, AUTH-006.
 Local commits: `c333c00` (API), `0f947d2` (UI), `e2a494b` (merge UI),
 `f9457de` (docs record), `feaba38` (review pin), this verifier record.
 Not pushed.
 Keep REDIS-005 Partial. Not pushed.
 Keep REDIS-005 Partial.
```

## PostgreSQL cluster security overview (2026-08-25)

```text
Requirement: PG-012 Partial (cluster GET + Security overview UI; no vault entries, rotation, or create)
Decision/ADR: ADR-001, ADR-004 (vault not implemented)
Source characterization: database-app get_security_overview at 1c3e8e2
 (connection grouping); Redgres lists all non-template DBs including
 postgres/database_console_vault; JSON key name; saved_credential always
 vault_not_implemented; no project_credentials query
Implementation files: internal/postgresadmin/{types,service,memory,adapter}.go;
 internal/httpapi/{server,postgres_routes}.go;
 web/src/api/postgres.ts (fetchPostgresSecurity + types);
 web/src/features/postgres/SecurityOverview.tsx;
 web/src/features/pages/Placeholders.tsx (postgres-security only);
 web/src/App.test.tsx; docs/UX.md
Unit tests: internal/postgresadmin/service_test.go (templates omitted,
 protected included, List still filters, nil catalog, no vault SQL, cap/
 truncated, connection labels, canary);
 internal/httpapi/postgres_security_routes_test.go (401/405/503/200 shape,
 arrays non-null, cap, canary absent, list/details still 404);
 App.test.tsx loading / 503 / 401 / empty-200 / protected vs project /
 vault Not available / connections table / truncated / login never GET /
 no localStorage
Integration tests: none run (no live PostgreSQL claimed; GET mocked in UI)
Security tests: 401 omits summary/databases/connections/saved_credential/
 truncated; canary redacted; no-store; vault_not_implemented without vault SQL;
 401 body keys ignored in UI; 503 ≠ empty healthy; no CSRF; identifiers
 displayText + bidi isolate; no localStorage/sessionStorage; no rotate/create/reveal
Deployment/migration impact: none. 001_initial.sql and go.mod unchanged.
Known limitations: no vault decrypt; no missing_password_count/can_rotate;
 no rotate/create/reveal; live PG 17/18 unproven; no viewport/zoom sign-off
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer feat/pg-012-security-api `9507111`:
  gofmt -l (touched Go) → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.556s; httpapi 19.860s
  go test -count=1 ./... → ok
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
 Parent review of API then FF onto master `9507111`:
  gofmt -l (touched Go) → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.592s; httpapi 19.328s
 Writer feat/pg-012-security-ui `18c467f` (Node v25.3.0, not web/.nvmrc 24.19.0):
  npm --prefix web run test:run → Tests 196 passed (196)
  npm --prefix web run build → tsc 7.0.2 + vite 8.2.2 (dist gitignored)
 Parent after UI merge `7bfc3a3` (parent-executed, not writer copy):
  npm --prefix web run test:run → Tests 196 passed (196), 30.70s
 Parent after evidence review on `6479e61`:
  npm --prefix web run test:run → Tests 196 passed (196), 29.03s
  git diff --name-only 0edf881..HEAD -- go.mod go.sum
   migrations/001_initial.sql → empty
 Not run: race, live PostgreSQL 17/18, CI, COMPATIBILITY.md §6, viewport/zoom
Local commits: `0edf881` (API freeze), `9507111` (API FF), `5d2f258` (API docs),
 `18c467f` (UI), `7bfc3a3` (merge UI), `b594502` (UI docs), `38cfc63` (API
 security pin), `f449ee1` (UI security pin), `6479e61` (UI review pin).
 Not pushed.
Reviewer/date: Security review (2026-08-25) on `9507111` approve Partial;
 no Critical/High/Medium/Low this-slice; session + postgres.read; no CSRF
 or audit; no-store; vault stub without project_credentials; static
 pg_stat_activity grouping with no query text; canary → 503; list/details
 still 404 protected names. Questions (not defects): postgres.read is
 owner-static; application_name is client-controlled (source 1c3e8e2
 same); GET audit-row pin absent; live pgx error text unproven.
 Security review (2026-08-25) on `18c467f`/`7bfc3a3` approve Partial UI;
 no Critical/High/Medium/Low this-slice; GET-only same-origin no CSRF;
 login never fetches; 401 extra keys ignored; 503 ≠ empty healthy;
 identifiers displayText + bidi isolate; saved credential “Not available”
 only; no rotate/create/reveal or web storage. Questions (not defects):
 no 200 extra-field canary; non-401 uses errorMessage; 401 does not force
 shell logout.
 UI review (2026-08-25) on `b594502`/`18c467f`/`7bfc3a3` approve Partial UI;
 no Critical/High/Medium. Optional Low (non-blocking, not required):
 table owner-flag headers shorter than stack/inspector; “this slice”
 jargon in page copy; ledger-badge left margin in Protected column.
 Explicitly NOT viewport/zoom sign-off: 360/768/1280/1600 and 200% zoom
 were not opened.
 Evidence review (2026-08-25) on `b594502` keep-Partial / reject-Complete;
 required docs fix: parent npm 196 after `7bfc3a3` is parent-executed
 (30.70s) and re-run on `6479e61` (29.03s); local-commits now include
 `0edf881` and `b594502`. Mocked UI GET is not live PostgreSQL.
 Optional SECURITY.md/DATA_AND_SECRETS.md GET name omitted (not a
 Partial blocker).
 Verifier (2026-08-25) PASS Partial on master `2a82329`. Independent
 re-runs go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.544s; httpapi 17.340s
  go test -count=1 ./... → ok (before npm build; re-run after dist
   settled → ok)
  go test -race -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 1.447s; httpapi 37.003s
  go vet ./... → no findings (before and after dist)
  go build -o NUL ./cmd/redgres → success (before and after dist)
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  npm --prefix web run test:run → Tests 196 passed (196), 29.27s
  npm --prefix web run build → tsc 7.0.2 + vite 8.2.2 (dist gitignored)
  git diff 0edf881..2a82329 -- go.mod go.sum migrations/001_initial.sql
   → empty
 Not executed: live PostgreSQL 17/18, §6, Playwright, viewport/zoom,
 gitleaks, govulncheck, CI, Node 24.19.0, vault decrypt, rotation,
 REDIS-008, AUTH-006.
 Local commits: `0edf881` (API freeze), `9507111` (API FF), `5d2f258`
 (API docs), `18c467f` (UI), `7bfc3a3` (merge UI), `b594502` (UI docs),
 `38cfc63` (API security pin), `f449ee1` (UI security pin), `6479e61`
 (UI review pin), `2a82329` (evidence pin), this verifier record.
 Not pushed.
 Keep PG-012 Partial. Keep REDIS-005 Partial. Not pushed.
```

## PgBouncer health on GET /status (2026-08-25)

```text
Requirement: PLAT-001 Partial (PgBouncer SHOW VERSION on GET /api/v1/status +
 Overview card; no live PgBouncer, no COMPATIBILITY.md §6, no viewport)
Decision/ADR: ADR-001, ADR-009 (admin path stays 5432)
Source characterization: official pgbouncer.org/usage.html console db pgbouncer
 SHOW VERSION; admin_users or stats_users; simple query protocol only.
 pgx v5.10.0 ConnConfig.DefaultQueryExecMode=QueryExecModeSimpleProtocol
 (not QueryExecModeExec); do not pgxpool.Ping (-- ping rejected); Exec
 SHOW VERSION no Scan. Sibling 1c3e8e2 runbook only (SHOW POOLS), not copied.
Implementation files: internal/config/{config.go,postgres.go};
 internal/postgresadmin/{types,memory,service,adapter}.go;
 internal/platform/status.go; internal/httpapi/status_routes.go;
 web/src/features/overview/OverviewPage.tsx (existing presentation
 mapping, not this-slice diff);
 web/src/App.test.tsx
Unit tests: config pooled-port valid/invalid/partial/production-optional;
 postgresadmin PingPooled nil/canary/catalog-still-5432/simple-protocol/
 ShouldPing false/SHOW VERSION SQL; platform five ids mixed ok/unavailable/
 nil not_configured/canary omitted; httpapi default not_configured, 401 no
 components, 405, healthz unchanged, version string absent;
 App.test.tsx default Not configured, Reachable/Unavailable independent rails,
 leftover not_implemented Not connected, login never /status
Integration tests: none run (no live PgBouncer claimed)
Security tests: canary host/password/version omitted from Collect and HTTP;
 401 omits components; no-store; no second password file; GET not audited
Deployment/migration impact: production serve still boots without pooled
 port. go.mod pgx v5.10.0 unchanged. 001_initial.sql unchanged.
Known limitations: live PgBouncer unproven; NewWithConfig construction
 failure still fails Open; no SHOW POOLS/CLIENTS; no /pgbouncer/status;
 no viewport/zoom
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer API feat/plat-001-pgbouncer-api `df8a9c2`:
  gofmt -l touched Go → empty
  go test -count=1 ./internal/config ./internal/postgresadmin
   ./internal/platform ./internal/httpapi
   → ok config 0.595s; postgresadmin 0.792s; platform 0.395s; httpapi 20.663s
  go test -count=1 ./... → ok; go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
 Parent review of API then merge `1966cf4`:
  gofmt -l touched Go → empty
  go test -count=1 ./internal/config ./internal/postgresadmin
   ./internal/platform ./internal/httpapi
   → ok config 0.515s; postgresadmin 0.780s; platform 0.392s; httpapi 18.783s
 Writer UI feat/plat-001-pgbouncer-ui `d37f299` (Node v25.3.0, not
 web/.nvmrc 24.19.0):
  npm --prefix web run test:run → Tests 201 passed (201)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 Parent after UI merge `4ab1f03`:
  go test -count=1 ./internal/config ./internal/postgresadmin
   ./internal/platform ./internal/httpapi
   → ok config 0.848s; postgresadmin 0.937s; platform 0.531s; httpapi 25.249s
  npm --prefix web run test:run → Tests 201 passed (201)
 Not run: race, live PgBouncer, COMPATIBILITY.md §6, CI, viewport/zoom
Local commits: `87b6914` (freeze), `b554b1b` (anti-patterns), `df8a9c2`
 (API), `1966cf4` (merge API), `d37f299` (UI), `4ab1f03` (merge UI),
 `01d91be` (docs record), `928cbee` (UI review pin), `6b5f45c` (security
 pin), `214dfd8` (evidence pin), this verifier record. Not pushed.
Reviewer/date: Security review (2026-08-25) on `01d91be`/`df8a9c2`/`1966cf4`
 approve Partial; no Critical/High/Medium. GET /status session +
 platform.read, no CSRF/audit, no-store; 401 omits components; healthz
 does not ping PgBouncer; canary host/password/version omitted; observer
 dbname pgbouncer, QueryExecModeSimpleProtocol, Exec SHOW VERSION no Scan
 and no pgxpool.Ping; catalog stays 5432; no second password file;
 production serve does not require pooled port; Open does not startup-Ping
 the console. Optional Low (non-blocking): no GET /status audit-count
 pin; optional pooled NewWithConfig failure still fails Open.
 UI review (2026-08-25) on `01d91be`/`d37f299`/`4ab1f03` approve Partial UI;
 no Critical/High/Medium. Optional Low (non-blocking): envelope 401/malformed
 tests do not name PgBouncer labels. Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) on `6b5f45c` keep-Partial / reject-Complete;
 mocked UI GET is not live PgBouncer; parent npm 201 after `4ab1f03` is
 parent-executed; OverviewPage mapping pre-existed `d37f299`. Hygiene:
 `6b5f45c` added to local-commits.
 Verifier (2026-08-25) PASS Partial on master `214dfd8`. Independent
 re-runs go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/config ./internal/postgresadmin
   ./internal/platform ./internal/httpapi
   → ok config 0.907s; postgresadmin 0.993s; platform 0.577s; httpapi 19.506s
  go test -count=1 ./... → ok
  go test -race -count=1 ./internal/config ./internal/postgresadmin
   ./internal/platform ./internal/httpapi
   → ok config 1.716s; postgresadmin 2.143s; platform 1.317s; httpapi 28.823s
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  npm --prefix web run test:run → Tests 201 passed (201)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
  go.mod and migrations/001_initial.sql vs 87b6914 → unchanged
  Go re-run after dist settled → focused + ./... ok; vet/build success
 Not executed: live PgBouncer, §6, Playwright, viewport/zoom, gitleaks,
 govulncheck, CI, Node 24.19.0.
 Keep PLAT-001 Partial. Keep PG-012 Partial. Keep REDIS-005 Partial.
 Not pushed.
```

## Optional expert tool links (2026-08-25)

```text
Requirement: PLAT-001 Partial (optional REDGRES_PGADMIN_URL /
 REDGRES_REDISINSIGHT_URL; GET /api/v1/session hrefs; GET /api/v1/status
 presence; Overview pgAdmin/RedisInsight anchors). No fetch/ping/proxy/embed.
Decision/ADR: ADR-001; freeze `1a528cd` (API, CONFIGURATION, ARCHITECTURE,
 SECURITY, UX)
Source characterization: redis-ui session field is top-level redisinsight_url
 with no URL validation (ConnectionRail target=_blank rel=noreferrer). Redgres
 uses nested tool_links, validates http(s), rejects userinfo/fragment/
 javascript/data/file/relative, production https-only, rel=noopener noreferrer.
Implementation files: internal/config/{config.go,tool_links.go};
 internal/platform/status.go; internal/httpapi/{auth_routes.go,status_routes.go};
 web/src/api/auth.ts; web/src/App.tsx; web/src/components/shell/AppShell.tsx;
 web/src/features/overview/OverviewPage.tsx; web/src/styles/shell.css;
 web/src/App.test.tsx
Unit tests: config empty/whitespace/alone/both/path+query/production optional
 https/production http reject/userinfo/javascript/data/file/relative/fragment/
 error omits URL; platform Collect false→not_configured true→ok, independent
 of postgres/redis down, never unavailable/not_implemented; httpapi login has
 no tool_links; session {} default, omits empty keys, both hrefs; GET session
 not audited, no-store; status 401 omits components; default not_configured;
 one-or-both set → ok without URLs in JSON; healthz unchanged; canary host
 omitted from status; App.test missing/{} no anchors, one/both hrefs +
 Reachable, storage, 401, login never /status/healthz, post-login GET /session
 CSRF vs login token, Refresh does not GET /session
Integration tests: none run (no live pgAdmin/RedisInsight claimed)
Security tests: userinfo/javascript/data/file/fragment rejected without
 echoing URL; status/healthz omit hrefs; GET session not audited;
 Cache-Control no-store; no server fetch of tool URLs
Deployment/migration impact: production serve still boots without these keys.
 go.mod pgx v5.10.0 and go-redis v9.22.0 unchanged. migrations/001_initial.sql
 unchanged. cmd/redgres unchanged.
Known limitations: jsdom is not viewport/zoom; URLs are never probed; ok means
 at least one URL is set; Placeholders.tsx unused OverviewPage path without
 toolLinks (AppShell renders OverviewPage directly); no default hostnames
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0):
 Writer API feat/plat-001-tool-links-api `33087b6`:
  gofmt -l touched Go → empty
  go test -count=1 ./internal/config ./internal/platform ./internal/httpapi
   → ok config 0.522s; platform 0.394s; httpapi 19.039s
  go test -count=1 ./... → ok
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent rerun API worktree before merge:
  gofmt -l → empty
  go test -count=1 ./internal/config ./internal/platform ./internal/httpapi
   → ok config 0.512s; platform 0.419s; httpapi 21.768s
 Writer UI feat/plat-001-tool-links-ui `fb0c6ce`:
  npm --prefix web run test:run → Tests 208 passed (208)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 Parent rerun UI worktree before merge:
  npm --prefix web run test:run → Tests 208 passed (208)
 Not run: race, Playwright, viewport/zoom, live expert tools, CI, gitleaks,
 govulncheck, Node 24.19.0
Local commits: `1a528cd` (freeze), `33087b6` (API), `fb0c6ce` (UI),
 `28faa83` (merge UI), `bf47017` (docs record), `46fdf6c` (UI review pin),
 `a0240d8` (security pin), `6ca659f` (evidence pin), this verifier record.
 Not pushed.
Reviewer/date: Security review (2026-08-25) on `bf47017`/`33087b6`/`28faa83`
 approve Partial; no Critical/High/Medium. GET /session session-gated hrefs;
 GET /status session + platform.read; no CSRF/audit; no-store; 401 omits
 components; Load/status never fetch operator URLs; javascript/data/file/
 userinfo/fragment/relative rejected without echoing URL; login has no
 tool_links; status JSON has no host/URL; UI post-login GET /session CSRF;
 Overview does not refetch /session; no iframe; no localStorage.
 Optional questions (non-blocking): login no-tool_links test without
 PgAdminURL set; 401 status without canary URLs; client parseToolLinks does
 not re-check http(s).
 UI review (2026-08-25) on `bf47017`/`fb0c6ce`/`28faa83` approve Partial UI;
 no Critical/High. Optional Low (non-blocking): shared `--focus` outline is
 button/input only, so the new `.text-button` anchors use the UA outline.
 Explicitly NOT viewport/zoom sign-off.
 Evidence review (2026-08-25) on `a0240d8` keep-Partial / reject-Complete.
 Parent-executed: focused Go config/platform/httpapi + npm test:run 208.
 Writer-only at evidence time: `go test ./...`, vet, build, `go list -m`,
 `npm run build`. Hygiene: `a0240d8` added to local-commits.
 Verifier (2026-08-25) PASS Partial on master `6ca659f`. Independent
 re-runs go1.27.0 windows/amd64, Node v25.3.0 (not web/.nvmrc 24.19.0):
  gofmt -l touched Go → empty
  go test -count=1 ./internal/config ./internal/platform ./internal/httpapi
   → ok config 0.617s; platform 0.438s; httpapi 18.012s
  go test -count=1 ./... → ok
  go test -race -count=1 ./internal/config ./internal/platform
   ./internal/httpapi
   → ok config 1.736s; platform 1.358s; httpapi 31.897s
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 208 passed (208)
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
  go.mod and migrations/001_initial.sql vs 1a528cd → unchanged
  Go re-run after dist settled → gofmt empty; vet clean;
   go test ./internal/web ok 0.396s; build success
 Not executed: live pgAdmin/RedisInsight, Playwright, viewport/zoom,
 gitleaks, govulncheck, CI, COMPATIBILITY.md §6, Node 24.19.0.
Keep PLAT-001 Partial. Keep PG-012 Partial. Keep REDIS-005 Partial.
 Do not mark Complete.
```

## PG-005 in-process Fernet/KDF gate (2026-08-25)

```text
Requirement: PG-005 Partial (in-process Fernet/KDF decrypt gate only; no HTTP
 reveal, vault SQL, or copied production ciphertext)
Decision/ADR: ADR-004; freeze `2bd4e40`
Source characterization: database-app modules/credential_vault.py _cipher at
 1c3e8e2; official Fernet spec; cryptography 49.0.0 fernet.py (ttl=None does
 not consider age). Compatibility research confirmed padded URLEncoding, no
 TTL, no fernet-go tag, PyPI 50.0.0 Fernet recipe unchanged.
Implementation files: internal/secrets/{fernet.go,kdf.go,fernet_test.go};
 internal/secrets/testdata/{python49.json,README.md};
 docs/ARCHITECTURE.md; docs/DATA_AND_SECRETS.md; docs/REPOSITORY_STRUCTURE.md
Unit tests: ASCII/Unicode decrypt; KDF exact vs Python 49; wrong-key; flipped
 ciphertext bit; truncated; invalid Base64; wrong version; 2010 timestamp
 succeeds; canary SESSION_SECRET/key/token/plaintext absent from err.Error()
Integration tests: none — no vault SQL/HTTP; go test does not invoke Python
Security tests: ErrInvalidToken + canary SESSION_SECRET/key/token/plaintext
 absent from err.Error() (tested). HMAC uses subtle.ConstantTimeCompare and
 PKCS#7 fail-closed only after HMAC (implementation, not isolated tests).
Deployment/migration impact: none. No env, config, PostgreSQL, or HTTP.
 go.mod unchanged (pgx v5.10.0, go-redis v9.22.0).
Known limitations: Gate 2 Go-encrypt→Python not this slice; Gate 4 copied
 production ciphertext outstanding; no reveal route; no
 REDGRES_LEGACY_VAULT_SECRET_FILE; token decode accepts missing Base64 padding
 (Python-lenient). Optional unadded tests: RawURLEncoding negative; independent
 HMAC tampers of timestamp/IV/MAC.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 Writer feat/pg-005-fernet-kdf `1268dbf`:
  gofmt -l internal/secrets/*.go → empty
  go test -count=1 ./internal/secrets → ok 0.365s
  go test -count=1 ./... → ok
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go test -race -count=1 ./internal/secrets → ok 1.548s
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent rerun worktree before merge:
  gofmt -l → empty
  go test -count=1 ./internal/secrets → ok 0.398s
 Parent rerun master `cec6587` after TRACEABILITY pin:
  gofmt -l internal/secrets → empty
  go test -count=1 ./internal/secrets → ok 0.390s
  go test -race -count=1 ./internal/secrets → ok 1.603s
  go test -count=1 ./... → ok (secrets 0.576s; httpapi 21.030s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
Local commits: `2bd4e40` (freeze), `1268dbf` (impl), `cec6587` (docs
 record), `cf29328` (security pin), `e859af6` (evidence pin), this
 verifier record. Not pushed.
Keep PG-005 Partial. Keep PLAT-001 Partial. Keep PG-012 Partial.
 Keep REDIS-005 Partial. Do not mark Complete.
```

## PG-005 Fernet/KDF security pin (2026-08-25)

```text
Requirement: PG-005 Partial (in-process Fernet/KDF decrypt gate only)
Decision/ADR: ADR-004; freeze `2bd4e40`
Reviewer/date: Security review (2026-08-25) on `cec6587` approve Partial;
 no Critical/High/Medium/Low. HMAC over version||timestamp||IV||ciphertext
 with subtle.ConstantTimeCompare; PKCS#7 only after HMAC; single
 ErrInvalidToken; canary SESSION_SECRET/key/token/plaintext absent from
 err.Error(); no env/config/HTTP/PostgreSQL; no fernet-go; testdata is fake
 canary not production ciphertext; no TTL is intentional. Questions
 (non-blocking): failure-class timing; optional timestamp/IV/MAC tampers;
 heap remnants of plaintext/key. Reviewer did not re-run go test.
Keep PG-005 Partial. Gate 4 copied production ciphertext, GET masked
 metadata, POST reveal, vault SQL, and REDGRES_LEGACY_VAULT_SECRET_FILE
 remain Complete blockers, not defects of this gate.
Not pushed.
```

## PG-005 Fernet/KDF evidence pin (2026-08-25)

```text
Requirement: PG-005 Partial (in-process Fernet/KDF decrypt gate only)
Decision/ADR: ADR-004; freeze `2bd4e40`
Reviewer/date: Evidence review (2026-08-25) on `cec6587` keep-Partial /
 reject-Complete. Fixture decrypt in internal/secrets is present; go.mod has
 no fernet-go; testdata is fake canary; TRACEABILITY does not mark Complete.
 Full PRD GET masked metadata / POST reveal / vault SQL / Gate 4 remain
 outstanding. Reviewer did not run tests (sandbox blocked git). Parent
 broader Go set at `cec6587` is recorded in `cf29328` (not visible to the
 evidence reviewer, who froze `cec6587`).
 Required corrections applied in this commit: TRACEABILITY Security tests
 wording; matrix Planned implementation includes internal/secrets; R-001
 retitled; SECURITY.md current-state note; local-commits include cec6587
 and cf29328.
Keep PG-005 Partial. Verifier PASS Partial recorded below. Not pushed.
```

## PG-005 Fernet/KDF verifier PASS Partial (2026-08-25)

```text
Requirement: PG-005 Partial (in-process Fernet/KDF decrypt gate only)
Decision/ADR: ADR-004; freeze `2bd4e40`
Reviewer/date: Verifier (2026-08-25) PASS Partial on master `e859af6`.
 Independent re-runs go1.27.0 windows/amd64:
  gofmt -l internal/secrets → empty
  go test -count=1 ./internal/secrets → ok 0.574s
  go test -race -count=1 ./internal/secrets → ok 1.584s
  go test -count=1 ./... → ok (httpapi 19.330s; secrets 0.488s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Security review (2026-08-25) on `cec6587` approve Partial; no
 Critical/High/Medium/Low.
 Evidence review (2026-08-25) on `cec6587` keep-Partial / reject-Complete;
 corrections landed in `e859af6`.
KDF independently recomputed to fixture key
 u5Qxug06sp24rfxkW82bVJCnEYp9qu-Fb9PoFDVnAOQ=. Old-timestamp token
 version 0x80 and timestamp 1262304000 verified from token bytes.
 Testdata is fake canary. go.mod unchanged. No postgresadmin/httpapi/
 config/web/cmd vault SQL or reveal. Sibling database-app clean at 1c3e8e2.
Not executed: live Python 49.0.0; Gate 2 encrypt; Gate 4 copied production
 ciphertext; full go test -race ./...; npm; COMPATIBILITY.md §6; HTTP
 reveal canaries.
Non-blocking nits: SOURCE_BASELINE.md Redgres row still says vault not
 started (stale snapshot; not refreshed here); ROADMAP M4 Fernet gates
 remain upcoming for create/reveal/rotate.
Keep PG-005 Partial. Do not mark Complete.
Local commits: `2bd4e40`, `1268dbf`, `cec6587`, `cf29328`, `e859af6`,
 this verifier record. Not pushed.
```

## PG-005/PG-012 vault existence GET (2026-08-25)

```text
Requirement: PG-005/PG-012/PG-002 Partial (vault existence GET; no decrypt/reveal)
Decision/ADR: ADR-004; API freeze `e915912`
Source characterization: database-app has_role_password +
 _saved_role_names/get_security_overview at 1c3e8e2; Redgres does not
 copy ensure_vault() or except Exception: return set(); lists
 database_console_vault; no has_saved_password
Implementation files: internal/postgresadmin/{errors,types,memory,adapter,service}.go
 + matching tests; internal/httpapi/postgres_routes_test.go,
 postgres_security_routes_test.go; docs/ARCHITECTURE.md implemented-now;
 web/src/api/postgres.ts; web/src/features/postgres/{DatabasesPage,SecurityOverview}.tsx;
 web/src/App.test.tsx; docs/UX.md
Unit tests: SavedRoleNames empty/no-connect; SQL forbids encrypted_password/
 updated_at; catalog/list SQL still has no vault; Details present/missing/
 not_available; Security ok + missing_password_count (incl. 0 and pre-cap
 501); vault_unavailable omits count and keeps databases; canaries absent;
 3D000/42P01 → ErrVaultUnavailable
HTTP tests: details present/missing/vault_unavailable 200; security ok +
 count; vault_unavailable 200 omits count; 401 omits keys; no
 has_saved_password/can_rotate
UI tests: Details Saved/Not saved/Not available; Security Missing vault
 entries (1 and 0); unavailable omits count; no reason-string leak
Integration tests: none — live PostgreSQL not run
Security tests: ciphertext columns absent from SQL; ErrVaultUnavailable not
 503; canaries absent from HTTP
Deployment/migration impact: none. go.mod unchanged (pgx v5.10.0,
 go-redis v9.22.0). No new routes or config keys.
Known limitations: connect-time 3D000 classified by failure class after
 connectTarget collapses to ErrUnavailable; unique owners SQL-capped at 500;
 Gate 4, POST reveal, REDGRES_LEGACY_VAULT_SECRET_FILE, decrypt of vault
 rows outstanding
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0):
 Writer API feat/pg-012-vault-existence-api `fea4764`:
  gofmt -l internal/postgresadmin internal/httpapi → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.933s; httpapi 21.229s
  go vet ./internal/postgresadmin ./internal/httpapi → no findings
 Parent rerun API worktree before merge:
  gofmt -l → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.938s; httpapi 20.761s
  go vet → no findings
 Writer UI feat/pg-012-vault-existence-ui `2a29cf6`:
  npm --prefix web run test:run → Tests 213 passed (213)
 Parent rerun UI worktree before merge:
  npm --prefix web run test:run → Tests 213 passed (213)
 Not run: race, live PostgreSQL, COMPATIBILITY.md §6, Playwright, viewport/zoom
Local commits: `e915912` (freeze), `fea4764` (API), `2a29cf6` (UI),
 `6f55c1d` (merge UI), `7bc2db6` (docs record), `05260ce` (security pin).
 UI/evidence pin this commit. Verifier pending. Not pushed.
Keep PG-005 Partial. Keep PG-012 Partial. Do not mark Complete.
```

## PG-005/PG-012 vault existence security pin (2026-08-25)

```text
Requirement: PG-005/PG-012/PG-002 Partial (vault existence GET; no decrypt/reveal)
Decision/ADR: ADR-004; freeze `e915912`
Reviewer/date: Security review (2026-08-25) on `7bc2db6` approve Partial;
 no Critical/High/Medium/Low. SQL is role_name ANY($1) only; connectTarget
 password not logged; ErrVaultUnavailable is HTTP 200 not 503; catalog
 failure remains 503; postgres.read; GET not audited; no-store; 401 omits
 keys; no secret file; UI never paints reason/ciphertext. Questions
 (non-blocking): writePostgresError has no ErrVaultUnavailable case
 (Details/SecurityOverview never return it); unique-owner SQL cap at 500;
 GET audit-count tests not added. Reviewer did not re-run go test.
Keep PG-005 Partial. Keep PG-012 Partial. Gate 4, POST reveal, and
 REDGRES_LEGACY_VAULT_SECRET_FILE remain Complete blockers.
Not pushed.
```

## PG-005/PG-012 vault existence UI and evidence pin (2026-08-25)

```text
Requirement: PG-005/PG-012/PG-002 Partial (vault existence GET; no decrypt/reveal)
Decision/ADR: ADR-004; freeze `e915912`
Reviewer/date: UI review (2026-08-25) on `7bc2db6` approve Partial UI;
 no Critical/High/Medium. Frozen copy holds in source and jsdom. Viewports
 360×800 / 768×1024 / 1280×800 / 1600×1000 and 200% zoom not rendered (no
 browser/Playwright). Explicitly NOT viewport/zoom sign-off. Optional
 polish (non-blocking, frozen residuals): header “Rotation is not available”
 vs vault Not available; owner-flag label length; Protected badge margin;
 no postgres page-header rail.
 Evidence review (2026-08-25) on `7bc2db6` keep-Partial / reject-Complete.
 Existence SQL + UI copy only. Full PG-005 still needs masked metadata
 URLs, POST reveal, Gate 4. Parent-executed: focused Go postgresadmin/
 httpapi + npm test:run 213. Writer/parent focused timings are attested;
 go test ./..., race, build, go list at 7bc2db6 remain verifier-required.
 SOURCE_BASELINE vault-not-started is a known nit; not refreshed.
Keep PG-005 Partial. Keep PG-012 Partial. Verifier pending. Not pushed.
```

## PG-005/PG-012 vault existence verifier PASS Partial (2026-08-25)

```text
Requirement: PG-005/PG-012/PG-002 Partial (vault existence GET; no decrypt/reveal)
Decision/ADR: ADR-004; API freeze `e915912`
Reviewer/date: Verifier (2026-08-25) on `d45b1d7` PASS Partial. HEAD
 `d45b1d7dacc7af44b20d1a74507418dee6dfa6de`; working tree clean. Security
 approve Partial on `7bc2db6` (no C/H/M/L). UI approve Partial UI on
 `7bc2db6` (NOT viewport/zoom). Evidence keep-Partial / reject-Complete
 on `7bc2db6`. Sibling database-app `1c3e8e2` untouched.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node
 v25.3.0 (not web/.nvmrc 24.19.0):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin ./internal/httpapi ./internal/secrets
 → ok postgresadmin 1.249s; httpapi 21.971s; secrets 0.379s
 go test -count=1 ./... first run FAIL embed (parallel npm build swapped
 ignored dist hashes); re-run after dist stable → all ok (httpapi 21.174s;
 cmd/redgres 2.953s; web 0.607s; migrations no tests)
 go test -race -count=1 ./internal/postgresadmin ./internal/secrets
 → ok postgresadmin 1.902s; secrets 1.369s
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → exit 0
 go list -m github.com/jackc/pgx/v5 → v5.10.0
 go list -m github.com/redis/go-redis/v9 → v9.22.0
 npm --prefix web run test:run → Tests 213 passed (213), 55.07s
 npm --prefix web run build → tsc + vite PASS (~1.07s); gitignored
 internal/web/dist/app hashes
 go.mod unchanged vs freeze. SavedRoleNames SQL is role_name ANY($1) only.
 httpapi/postgresadmin do not import internal/secrets.
Not run: live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright,
 viewport/zoom, Gate 4, POST reveal, go test -race ./..., CI, gitleaks,
 govulncheck.
Known nits: SOURCE_BASELINE still says vault not started; no dedicated
 details 401 saved_credential-omission test (security 401 + writeError
 inspection); unique-owner SQL cap 500.
Keep PG-005 Partial. Keep PG-012 Partial. Do not mark Complete.
Local commit to verify: `d45b1d7`. This verifier record. Not pushed.
```

## PG-004/PG-005 masked connection GET Partial (2026-08-25)

```text
Requirement: PG-004/PG-005 Partial (GET masked connection metadata; no reveal/decrypt)
Decision/ADR: ADR-001, ADR-003, ADR-004; freeze `99986a1`
Source characterization: database-app get_database_connection_url +
 connection_urls.py at 1c3e8e2; Redgres omits sibling null aliases and
 has_saved_password; no silent db.example.com; sslmode=require;
 POOLED_PORT reused for pooled URL port; Redis omit-key analog
Implementation files: internal/config/{config,postgres,postgres_test}.go;
 internal/postgresadmin/{connection,connection_test,types,service,service_test}.go;
 internal/httpapi/{server,postgres_routes,postgres_routes_test}.go;
 web/src/api/postgres.ts; web/src/features/postgres/DatabasesPage.tsx;
 web/src/App.test.tsx
Unit tests: builder encoding/omit/sslmode; Connection present/missing/
 not_available; domain has no URL fields; config public host/direct port
HTTP tests: 401/404/400/503; omit rules; no-store; GET without CSRF;
 no canaries; POST /connection 405; POST /connection/reveal unregistered 404
UI tests: inspector Direct/Pooled URLs + copy; clear on selection/logout;
 401/503; isDetailsUrl excludes /connection; no reason leak; no Reveal
Integration tests: none — live PostgreSQL 17/18 not run
Security tests: postgres.read; no decrypt; no internal/secrets on path;
 ciphertext SQL unchanged; 401 omits keys
Deployment/migration impact: none. go.mod unchanged (pgx v5.10.0,
 go-redis v9.22.0). New optional REDGRES_POSTGRES_PUBLIC_HOST /
 REDGRES_POSTGRES_DIRECT_PORT. No REDGRES_LEGACY_VAULT_SECRET_FILE.
Known limitations: POST reveal, Gate 4 copied production ciphertext,
 live PostgreSQL 17/18, Playwright viewports outstanding;
 POST /connection/reveal is 404 (unregistered) not 405
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node
 v25.3.0 (not web/.nvmrc 24.19.0):
 Writer API feat/pg-004-masked-connection-api `20addbf`:
  gofmt -l → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.922s; httpapi 21.991s
  go test -count=1 ./... → ok (httpapi 23.541s)
  go vet → no findings
  go build -o NUL ./cmd/redgres → success
 Writer UI feat/pg-004-masked-connection-ui `80c958d`:
  npm --prefix web run test:run → Tests 222 passed (222)
 Parent command set executed after merge `60825fd` (docs-only record
 `22f12f4` does not change behavior):
  gofmt -l internal/postgresadmin internal/httpapi/postgres_routes.go
   internal/httpapi/server.go internal/config → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi ./internal/config
   → ok postgresadmin 1.084s; httpapi 21.214s; config 0.501s
  go test -count=1 ./... → all ok (httpapi 22.553s; cmd/redgres 2.322s;
   postgresadmin 1.235s; web 0.587s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 222 passed (222), 31.82s
Local commits: `99986a1` (freeze), `20addbf` (API), `80c958d` (UI),
 `7ed29df` (merge API), `60825fd` (merge UI), `22f12f4` (docs record).
 Not pushed.
Keep PG-004 Partial. Keep PG-005 Partial. Do not mark Complete.
```

## PG-004/PG-005 masked connection security pin (2026-08-25)

```text
Requirement: PG-004/PG-005 Partial (GET masked connection metadata; no reveal/decrypt)
Decision/ADR: ADR-004; freeze `99986a1`
Reviewer/date: Security review (2026-08-25) on `22f12f4` approve Partial;
 no Critical/High/Medium. Session + postgres.read; no CSRF; no audit;
 no-store; omit URLs unless present + public host + matching port; never
 copies admin host/port; SavedRoleNames still role_name only; 401 omits
 keys; POST /connection 405; POST /connection/reveal unregistered 404;
 UI memory-only, no auto-copy, clear on selection/logout. Residual Lows
 (non-blocking): loadConnection does not match body.database to selected
 (AbortController); public host validation same as Redis. Reviewer did
 not re-run go test.
Keep PG-004 Partial. Keep PG-005 Partial. POST reveal and Gate 4 remain
 Complete blockers.
Not pushed.
```

## PG-004/PG-005 masked connection UI and evidence pin (2026-08-25)

```text
Requirement: PG-004/PG-005 Partial (GET masked connection metadata; no reveal)
Decision/ADR: ADR-004; freeze `99986a1`
Reviewer/date: UI review (2026-08-25) on `22f12f4` approve Partial UI;
 no Critical/High/Medium. Frozen copy holds in source and jsdom. Copy is
 click-only text-button; no toast; no Reveal/Rotate/Create; clear on
 selection/logout; isDetailsUrl excludes /connection; Saved credential
 stays on details GET. Explicitly NOT viewport/zoom sign-off. Missing
 evidence (non-blocking): no delayed “Loading connection.” test;
 Direct-only/Pooled-only rows untested; connection abort race untested.
 Evidence review (2026-08-25) on `22f12f4` keep-Partial / reject-Complete.
 Freeze GET connection criteria map to implementation and claimed tests.
 Required corrections applied in this commit: TRACEABILITY names `22f12f4`
 as docs-only record; parent commands after `60825fd`; Known limitations
 include live PostgreSQL 17/18. Reviewers did not re-run tests. Parent
 broader set at `60825fd` is recorded in `22f12f4`.
Keep PG-004 Partial. Keep PG-005 Partial. Verifier pending. Not pushed.
```

## PG-004/PG-005 masked connection GET verifier PASS Partial (2026-08-25)

```text
Requirement: PG-004/PG-005 Partial (GET masked connection metadata; no reveal/decrypt)
Decision/ADR: ADR-004; freeze `99986a1`
Verifier/date: independent redgres-verifier (2026-08-25) on `f8bc9a3`
HEAD: `f8bc9a3df37648a13721b71321b5bf11c887c053` (clean worktree)
Verdict: PASS Partial. Keep PG-004 Partial. Keep PG-005 Partial. Reject Complete.
Security/UI/evidence reviews accepted as already pinned; not re-run.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin ./internal/httpapi ./internal/config
 → ok postgresadmin 1.012s; httpapi 25.356s; config 0.545s
 go test -count=1 ./... → all ok (httpapi 25.942s; cmd/redgres 5.158s;
 postgresadmin 2.162s; web 0.630s; migrations no tests)
 go test -race -count=1 ./internal/postgresadmin ./internal/config
 → ok postgresadmin 3.275s; config 2.922s
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 go list -m github.com/jackc/pgx/v5 → v5.10.0
 go list -m github.com/redis/go-redis/v9 → v9.22.0
 npm --prefix web run test:run → Tests 222 passed (222), 36.80s
 npm --prefix web run build → success (ignored internal/web/dist/app/)
Unexecuted: live PostgreSQL 17/18, Playwright viewports, Gate 4, POST reveal,
 REDGRES_LEGACY_VAULT_SECRET_FILE, CI, Node 24.19.0, go test -race ./...
Slice diff 99986a1..f8bc9a3: 15 files, no .env/.db/certs/secret files.
Known limitations unchanged: POST reveal, Gate 4 copied production ciphertext,
 live PostgreSQL 17/18, Playwright viewports outstanding;
 POST /connection/reveal is 404 (unregistered) not 405.
Not pushed.
Keep PG-004 Partial. Keep PG-005 Partial. Do not mark Complete.
```

## PG-012 rotation eligibility Partial (2026-08-25)

```text
Requirement: PG-012 Partial (rotation_eligible on GET /api/v1/postgres/security
 databases[] rows + Security overview last column; no POST rotate/reveal/create)
Decision/ADR: ADR-004 (vault decrypt still unused); freeze `674bd5c`
Source characterization: database-app security_ops.get_security_overview at
 1c3e8e2 uses can_rotate = rolcanlogin && !rolsuper && owner ∉ {postgres,
 adminpg, database_console, onelife_pg_admin}. Redgres does not emit
 can_rotate and does not hardcode adminpg. Derives in Service from existing
 row fields + Policy.Manageable (protected = !Manageable). No new catalog SQL.
Implementation files: internal/postgresadmin/{types.go,service.go,rotation.go,
 service_test.go}; internal/httpapi/postgres_security_routes_test.go;
 web/src/api/postgres.ts; web/src/features/postgres/SecurityOverview.tsx;
 web/src/App.test.tsx
Unit tests: project_a true; postgres/zeta_last/empty owner/owned_by_admin/
 no_connect false; vault unavailable still emits booleans; JSON never omits
 rotation_eligible; can_rotate absent
HTTP tests: 200 boolean on every row; project_a true / protected false;
 can_rotate/has_saved_password still nil; 401/503 unchanged
UI tests: last-column Rotation eligible Yes/No; missing field → —; header
 still “Rotation is not available.”; no Rotate/Reveal/Create
Integration tests: none — live PostgreSQL 17/18 not run
Security tests: can_rotate still absent; vault_unavailable does not change
 eligibility; no-store unchanged; eligibility is diagnostic only
Deployment/migration impact: none. go.mod unchanged (pgx v5.10.0,
 go-redis v9.22.0). No REDGRES_LEGACY_VAULT_SECRET_FILE. Route table
 unchanged.
Known limitations: POST rotate/reveal not registered; no summary eligible
 count; no details-GET field; Gate 4 copied production ciphertext,
 live PostgreSQL 17/18, Playwright viewports outstanding; go test -race ./...,
 CI, Node 24.19.0 unexecuted. No .env/.db/certs/secret files in 674bd5c..HEAD.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node
 v25.3.0 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/pg-012-rotation-eligibility-api `686bbd9`:
  gofmt -l → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.986s; httpapi 22.396s
  go test -count=1 ./... → ok (httpapi 25.578s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
 Writer UI feat/pg-012-rotation-eligibility-ui `07130ea`:
  npm --prefix web run test:run → Tests 224 passed (224), 33.43s
  npm --prefix web run build → success (ignored internal/web/dist/app/)
 Parent command set executed after merge `387b8d8` (docs-only record
 `bd7b067` does not change behavior):
  gofmt -l internal/postgresadmin
   internal/httpapi/postgres_security_routes_test.go → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 1.365s; httpapi 24.792s
  go test -count=1 ./... → all ok (httpapi 29.912s; cmd/redgres 3.689s;
   postgresadmin 1.808s; web 0.972s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/jackc/pgx/v5 → v5.10.0
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 224 passed (224), 37.64s
Local commits: `674bd5c` (freeze), `686bbd9` (API), `07130ea` (UI),
 `744f8df` (merge API), `387b8d8` (merge UI), `bd7b067` (docs record),
 `41ef55a` (name TRACEABILITY HEAD). UI pin `53c0d1b` and security pin
 `c27700e` follow this block.
 Not pushed.
Keep PG-012 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Do not mark Complete.
```

## PG-012 rotation eligibility UI pin (2026-08-25)

```text
Requirement: PG-012 Partial (Security overview paints rotation_eligible; no rotate)
Decision/ADR: ADR-004; freeze `674bd5c`
Reviewer/date: UI review (2026-08-25) on `41ef55a` approve Partial UI;
 no Critical/High/Medium. Frozen copy holds: last ledger/stack column after
 Connections; Yes/No/— via yesNo/optionalBoolean; header still “Rotation
 is not available.”; no Rotate/Reveal/Create; loading/401/503 unchanged;
 no toasts; persistence test still no setItem. Explicitly NOT viewport/zoom
 sign-off. Missing evidence (non-blocking): extra column overflow/stack
 swap unproven in jsdom; reviewer did not re-run npm tests. Optional polish
 (non-blocking): em dash has no extra SR phrase.
Keep PG-012 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 POST rotate, POST reveal, Gate 4, live PostgreSQL 17/18, Playwright
 viewports remain Complete blockers.
Not pushed.
```

## PG-012 rotation eligibility security pin (2026-08-25)

```text
Requirement: PG-012 Partial (rotation_eligible on GET /api/v1/postgres/security;
 no POST rotate/reveal/create)
Decision/ADR: ADR-004; freeze `674bd5c`
Reviewer/date: Security review (2026-08-25) on `41ef55a` approve Partial;
 no Critical/High/Medium. Formula holds; postgres.read; no CSRF; no audit;
 no-store; no new route; POST rotate unregistered; can_rotate absent;
 adminpg not hardcoded; no new catalog SQL; vault does not change
 eligibility; diagnostic only; details GET has no field. Residual Lows
 (non-blocking): no HTTP 404 assertion for POST rotate; documented adminpg
 Policy delta; UI “Yes” under “Rotation is not available.” Reviewer did
 not re-run tests.
Keep PG-012 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 POST rotate, POST reveal, Gate 4 remain Complete blockers.
Not pushed.
```

## PG-012 rotation eligibility evidence pin (2026-08-25)

```text
Requirement: PG-012 Partial (rotation_eligible on GET /api/v1/postgres/security
 + Security overview last column; no POST rotate)
Decision/ADR: ADR-004; freeze `674bd5c`
Reviewer/date: Evidence review (2026-08-25) on `41ef55a` keep-Partial /
 reject-Complete. Freeze criteria map to implementation and claimed tests.
 Required corrections applied in this commit: implementation files include
 service_test.go; Local commits include `41ef55a`; Known limitations name
 live PostgreSQL 17/18, Gate 4, go test -race ./..., CI, Node 24.19.0,
 no secret artifacts. Independent UI pin `53c0d1b` and security pin
 `c27700e` already landed (reviewers did not re-run tests). Parent
 command set remains after `387b8d8`. Do not mark Complete.
Keep PG-012 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Verifier pending. Not pushed.
```

## PG-012 rotation eligibility verifier PASS Partial (2026-08-25)

```text
Requirement: PG-012 Partial (rotation_eligible on GET /api/v1/postgres/security
 databases[] + Security overview last column; no POST rotate/reveal/create)
Decision/ADR: ADR-004 (vault decrypt still unused); freeze `674bd5c`
Verifier/date: independent verifier (2026-08-25) on HEAD
 `8ed12b1bc5da42f462e527ff62367f5b352152b2`. Tracked tree clean.
 PASS Partial. Do not mark Complete.
Freeze: formula holds; always JSON boolean (no omitempty); no can_rotate;
 no new catalog SQL; POST rotate unregistered (chi 404; no dedicated HTTP
 404 test — residual Low); UI last column Yes/No/—; header still
 “Rotation is not available.”; 401 omits databases; 405 other methods;
 vault unavailable still emits booleans; 401/503 UI unchanged.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin ./internal/httpapi
 → ok postgresadmin 1.201s; httpapi 31.349s
 go test -count=1 ./... → all ok (httpapi 31.056s; cmd/redgres 5.049s;
 postgresadmin 1.728s; web 0.731s; migrations no tests)
 go test -race -count=1 ./internal/postgresadmin → ok 2.327s
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success (retry after npm build;
 first attempt raced hashed embed rewrite)
 go list -m github.com/jackc/pgx/v5 → v5.10.0
 go list -m github.com/redis/go-redis/v9 → v9.22.0
 npm --prefix web run test:run → Tests 224 passed (224), 44.84s
 npm --prefix web run build → success (ignored internal/web/dist/app/)
Unexecuted: live PostgreSQL 17/18, Playwright viewports, Gate 4 copied
 production ciphertext, POST rotate/reveal, CI, Node 24.19.0,
 go test -race ./..., REDGRES_LEGACY_VAULT_SECRET_FILE.
No secret artifacts in 674bd5c..HEAD. Prior UI `53c0d1b`, security
 `c27700e`, evidence `8ed12b1` pins stand.
Keep PG-012 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Not pushed.
```

## REDIS-008 ACL delete + AUTH-006 in-handler reauth Partial (2026-08-25)

```text
Requirement: REDIS-008 Partial + AUTH-006 Partial (DELETE /api/v1/redis/users/{username}
 in-handler reauth only; inspector Delete + danger dialog). Keep REDIS-008 Partial.
 Keep AUTH-006 Partial (this DELETE only). Keep PG-012/PG-004/PG-005 Partial.
 Do not mark Complete. No REDGRES_FEATURE_REDIS_USER_DELETE, no
 POST /api/v1/auth/reauth, no live Redis, no PostgreSQL drop/truncate/row-delete,
 no go-redis bump, no CLIENT KILL, no key deletion. go-redis stays v9.22.0.
Decision/ADR: ADR-006 unused for delete (ACL DELUSER, not command grants). AUTH-006
 is in-handler LookupOwnerByUsername + Verify on body owner_password; no reauth
 endpoint / short-lived grant. Capability redis.destructive + CSRF. PLAT-002:
 redis.user.delete audit metadata username only.
Source: redis-ui handleDeleteUser ~460–492 (read-only). Did not copy {ok: true} or
 audit reason: reauth. Official Redis ACL DELUSER deletes the user and terminates
 that user’s connections; cannot remove default; keys are not deleted.
 go-redis v9.22.0 acl_commands.go: ACLDelUser(ctx, username) *IntCmd →
 NewIntCmd(ctx, "acl", "deluser", username); adapter wraps .Result() as
 (int64, error).
Implementation files: internal/auth/{reauth.go,reauth_test.go};
 internal/redisadmin/{service.go,adapter.go,memory.go,delete_test.go};
 internal/httpapi/{server.go,redis_users_routes.go,redis_users_routes_test.go};
 web/src/features/redis/{DeleteAclUserDialog.tsx,AclUsersPage.tsx};
 web/src/api/redis.ts (deleteRedisUser); web/src/styles/globals.css
 (.danger-button uses var(--danger), not --redis); web/src/App.test.tsx.
 docs/API.md frozen on 372fbfa.
Unit/HTTP: Reauthenticate mismatch is ErrReauthRequired (not ErrMismatchedHash);
 missing owner VerifyUnknown then ErrUnauthorized; lookup failure distinct.
 Service: protected (default/admin/redact_admin/configured admin EqualFold)
 never Redis; missing no DELUSER; n==0 → ErrNotFound; DELUSER username only;
 canary Redis errors classified without leak. HTTP: 401 no session; 403 CSRF;
 400 confirmation fields.username_confirmation no audit no Redis; 403
 reauth_required audit username only (canary password absent; no reason:reauth;
 login_attempts unchanged; no 429); 403 protected no DELUSER; 404; 503 Redis
 canary; 200 {request_id} only (no ok/user/state/credential/reason); audit-fail
 after DELUSER is 503 fail-closed; collection DELETE stays 405; item DELETE
 no longer 405; enable/rotate/PATCH regressions still pass.
UI tests: ok non-protected shows Delete; protected/loading/unavailable/
 not_configured hide Delete; existing protected-user tests still assert no
 Delete; dialog does not DELETE until fields valid and Confirm; CSRF +
 encoded path + body keys only; 200 clears secrets/selection/refreshes list
 (memory only; no setItem); 401 session-expired, no leftover password;
 reauth_required stays on dialog, announces error, clears password, keeps
 confirmation; 403 protected / 404 / 503 same copy families as rotate;
 focus trap; danger class --danger not Redis identity; login never DELETE;
 search never DELETE.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/redis-008-delete-api `74f327f` (worktree
 D:\code\github\Redgres-worktrees\redis-008-delete-api):
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/auth ./internal/redisadmin ./internal/httpapi
   → ok auth 2.844s; redisadmin 1.939s; httpapi 31.428s
  go test -count=1 ./... → ok (httpapi 24.983s; redisadmin 2.184s; auth 4.154s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Writer UI feat/redis-008-delete-ui `7d96501` (worktree
 D:\code\github\Redgres-worktrees\redis-008-delete-ui):
  npm --prefix web run test:run → Tests 242 passed (242), 41.95s
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 Parent after API merge `5534e86`:
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/auth ./internal/redisadmin ./internal/httpapi
   → ok auth 3.616s; redisadmin 2.188s; httpapi 26.642s
  go list -m github.com/redis/go-redis/v9 → v9.22.0
 Parent after UI merge `967e156`:
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/auth ./internal/redisadmin ./internal/httpapi
   → ok auth 5.292s; redisadmin 2.455s; httpapi 35.938s
  go test -count=1 ./... → all ok (httpapi 29.952s; cmd/redgres 3.320s;
   redisadmin 2.554s; auth 5.679s; web 0.680s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  go list -m github.com/redis/go-redis/v9 → v9.22.0
  npm --prefix web run test:run → Tests 242 passed (242), 49.18s
  npm --prefix web run build → success (ignored internal/web/dist/app/)
Not run: live Redis 8.2/8.8, COMPATIBILITY.md §6, Playwright viewports,
 gitleaks, govulncheck, CI, race, Node 24.19.0.
Known limitations: AUTH-006 is this DELETE only. Missing-owner HTTP path is
 covered at auth.Reauthenticate; session JOIN owners means a deleted owner
 row 401s in requireSession before the handler. DELUSER-then-audit-fail
 leaves the Redis user gone (fail-closed, not returned as 200). GetUser/
 DELUSER race can 404 after LIST. MemoryClient is not live Redis. No
 dedicated reauth throttle. PostgreSQL drop/truncate/row-delete still
 unregistered. jsdom does not resolve CSS variables to computed RGB
 (danger vs Redis red asserted via class + globals.css source).
Local commits: `372fbfa` (freeze), `74f327f` (API), `5534e86` (merge API),
 `7d96501` (UI), `967e156` (merge UI), this docs record.
 Not pushed.
Reviewer/date: pending parent/security/verifier. Keep Partial.
Keep REDIS-008 Partial. Keep AUTH-006 Partial. Keep PG-012 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial. Do not mark Complete.
```

## REDIS-008 ACL delete UI pin (2026-08-25)

```text
Requirement: REDIS-008 Partial (inspector Delete + danger dialog; AUTH-006 this
 DELETE UI only)
Decision/ADR: AUTH-006 in-handler reauth; freeze `372fbfa`
Reviewer/date: UI review (2026-08-25) on `7b20bd8` approve Partial UI;
 no Critical/High/Medium. Frozen copy holds: title Delete Redis user;
 connections terminated; keys not deleted; cannot be undone. Page intro no
 longer says “Delete is not available in this slice.” Danger token holds:
 .danger-button background var(--danger), not --redis. Secret clearing
 holds in memory (200/401/dismiss/inspect-other/logout; reauth_required
 clears password only). Search never DELETE; login never DELETE; protected
 tests still hide Delete. Explicitly NOT viewport/zoom sign-off. Missing
 evidence (non-blocking): Playwright viewports; section-change unmount
 without dedicated jsdom; enable/rotate/edit in-flight disable not
 separately asserted; reviewer did not re-run npm tests. Optional polish
 (non-blocking): dialog does not echo username; background not inert;
 focus does not restore to trigger; 403 protected/503 leave password.
Keep REDIS-008 Partial. Keep AUTH-006 Partial. Keep PG-012 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
 Playwright 360×800 / 768×1024 / 1280×800 / 1600×1000 + 200% zoom, live
 Redis 8.2/8.8, COMPATIBILITY.md §6 remain Complete blockers.
Not pushed.
```

## REDIS-008 ACL delete security pin (2026-08-25)

```text
Requirement: REDIS-008 Partial + AUTH-006 Partial (DELETE /api/v1/redis/users/{username}
 in-handler reauth; inspector Delete; no POST /api/v1/auth/reauth)
Decision/ADR: AUTH-006 in-handler LookupOwnerByUsername + Verify; freeze `372fbfa`
Reviewer/date: Security review (2026-08-25) on `062fb4c` approve Partial;
 no Critical/High/Medium. Freeze check order holds: redis.destructive +
 CSRF; exact username_confirmation (no audit); reauth_required username-only
 audit (never reason: reauth, never password); AUTH-005 login_attempts
 unchanged; no 429; protected names never ACL DELUSER; 200 {request_id}
 only; audit-fail after DELUSER is 503 fail-closed; go-redis v9.22.0;
 no CLIENT KILL/KEYS/DEL/FLUSH*; no feature flag; collection DELETE 405.
 Residual questions (non-blocking): missing-owner HTTP mostly unreachable
 behind session JOIN; admin protection includes configured Redis URL
 username. Reviewer did not re-run tests. Independent UI pin `062fb4c`
 already landed on `7b20bd8`.
Keep REDIS-008 Partial. Keep AUTH-006 Partial. Keep PG-012 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
 Live Redis 8.2/8.8, COMPATIBILITY.md §6, dedicated reauth throttle,
 PostgreSQL reauth consumers remain Complete blockers.
Not pushed.
```

## REDIS-008 ACL delete evidence pin (2026-08-25)

```text
Requirement: REDIS-008 Partial + AUTH-006 Partial (DELETE /api/v1/redis/users/{username}
 in-handler reauth + inspector Delete; no POST /api/v1/auth/reauth)
Decision/ADR: AUTH-006 in-handler; freeze `372fbfa`
Reviewer/date: Evidence review (2026-08-25) on `bae3c8f` keep-Partial /
 reject-Complete. Freeze criteria map to implementation and claimed tests.
 Status rows AUTH-006 / REDIS-008 Partial; AGENTS.md current truth names
 this DELETE only. Canonical API/UX/SECURITY/DATA_AND_SECRETS/ARCHITECTURE
 updated at freeze. Historical TRACEABILITY “not started” slices left
 archival. Independent UI pin `062fb4c` and security pin `bae3c8f` already
 landed (reviewers did not re-run tests). Parent command set remains after
 `967e156`. This reviewer did not re-run tests. Do not mark Complete.
Keep REDIS-008 Partial. Keep AUTH-006 Partial. Keep PG-012 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
 Verifier pending. Not pushed.
```

## REDIS-008 ACL delete verifier PASS Partial (2026-08-25)

```text
Requirement: REDIS-008 Partial + AUTH-006 Partial (DELETE /api/v1/redis/users/{username}
 in-handler reauth + inspector Delete; no POST /api/v1/auth/reauth)
Decision/ADR: AUTH-006 in-handler LookupOwnerByUsername + Verify; freeze `372fbfa`
Verifier/date: independent verifier (2026-08-25) on HEAD
 `c945811befd6805bc2f7c8d1aef4d8a9b77ee685`. Tracked tree clean.
 PASS Partial. Do not mark Complete.
Freeze: check order holds; redis.destructive + CSRF; exact confirmation
 (400 no Redis no audit); reauth_required username-only audit (never
 reason: reauth; AUTH-005 unchanged; no 429); protected never DELUSER;
 200 {request_id} only; audit-fail after DELUSER is 503 fail-closed;
 collection DELETE 405; no POST /api/v1/auth/reauth; go-redis v9.22.0.
 UI: Delete for non-protected when list ok; hidden for protected/loading/
 unavailable/not_configured; .danger-button --danger not --redis; dialog
 Delete Redis user; 200 clears secrets/selection; reauth_required stays
 and clears password only; search/login never DELETE.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/auth ./internal/redisadmin ./internal/httpapi
 → ok auth 15.015s; redisadmin 5.409s; httpapi 137.982s
 go test -count=1 ./... → all ok (httpapi 120.770s; auth 14.208s;
 redisadmin 5.611s; cmd/redgres 7.341s; migrations no tests)
 go test -race -count=1 ./internal/auth → ok 11.313s
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 go list -m github.com/redis/go-redis/v9 → v9.22.0
 npm --prefix web run test:run → Tests 242 passed (242), 65.16s
 (first vitest run timed out under parallel Go load; rerun alone passed)
Unexecuted: live Redis 8.2/8.8, COMPATIBILITY.md §6, Playwright viewports,
 dedicated reauth throttle, PostgreSQL drop/truncate/row-delete,
 POST /api/v1/auth/reauth, CI, Node 24.19.0, gitleaks, govulncheck,
 go test -race ./...
No secret artifacts in 372fbfa..HEAD. Prior UI `062fb4c`, security
 `bae3c8f`, evidence `c945811` pins stand (this verifier ran on evidence
 HEAD).
Keep REDIS-008 Partial. Keep AUTH-006 Partial. Keep PG-012 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
 Not pushed.
```

## PG-005 POST connection reveal Partial (2026-08-25)

```text
Requirement: PG-005 Partial (POST /api/v1/postgres/databases/{db}/connection/reveal
 + inspector Reveal ticket). Keep PG-005 Partial. Keep PG-004 Partial (GET
 /connection still no decrypt). Keep PG-012 / REDIS-008 / AUTH-006 Partial.
 Do not mark Complete. No Gate 4, no live PostgreSQL, no POST rotate/create,
 no POST /api/v1/auth/reauth, no ensure_vault, no GET decrypt.
Decision/ADR: ADR-004; freeze `cece386`. AUTH-006 does not apply.
Source: database-app load_role_password / reveal_database_connection_url at
 1c3e8e2 (read-only). Did not copy ensure_vault, sibling 404-on-InvalidToken,
 FastAPI no-CSRF, or {direct_url, has_saved_password}. Fixtures
 internal/secrets/testdata/python49.json (cryptography==49.0.0).
Implementation files: internal/config/{config.go,postgres.go,postgres_test.go};
 internal/postgresadmin/{adapter.go,connection.go,memory.go,service.go,types.go,
 reveal_test.go,vault_file_test.go,connection_test.go,service_test.go};
 internal/httpapi/{server.go,postgres_routes.go,postgres_routes_test.go,
 postgres_reveal_routes_test.go};
 web/src/api/postgres.ts; web/src/features/postgres/DatabasesPage.tsx;
 web/src/features/redis/CredentialTicket.tsx (kind=postgres; Redis copy
 unchanged); web/src/features/pages/Placeholders.tsx (csrf to DatabasesPage);
 web/src/App.test.tsx.
Unit/HTTP: vault file not in PostgresConfigured/postgresAnySet; production
 0600; empty/unreadable named env var; Open stores derived key and wipes raw
 secret. Reveal: python49 ASCII decrypt; missing row 404; vault/secret/invalid
 token 503; empty owner 404; protected never SELECT; GET connection still
 SavedRoleNames only. HTTP: 401; 403 CSRF; 400 invalid name no audit; 404;
 503 canary absent; 200 {request_id} + one_time false + username=owner +
 password + omitted-or-present urls; audit metadata database+owner only;
 audit-fail 503 no credential; POST /connection 405; GET /reveal 405.
UI: present shows Reveal text-button; missing/not_available/loading hide;
 no confirm dialog; CSRF + encodeURIComponent + empty body; 200 PostgreSQL
 alertdialog frozen title/copy; Direct/Pooled URL copy only when keys present;
 no setItem; 401/404/503 no leftover password/no ticket; Security overview
 still no Reveal; login/search never POST reveal; Redis tickets still shown now.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/pg-005-reveal-api `7bfb569`:
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/config ./internal/postgresadmin ./internal/httpapi
   → ok config 1.972s; postgresadmin 1.592s; httpapi 107.222s
  go test -count=1 ./... → ok (httpapi 56.592s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
 Writer UI feat/pg-005-reveal-ui `ad2046a`:
  npm --prefix web run test:run → Tests 252 passed (252), 49.10s
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 Parent after merges `9e00819` (API) `d9a797e` (UI):
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/config ./internal/postgresadmin ./internal/httpapi
   → ok config 0.942s; postgresadmin 1.550s; httpapi 60.746s
  go test -count=1 ./... → all ok (httpapi 50.728s; cmd/redgres 5.355s;
   postgresadmin 2.024s; web 1.204s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Tests 252 passed (252), 50.21s
Not run: live PostgreSQL 17/18, Gate 4, COMPATIBILITY.md §6, Playwright,
 gitleaks, govulncheck, CI, race, Node 24.19.0.
Known limitations: AUTH-006 does not apply. POSIX 0600 vault-file tests skip
 on Windows. Reveal SELECTs ciphertext before checking empty vaultKey
 (still 503). Page header still says “Passwords are not revealed.”
Local commits: `cece386` (freeze), `7bfb569` (API), `9e00819` (merge API),
 `ad2046a` (UI), `d9a797e` (merge UI), this docs record.
 Not pushed.
Reviewer/date: UI and security Approve Partial on `8e411ff`; evidence
 PASS Partial on `f7ed0de`; verifier pending. Keep Partial.
Keep PG-005 Partial. Keep PG-004 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial. Do not mark Complete.
```

## PG-005 POST reveal UI pin (2026-08-25)

```text
Requirement: PG-005 Partial (inspector Reveal ticket; GET connection still no decrypt)
Decision/ADR: ADR-004; freeze `cece386`
Reviewer/date: UI review (2026-08-25) on `8e411ff` approve Partial;
 no Critical/High/Medium. Frozen copy holds: title This PostgreSQL password
 is still saved; vault-repeatable; not a one-time Redis credential. CSRF
 via Placeholders; encodeURIComponent; empty body; no confirm dialog.
 Clearing holds (dismiss/selection/logout; 401/404/503 no ticket).
 Security overview still no Reveal. Login/search never POST reveal. Redis
 tickets still shown now. Explicitly NOT viewport/zoom sign-off. Missing
 evidence (non-blocking): Playwright; focus trap (same as Redis tickets);
 dedicated Back-to-databases clearing test (page has no that control).
 Optional polish (non-blocking): header still says “Passwords are not
 revealed.”
Keep PG-005 Partial. Keep PG-004 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Playwright 360×800 / 768×1024 / 1280×800 / 1600×1000 + 200% zoom, live
 PostgreSQL 17/18, Gate 4, COMPATIBILITY.md §6 remain Complete blockers.
Not pushed.
```

## PG-005 POST reveal security pin (2026-08-25)

```text
Requirement: PG-005 Partial (POST /api/v1/postgres/databases/{db}/connection/reveal
 + inspector Reveal). Keep PG-004 Partial (GET /connection still no decrypt).
Decision/ADR: ADR-004; freeze `cece386`. AUTH-006 does not apply.
Reviewer/date: Security review (2026-08-25) on `8e411ff` approve Partial;
 no Critical/High/Medium. Freeze holds: session + postgres.credentials +
 CSRF; GET connection still postgres.read and SavedRoleNames only; empty
 body never owner_password; path 400 no catalog/vault/audit; protected
 never SELECT ciphertext; parameterized EncryptedPassword SQL; no
 ensure_vault; vault file fail-closed; derived key on Service only; unset
 file Open succeeds Reveal 503; audit-fail 503 no credential; 200 no-store
 one_time false; public host/ports sslmode=require; canary absent from
 audit/errors; no POST /api/v1/auth/reauth; pgx v5.10.0; go-redis v9.22.0.
 Residual questions (non-blocking): requireCapability is static owner
 allow-list (CSRF is the extra POST control vs GET). Reviewer could not
 re-check working-tree cleanliness from sandbox; parent tree at pin time
 is clean on `8e411ff`. Independent UI pin on the same product SHA.
Keep PG-005 Partial. Keep PG-004 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Gate 4, live PostgreSQL 17/18, Playwright, PG-003/PG-006, production
 vault-secret probe remain Complete blockers.
Not pushed.
```

## PG-005 POST reveal evidence pin (2026-08-25)

```text
Requirement: PG-005 Partial (POST /api/v1/postgres/databases/{db}/connection/reveal
 + inspector Reveal). Keep PG-004 Partial (GET /connection still no decrypt).
Decision/ADR: ADR-004; freeze `cece386`. AUTH-006 does not apply.
Reviewer/date: Evidence review (2026-08-25) on `f7ed0de` PASS Partial /
 reject-Complete. Freeze criteria map to implementation and claimed tests.
 Status rows PG-005/PG-004 Partial; AGENTS.md names POST reveal. Canonical
 API/UX/SECURITY/CONFIG/DATA_AND_SECRETS frozen at `cece386`. Hygiene after
 this review: CONFIGURATION intro no longer calls vault-secret target;
 PG-012 remainder no longer lists POST reveal / vault file as unimplemented.
 Independent UI and security Approve Partial on `8e411ff`. Parent command
 set remains after `d9a797e` / record `8e411ff`. This reviewer did not
 re-run tests. Over-mapped (non-blocking): HTTP 404 missing-vault-row is
 service-only; credentials-vs-read test uses isolated middleware; wipe
 asserted as derived-key inequality not zeroed bytes. Do not mark Complete.
Keep PG-005 Partial. Keep PG-004 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Verifier PASS Partial on `658f61d`. Not pushed.
```

## PG-005 POST reveal verifier PASS Partial (2026-08-25)

```text
Requirement: PG-005 Partial (POST /api/v1/postgres/databases/{db}/connection/reveal
 + inspector Reveal). Keep PG-004 Partial (GET /connection still no decrypt).
Decision/ADR: ADR-004; freeze `cece386`. AUTH-006 does not apply.
Verifier/date: independent verifier (2026-08-25) on HEAD
 `658f61d37df645191b82f9f8dfc087da9a412345`. Tracked tree clean.
 PASS Partial. Do not mark Complete.
Freeze: check order holds; postgres.credentials + CSRF; empty body never
 owner_password; path 400 no catalog/vault/audit; protected never SELECT;
 EncryptedPassword parameterized; no ensure_vault; vault file fail-closed;
 derived key on Service only; missing vault row 404; invalid token 503;
 audit-fail 503 no credential; 200 no-store one_time false; GET connection
 still SavedRoleNames only; no POST /api/v1/auth/reauth; pgx v5.10.0;
 go-redis v9.22.0.
 UI: present text-button Reveal; hidden missing/not_available/loading;
 no confirm; CSRF + encodeURIComponent + empty body; vault-repeatable
 ticket; Security overview no Reveal; login/search never POST reveal.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/config ./internal/postgresadmin ./internal/httpapi
 → ok config 2.347s; postgresadmin 1.190s; httpapi 44.882s
 go test -count=1 ./... → all ok (httpapi 77.710s; cmd/redgres 4.625s;
 postgresadmin 2.900s; web 0.958s; migrations no tests)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 npm --prefix web run test:run → Tests 252 passed (252), 49.49s
Unexecuted: Gate 4, live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, PG-003 create, PG-006 POST rotate, POST /api/v1/auth/reauth,
 ensure_vault, GET decrypt, production vault-secret probe, CI, Node 24.19.0,
 gitleaks, govulncheck, go test -race ./..., npm web build.
No secret artifacts in cece386..HEAD. Prior UI/security `8e411ff`, evidence
 `f7ed0de` pins stand (this verifier ran on evidence HEAD).
Keep PG-005 Partial. Keep PG-004 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Not pushed.
```

## PG-003 POST database create Partial (2026-08-25)

```text
Requirement: PG-003 Partial (POST /api/v1/postgres/databases create role+database
 + CONNECT lock + secrets.Encrypt + vault INSERT + compensation + Databases
 Create dialog). Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 / REDIS-008 / AUTH-006 Partial. Do not mark Complete. No Gate 4,
 no live PostgreSQL, no POST rotate, no POST /api/v1/auth/reauth, no ensure_vault,
 no client password, no create_role reuse.
Decision/ADR: ADR-004; freeze `7e85055`. AUTH-006 does not apply.
Source: database-app create_database / store_role_password /
 _ensure_console_can_set_role / generateOwnerName at 1c3e8e2 (read-only).
 Did not copy FastAPI no-CSRF, ensure_vault, client password, create_role reuse,
 ON CONFLICT upsert, or {created, direct_url, has_saved_password}.
 Fernet Encrypt: official spec and cryptography 49.0.0 fernet.py (no fernet-go,
 no new Go module).
Implementation files: internal/secrets/{fernet.go,fernet_test.go};
 internal/postgresadmin/{create.go,create_test.go,credentials.go,credentials_test.go,
 errors.go,types.go,policy.go,memory.go,service.go,service_test.go};
 internal/httpapi/{server.go,postgres_routes.go,postgres_routes_test.go,
 postgres_create_routes_test.go};
 web/src/api/postgres.ts; web/src/features/postgres/CreateDatabaseForm.tsx;
 web/src/features/postgres/DatabasesPage.tsx; web/src/features/pages/Placeholders.tsx;
 web/src/App.test.tsx. CredentialTicket.tsx unchanged (kind=postgres reuse).
Unit/HTTP: Encrypt roundtrip ASCII/Unicode fixture plaintext; invalid-token class
 unchanged. GeneratePassword length 32 / uniqueness. CREATE ROLE SQL
 LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 20.
 Protected 403 no DDL. Conflict 409 fields.database / fields.owner. Unknown field
 400. CSRF 403. 401 no credential keys. Missing vault key 503 no DDL. Vault INSERT
 failure compensates drop db+role. Role-create then db-fail drops role only.
 Canary password/token absent from audit/errors. 201 no-store one_time JSON false.
 GET list still 200. POST rotate unregistered. Audit postgres.database.create
 metadata database+owner only; audit-fail 503 no credential (cluster+vault remain).
UI: header Create database; hidden on list error/503; shown on HTTP 200 including
 empty; dialog title Create database; no password field; POST CSRF {database,owner};
 201 vault-repeatable ticket; 401/403/409/400/503 as frozen; postgres-create not
 placeholder; inspector/Security/search/login never POST create; Redis tickets
 still shown now; no setItem; nav/search Create does not open or POST while a
 ticket is open; list GET 401 after 201 clears ticket (session-expired copy).
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/pg-003-create-api `70cd1ab`:
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/secrets ./internal/postgresadmin ./internal/httpapi
   → ok secrets 0.844s; postgresadmin 1.602s; httpapi 82.180s
  go test -count=1 ./... → all ok (httpapi 79.607s)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
 Writer UI feat/pg-003-create-ui `584c075`:
  npm --prefix web run test:run → Tests 267 passed (267), 55.36s
  npm --prefix web run build → tsc + vite success
 Parent after merges `0b7b3e3` (UI) `d533e63` (API):
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/secrets ./internal/postgresadmin ./internal/httpapi
   → ok secrets 0.540s; postgresadmin 1.387s; httpapi 42.266s
  go test -count=1 ./... → all ok (httpapi 45.100s; cmd/redgres 4.620s;
   postgresadmin 1.982s; secrets 0.715s; web 0.957s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Tests 267 passed (267), 42.24s (after UI merge;
   API merge did not touch web)
 Parent UI correction `f49d9e5`:
  npm --prefix web run test:run → Tests 269 passed (269), 40.58s
Python Gate 2 not executed (Python 3.14.6 present; cryptography not installed).
Not run: live PostgreSQL 17/18, Gate 4, COMPATIBILITY.md §6, Playwright,
 gitleaks, govulncheck, CI, race, Node 24.19.0.
Known limitations: GRANT SET skipped when admin is empty or postgres or equals
 the new role. Unregistered POST rotate is chi 404. Header still says
 “Passwords are not revealed.” Compensated create failures are not audited
 (security Low L1; freeze requires success audit + nil-adapter failure only).
Local commits: `7e85055` (freeze), `70cd1ab` (API), `584c075` (UI),
 `0b7b3e3` (merge UI), `d533e63` (merge API), `0ec4af6` (docs record),
 `f49d9e5` (UI Medium 1–2 ticket guards).
 Not pushed.
Reviewer/date: Security Approve Partial on `0ec4af6`; UI first pass at
 `0ec4af6` (Medium 1–2); UI Approve Partial on correction `f49d9e5`
 (Medium 1–2 closed). Evidence PASS Partial on `b4c7575`; verifier PASS
 Partial on `53630c3`. Keep Partial.
Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Do not mark Complete.
```

## PG-003 POST create security pin (2026-08-25)

```text
Requirement: PG-003 Partial (POST /api/v1/postgres/databases create + vault INSERT
 + compensation + Databases Create dialog). Keep PG-004 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `7e85055`. AUTH-006 does not apply.
Reviewer/date: Security review (2026-08-25) on `0ec4af6` Approve Partial;
 no Critical/High/Medium. Freeze holds: session + postgres.provision + CSRF
 (requireMutation); not postgres.read / credentials / destructive; protected
 never DDL (403 before catalog mutation); missing vault key 503 before DDL;
 parameterized vault INSERT no ON CONFLICT / no ensure_vault; audit-fail 503
 no credential (cluster+vault remain); public host/ports sslmode=require;
 no POST /api/v1/auth/reauth; body {database, owner} only; pgx v5.10.0;
 go-redis v9.22.0; no fernet-go. UI at that SHA: CSRF header; POST body
 {database, owner}; no setItem.
 Low L1 (non-blocking): compensated create failures are not audited; leftover
 LOGIN role possible if OwnedDatabaseCount errors during compensation.
 Independent UI correction landed after this pin as `f49d9e5` (API unchanged).
Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Gate 4, live PostgreSQL 17/18, Playwright, PG-006 POST rotate,
 POST /api/v1/auth/reauth, ensure_vault, production vault-secret probe,
 Python Gate 2 remain Complete blockers.
Not pushed.
```

## PG-003 POST create UI pin (2026-08-25)

```text
Requirement: PG-003 Partial (Databases Create dialog + 201 vault-repeatable ticket).
 Keep PG-004 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `7e85055`.
Reviewer/date: UI review (2026-08-25) on `0ec4af6` Approve Partial with Medium
 1–2; correction `f49d9e5` UI re-review (2026-08-25) Approve Partial, Medium 1–2
 closed, no new Critical/High/Medium.
 Medium 1 closed: openCreate effect requires ticket === null; handleCreate
 no-ops while ticket open; createOpened.current latched only in that branch;
 search/nav share AppShell.go postgres-create on the same mounted page.
 Medium 2 closed: loadList 401 clearTicket, pendingSelect null, close create,
 session-expired copy; ticket unmounts.
 Explicitly NOT viewport/zoom/Playwright sign-off. This reviewer did not
 re-run npm tests; parent recorded 269 passed at `f49d9e5`.
 Low (out of freeze): header still says “Passwords are not revealed.”
 Missing (non-blocking): dedicated search-while-ticket and Reveal+nav tests
 (same ticket + openCreate path as the nav test).
Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Playwright 360×800 / 768×1024 / 1280×800 / 1600×1000 + 200% zoom, live
 PostgreSQL 17/18, Gate 4 remain Complete blockers.
Not pushed.
```

## PG-003 POST create evidence pin (2026-08-25)

```text
Requirement: PG-003 Partial (POST /api/v1/postgres/databases create + vault INSERT
 + compensation + Databases Create dialog + ticket-open guards). Keep PG-004
 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `7e85055`. AUTH-006 does not apply.
Reviewer/date: Evidence review (2026-08-25) on `b4c7575` PASS Partial /
 reject-Complete. Freeze criteria map to implementation and claimed tests.
 Status row PG-003 Partial; AGENTS.md names POST create + ticket-open guards
 + list GET 401 clear. Canonical freeze docs at `7e85055`. Independent
 security Approve Partial on `0ec4af6`; UI Approve Partial on `f49d9e5`.
 This reviewer did not re-run tests; parent recorded 269 passed at `f49d9e5`.
 Over-mapped (non-blocking): search-while-ticket shares nav path, no dedicated
 test; capability test proves export denied not provision wiring (server.go:91);
 GRANT skip empty-admin / admin==role implemented, only postgres-admin skip
 tested; writer SHAs 70cd1ab / 584c075 not on first-parent reflog.
 Historical remainder lists still name PG-003 unimplemented (Wave 0; PG-005
 remainder); current-facing TRACEABILITY:9 and AGENTS.md are Partial.
 Do not mark Complete.
Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Verifier PASS Partial on `53630c3`. Not pushed.
```

## PG-003 POST create verifier PASS Partial (2026-08-25)

```text
Requirement: PG-003 Partial (POST /api/v1/postgres/databases create + vault INSERT
 + compensation + Databases Create dialog + ticket-open guards). Keep PG-004
 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `7e85055`. AUTH-006 does not apply.
Verifier/date: independent verifier (2026-08-25) on HEAD
 `53630c3`. Tracked tree clean. PASS Partial. Do not mark Complete.
Freeze: session + postgres.provision + CSRF requireMutation; body {database,
 owner} DisallowUnknownFields; protected 403 no DDL; exists 409 fields; missing
 vault key 503 before DDL; CREATE ROLE CONNECTION LIMIT 20; QuoteIdentifier;
 password string-literal quoting; CREATE DATABASE simple-protocol Exec not in a
 transaction; GRANT SET skip empty/postgres/equals-new-role; 24-byte rand
 RawURLEncoding in postgresadmin; secrets.Encrypt no fernet-go; parameterized
 INSERT no ON CONFLICT no ensure_vault; 201 no-store one_time JSON false; audit
 postgres.database.create metadata database+owner only; audit-fail 503 no
 credential; compensation this-op vault then drop db then drop role if owns 0;
 POST rotate unregistered; no POST /api/v1/auth/reauth; pgx v5.10.0;
 go-redis v9.22.0.
 UI: header Create database; nav postgres-create is form; 201 existing postgres
 vault-repeatable ticket; CSRF {database, owner} no password; no setItem;
 ticket-open nav/search does not POST; list GET 401 clearTicket.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/secrets ./internal/postgresadmin ./internal/httpapi
 → ok secrets 0.517s; postgresadmin 1.096s; httpapi 24.575s
 go test -count=1 ./... → all ok (httpapi 38.861s; cmd/redgres 3.702s;
 postgresadmin 1.451s; secrets 0.515s; web 0.610s; migrations no tests)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 npm --prefix web run test:run → Tests 269 passed (269), 43.42s
Unexecuted: Gate 4, live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, Python Gate 2 (cryptography not installed), PG-006 POST rotate,
 POST /api/v1/auth/reauth, ensure_vault, production vault-secret probe, CI,
 Node 24.19.0, gitleaks, govulncheck, go test -race ./..., npm web build.
No secret artifacts in 7e85055..HEAD. Prior security `0ec4af6`, UI `f49d9e5`,
 evidence `b4c7575` pins stand (this verifier ran on evidence HEAD `53630c3`).
Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial.
 Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Not pushed.
```

## PG-006 POST credential rotate Partial (2026-08-25)

```text
Requirement: PG-006 Partial (POST /api/v1/postgres/databases/{db}/credentials/rotate
 ALTER ROLE + vault upsert + Databases inspector Rotate). Keep PG-006 Partial.
 Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial. Keep PG-012 /
 REDIS-008 / AUTH-006 Partial. Do not mark Complete. No Gate 4, no live
 PostgreSQL, no POST /api/v1/auth/reauth, no ensure_vault, no
 migrations/002_operations.sql, no client password, no sibling token_hex(32).
Decision/ADR: ADR-004; ADR-005 (no operations table); freeze `c9d8e27`.
 AUTH-006 does not apply.
Source: database-app rotate_database_owner_password / store_role_password /
 app.py rotate route at 1c3e8e2 (read-only). Did not copy FastAPI no-CSRF,
 ensure_vault, secrets.token_hex(32), ENABLE_PASSWORD_ROTATION, or
 {database, username, direct_url, pooled_url, warning}.
 Official PG 17 ALTER ROLE (PASSWORD + CONNECTION LIMIT).
Implementation files: internal/postgresadmin/{rotate.go,rotate_test.go,types.go,
 service.go,memory.go,errors.go}; internal/httpapi/{server.go,postgres_routes.go,
 postgres_rotate_routes_test.go,postgres_create_routes_test.go};
 web/src/api/postgres.ts; web/src/features/postgres/DatabasesPage.tsx;
 web/src/features/postgres/RotatePasswordDialog.tsx;
 web/src/features/redis/CredentialTicket.tsx (optional rotateWarning default
 off); web/src/App.test.tsx. create.go INSERT unchanged (no ON CONFLICT).
Unit/HTTP: eligibility 404/403 no ALTER; missing vault key / vault probe 503
 no ALTER; ALTER SQL QuoteIdentifier + quoteStringLiteral CONNECTION LIMIT 20;
 upsert ON CONFLICT; create INSERT still no ON CONFLICT; lock 409 no second ALTER;
 vault fail after ALTER → VaultUnsynced, 3 upsert retries, no password, no re-ALTER;
 401/CSRF/credentials; 200 no-store one_time JSON false; extra body field 400;
 confirmation mismatch 400 no audit; protected 403/404; audit
 postgres.credential.rotate metadata database+owner only; canary absent from
 audit/error JSON; audit-fail 503 no credential; GET/PUT/PATCH/DELETE 405.
UI: Rotate text-button when details loaded, owner non-empty, owner_can_login
 true, owner_is_superuser false (missing flags hide), including saved_credential
 missing. Hidden while details loading. Disabled while rotate/reveal/create in
 flight or ticket open. Confirm role=dialog title Rotate password?; typed
 database name; Rotate now disabled until exact match. POST CSRF +
 encodeURIComponent(db) + {"confirmation":"<db>"}. HTTP 200 vault-repeatable
 ticket plus form-warning. 401 session-expired, clear secrets. 400/403 stay on
 dialog. 404 / generic 503 inspector families. Vault-out-of-sync 503 stays on
 dialog. Security overview / search / login never POST rotate. Redis tickets
 stay shown now. No setItem.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/pg-006-rotate-api `286b10f`:
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 1.266s; httpapi 38.563s
  go vet ./... → no findings
 Writer UI feat/pg-006-rotate-ui `21ca8fd`:
  npm --prefix web run test:run → Tests 287 passed (287), 46.48s
  npm --prefix web run build → tsc + vite 8.2.2 (dist gitignored)
 Parent after merges `f53d5f8` (API) `acc1999` (UI):
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 1.094s; httpapi 31.134s
  go test -count=1 ./... → all ok (httpapi 42.875s; cmd/redgres 3.314s;
   postgresadmin 1.662s; secrets 0.858s; web 1.057s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Tests 287 passed (287), 49.74s
Not run: live PostgreSQL 17/18, Gate 4, COMPATIBILITY.md §6, Playwright,
 gitleaks, govulncheck, CI, race, Node 24.19.0, Python Gate 2.
Known limitations: in-process TryLock only; failure audit is nil-adapter +
 vault-unsynced (not 400/403/404/409); capability test proves export denied,
 not a provision-stripped owner. Header still says “Passwords are not
 revealed.” ALTER then vault-fail cannot restore the old password (rotate
 again is recovery).
Local commits: `c9d8e27` (freeze), `286b10f` (API), `21ca8fd` (UI),
 `f53d5f8` (merge API), `acc1999` (merge UI), this docs record.
 Not pushed.
Reviewer/date: Security review (2026-08-25) on `4c06f91` Approve Partial;
 UI review (2026-08-25) on `4c06f91` Approve Partial; evidence review
 (2026-08-25) on `f6a2ee9` PASS Partial / reject-Complete; verifier
 (2026-08-25) PASS Partial on `f6a2ee9`. Keep Partial.
Keep PG-006 Partial. Keep PG-003 Partial. Keep PG-004 Partial.
 Keep PG-005 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial.
 Keep AUTH-006 Partial. Do not mark Complete.
```

## PG-006 POST rotate security pin (2026-08-25)

```text
Requirement: PG-006 Partial (POST /api/v1/postgres/databases/{db}/credentials/rotate
 ALTER ROLE + vault upsert + Databases inspector Rotate). Keep PG-003 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; ADR-005 (no operations table); freeze `c9d8e27`.
 AUTH-006 does not apply.
Reviewer/date: Security review (2026-08-25) on `4c06f91` Approve Partial;
 no Critical/High/Medium. Freeze holds: session + postgres.credentials + CSRF
 (requireMutation); not postgres.read / provision / destructive; protected /
 ineligible 404 or 403 no ALTER; missing vault key and vault probe 503 before
 ALTER; ALTER QuoteIdentifier + quoteStringLiteral CONNECTION LIMIT 20;
 parameterized upsert ON CONFLICT; create INSERT still no ON CONFLICT;
 vault-fail after ALTER VaultUnsynced 3 retries no password no re-ALTER;
 200 no-store one_time JSON false; audit postgres.credential.rotate metadata
 database+owner only; audit-fail 503 no credential; no POST /api/v1/auth/reauth;
 body {confirmation} only; pgx v5.10.0; go-redis v9.22.0; no fernet-go.
 UI at that SHA: CSRF header; no setItem; Security overview never POSTs rotate.
 Low L1 (non-blocking): HTTP capability test proves export denied, not a
 credentials-stripped owner. Low L2 (accepted Partial): in-process per-owner
 TryLock only; two processes could interleave ALTER and vault upsert.
Keep PG-006 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Gate 4, live PostgreSQL 17/18, Playwright, POST /api/v1/auth/reauth,
 ensure_vault, dual-secret ADR, production vault-secret probe, durable
 cross-process lock remain Complete blockers.
Not pushed.
```

## PG-006 POST rotate UI pin (2026-08-25)

```text
Requirement: PG-006 Partial (Databases inspector Rotate + typed confirm + 200
 vault-repeatable ticket + rotate warning). Keep PG-003 Partial. Keep PG-004
 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `c9d8e27`.
Reviewer/date: UI review (2026-08-25) on `4c06f91` Approve Partial; no
 Critical/High/Medium. Freeze holds: Rotate text-button not --danger; show
 when details loaded + owner non-empty + owner_can_login true +
 owner_is_superuser false (missing flags hide), including vault missing;
 hidden while details loading; disabled while rotate/reveal/create in flight
 or ticket open; confirm role=dialog title Rotate password? same focus trap
 as Redis rotate; typed database name; Rotate now until exact match; POST CSRF
 + encodeURIComponent(db) + {confirmation}; HTTP 200 existing PostgreSQL
 ticket plus form-warning; 401 session-expired clear secrets; 400/403 stay on
 dialog; 404 / generic 503 inspector families no ticket; vault-out-of-sync
 503 stays on dialog; Security overview / search / login never POST rotate;
 Redis tickets stay shown now; no setItem; no auto-copy / no toast of secrets.
 Explicitly NOT viewport/zoom/Playwright sign-off. This reviewer did not
 re-run npm tests; parent recorded 287 passed at `acc1999`.
 Missing (non-blocking): dedicated missing-flags hide test; dedicated
 search-never-POSTs-rotate test; dedicated rotate-ticket dismiss/selection
 test (shared clearTicket with Reveal).
Keep PG-006 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Playwright 360×800 / 768×1024 / 1280×800 / 1600×1000 + 200% zoom, live
 PostgreSQL 17/18, Gate 4 remain Complete blockers.
Not pushed.
```

## PG-006 POST rotate evidence pin (2026-08-25)

```text
Requirement: PG-006 Partial (POST /api/v1/postgres/databases/{db}/credentials/rotate
 ALTER ROLE + vault upsert + Databases inspector Rotate). Keep PG-003 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `c9d8e27`. AUTH-006 does not apply.
Reviewer/date: Evidence review (2026-08-25) on `f6a2ee9` PASS Partial /
 reject-Complete. Freeze criteria map to implementation and recorded tests
 at `acc1999` (go test ./... ok; npm 287 passed). Status row PG-006 Partial;
 AGENTS.md names POST rotate + inspector Rotate. Canonical freeze docs at
 `c9d8e27`. Independent security Approve Partial on `4c06f91` (pin `8dea290`);
 UI Approve Partial on `4c06f91` (pin `f6a2ee9`). This reviewer did not
 re-run tests.
 Over-mapped (non-blocking): capability test proves export denied not a
 credentials-stripped owner; HTTP 409 untested at HTTP (domain lock test
 exists); missing vault key / vault probe 503 are domain-only; dedicated
 missing-flags hide / rotate-ticket dismiss tests remain UI-reviewer gaps.
 Do not mark Complete.
Keep PG-006 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Gate 4, live PostgreSQL 17/18, Playwright, POST /api/v1/auth/reauth,
 ensure_vault, dual-secret ADR, production vault-secret probe remain Complete
 blockers.
Not pushed.
```

## PG-006 POST rotate verifier PASS Partial (2026-08-25)

```text
Requirement: PG-006 Partial (POST /api/v1/postgres/databases/{db}/credentials/rotate
 ALTER ROLE + vault upsert + Databases inspector Rotate). Keep PG-003 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial.
Decision/ADR: ADR-004; freeze `c9d8e27`. AUTH-006 does not apply.
Verifier/date: independent verifier (2026-08-25) on product tree `f6a2ee9`
 (implementation `acc1999` / record `4c06f91`). Tracked product files
 unchanged vs `f6a2ee9` when HEAD later moved to evidence pin `b93aa5b`.
 PASS Partial. Do not mark Complete.
Freeze: session + postgres.credentials + CSRF requireMutation; body
 {confirmation} DisallowUnknownFields; confirmation equals path db; protected
 404 / ineligible 403 no ALTER; missing vault key and vault probe 503 before
 ALTER; GeneratePassword 24-byte RawURLEncoding; secrets.Encrypt no fernet-go;
 ALTER QuoteIdentifier + quoteStringLiteral CONNECTION LIMIT 20; parameterized
 upsert ON CONFLICT; create INSERT still no ON CONFLICT; TryLock 409 no second
 ALTER; vault-fail after ALTER VaultUnsynced 3 retries no password no re-ALTER;
 200 no-store one_time JSON false; audit postgres.credential.rotate metadata
 database+owner only; audit-fail 503 no credential; GET/PUT/PATCH/DELETE 405;
 no /rotate alias; no POST /api/v1/auth/reauth; pgx v5.10.0; go-redis v9.22.0.
 UI: inspector Rotate text-button; typed confirm; 200 vault-repeatable ticket
 plus form-warning; Security overview never POSTs rotate; no setItem.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin ./internal/httpapi
 → ok postgresadmin 1.370s; httpapi 27.233s
 go test -count=1 ./... → all ok (httpapi 27.377s; cmd/redgres 2.657s;
 postgresadmin 1.528s; secrets 0.620s; web 0.742s; migrations no tests)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 npm --prefix web run test:run → Tests 287 passed (287), 68.05s
Unexecuted: Gate 4, live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, Python Gate 2, POST /api/v1/auth/reauth, ensure_vault, dual-secret
 ADR, production vault-secret probe, CI, Node 24.19.0, gitleaks, govulncheck,
 go test -race ./..., npm web build.
No secret artifacts in c9d8e27..f6a2ee9. Prior security `8dea290`, UI
 `f6a2ee9`, evidence `b93aa5b` pins stand (this verifier ran on UI pin
 `f6a2ee9` before the evidence pin commit).
 Keep PG-006 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial. Keep AUTH-006 Partial.
 Not pushed.
```

## PG-010 POST database duplicate Partial (2026-08-25)

```text
Requirement: PG-010 Partial (POST /api/v1/postgres/databases/{db}/duplicate
 TEMPLATE clone + unique owner + vault INSERT + clone-only compensation +
 Databases inspector Duplicate). Keep PG-010 Partial. Keep PG-003 Partial.
 Keep PG-004 Partial. Keep PG-005 Partial. Keep PG-006 Partial. Keep PG-012 /
 REDIS-008 / AUTH-006 Partial. Do not mark Complete. No Gate 4, no live
 PostgreSQL, no 202, no 002_operations.sql, no feature flag, no
 POST /api/v1/auth/reauth, no ensure_vault, no client password, no REASSIGN
 OWNED, no ON CONFLICT upsert.
Decision/ADR: ADR-004; ADR-005 (no operations table); freeze `376f11e`.
 AUTH-006 does not apply.
Source: database-app duplicate_database / _transfer_clone_object_ownership /
 _assert_source_unchanged / _cleanup_created_database_and_role /
 _database_ownership_snapshot / _alter_owner_as at 1c3e8e2 (read-only).
 Did not copy FastAPI no-CSRF, ENABLE_DESTRUCTIVE_ACTIONS, client
 new_owner_password, ensure_vault, HTTP 500 str(e), or
 {duplicated, warning, transferred_from}.
 Official PostgreSQL 17/18: CREATE DATABASE cannot run in a transaction;
 TEMPLATE clone requires no other sessions; REASSIGN OWNED also reassigns
 shared objects.
Implementation files: internal/postgresadmin/{duplicate.go,duplicate_test.go,
 types.go,memory.go,errors.go,service.go};
 internal/httpapi/{server.go,postgres_routes.go,postgres_duplicate_routes_test.go};
 web/src/api/postgres.ts; web/src/features/postgres/DuplicateDatabaseForm.tsx;
 web/src/features/postgres/DatabasesPage.tsx; web/src/App.test.tsx.
 create.go INSERT unchanged (no ON CONFLICT). rotate upsert unchanged.
Unit/HTTP: CREATE DATABASE {new} TEMPLATE {source} OWNER {new_owner} quoted.
 terminate parameterized $1 on source datname. Package source has no
 REASSIGN OWNED / SET ROLE. Unique owner 400 before DDL. Existing db/role
 409. Protected source 404 / new name 403 no DDL. Fingerprint mismatch 503
 compensates clone+role only (source remains). Vault insert fail drops
 clone+role only. Missing vault key 503 no DDL. Concurrent Duplicate 409 copy
 "A database duplicate is already in progress." Rotate 409 copy unchanged.
 CSRF/session/postgres.provision. Unknown fields 400. 201 no-store one_time
 JSON false. Audit postgres.database.duplicate metadata database+owner+source
 only. Canary secrets absent.
UI: inspector Duplicate text-button not --danger; show when details loaded +
 owner_can_login true + owner_is_superuser false (missing flags hide). Hidden
 while details loading. Disabled while duplicate/create/reveal/rotate in
 flight or ticket open. Dialog title Duplicate database; terminate copy
 includes connection_count including 0; POST CSRF + encodeURIComponent(source)
 + {database, owner}; 201 vault-repeatable ticket; 401 session-expired clear
 secrets; 400/403/409 stay on dialog; isolation-rollback 503 stays on dialog;
 Security overview / search / login never POST duplicate; no setItem.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 Writer API feat/pg-010-duplicate-api `382e801`:
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 0.995s; httpapi 28.600s
  go vet ./... → no findings
 Writer UI feat/pg-010-duplicate-ui `158da45`:
  npm --prefix web run test:run → Tests 309 passed (309), 45.88s
  npm --prefix web run build → tsc + vite success
 Parent after merges `0dff8d1` (API) `158da45` (UI):
  gofmt -l cmd internal migrations → empty
  go test -count=1 ./internal/postgresadmin ./internal/httpapi
   → ok postgresadmin 1.255s; httpapi 42.681s
  go test -count=1 ./... → all ok (httpapi 42.585s; cmd/redgres 3.533s;
   postgresadmin 2.302s; secrets 1.060s; web 1.186s; migrations no tests)
  go vet ./... → no findings
  go build -o NUL ./cmd/redgres → success
  npm --prefix web run test:run → Tests 309 passed (309), 68.45s
Not run: live PostgreSQL 17/18, Gate 4, COMPATIBILITY.md §6, Playwright,
 gitleaks, govulncheck, CI, race, Node 24.19.0, Python Gate 2.
Known limitations: handler timeout 30s; large TEMPLATE clones remain a
 later-operations Complete limitation. Clone object transfer uses
 connectTarget (5s). Compensated Duplicate failures are not audited.
 In-process TryLock only. Header still says “Passwords are not revealed.”
Local commits: `376f11e` (freeze), `382e801` (API), `158da45` (UI),
 `0dff8d1` (merge API), this docs record.
 Not pushed.
Reviewer/date: UI Approve Partial on `26a2a62` (pin `a10e0d9`);
 security Approve Partial with conditions on `26a2a62` (pin `e10cc08`);
 catalog quoting / superuser-skip correction `1c0ad73`/`7d3b5a8`;
 security re-review Approve Partial on `7d3b5a8` (pin `8cf383a`); evidence
 PASS Partial on `8cf383a` (pin `4223a8c`); verifier PASS Partial
 (following pin). Keep Partial.
Keep PG-010 Partial. Keep PG-003 Partial. Keep PG-004 Partial.
 Keep PG-005 Partial. Keep PG-006 Partial. Keep PG-012 Partial.
 Keep REDIS-008 Partial. Keep AUTH-006 Partial. Do not mark Complete.
```

## PG-010 POST duplicate UI pin (2026-08-25)

```text
Requirement: PG-010 Partial (Databases inspector Duplicate + Duplicate
 database dialog + 201 vault-repeatable ticket + terminate disclosure).
 Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005 Partial. Keep
 PG-006 Partial.
Decision/ADR: ADR-004; freeze `376f11e`. AUTH-006 does not apply.
Reviewer/date: UI review (2026-08-25) on `26a2a62` Approve Partial; no
 Critical/High/Medium. Freeze holds: Duplicate text-button not --danger,
 inspector only not header/nav; show when details loaded + owner non-empty
 + owner_can_login true + owner_is_superuser false (missing flags hide);
 hidden while details loading; disabled while duplicate/create/reveal/rotate
 in flight or ticket open; dialog role=dialog title Duplicate database same
 focus trap as Create; fields New database name / Project user; suggest
 app_${database}; no password; form-warning unique owner + N connections
 terminated (N includes 0); Submit until valid identifiers and database !==
 source; POST CSRF + encodeURIComponent(source) + {database, owner}; HTTP
 201 existing PostgreSQL ticket This PostgreSQL password is still saved.;
 select new db after dismiss; 401 session-expired clear secrets; 400/403/409
 stay on dialog; 404 / generic 503 inspector families; isolation-rollback
 503 stays on dialog; no setItem; Security overview / search / login never
 POST duplicate; Redis tickets stay shown now.
 Explicitly NOT viewport/zoom/Playwright sign-off. This reviewer did not
 re-run npm tests; parent recorded 309 passed at `0dff8d1`.
 Lows (non-blocking, not corrected here): missing connection_count coerced
 to 0 in the warning (DetailsFacts paints —); open Duplicate dialog does
 not set mutationBusy (same as Create vs Rotate); source name in warning is
 displayText only (freeze requires displayText).
 Missing (non-blocking): Duplicate-specific Tab wrap; dedicated
 create/reveal/rotate-in-flight disable tests; Redis “shown now” after
 Duplicate exists; encodeURIComponent reserved-character CSRF test.
Keep PG-010 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-006 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial.
 Keep AUTH-006 Partial. Playwright 360×800 / 768×1024 / 1280×800 /
 1600×1000 + 200% zoom, live PostgreSQL 17/18, Gate 4 remain Complete
 blockers.
Not pushed.
```

## PG-010 POST duplicate security pin (2026-08-25)

```text
Requirement: PG-010 Partial (POST /api/v1/postgres/databases/{db}/duplicate
 TEMPLATE clone + unique owner + vault INSERT + clone-only compensation +
 inspector Duplicate). Keep PG-003 Partial. Keep PG-004 Partial. Keep
 PG-005 Partial. Keep PG-006 Partial.
Decision/ADR: ADR-004; freeze `376f11e`. AUTH-006 does not apply.
Reviewer/date: security review (2026-08-25) on `26a2a62` Approve Partial
 with conditions. No Critical/High. Confirmed: session + postgres.provision
 + CSRF; no AUTH-006; protected source 404 / ineligible 403 no DDL; HTTP
 identifiers ValidateIdentifier + QuoteIdentifier; terminate parameterized
 on source; no REASSIGN OWNED / SET ROLE; compensation clone/role/vault
 only; vault INSERT no ON CONFLICT; 201 no-store one_time JSON false; audit
 metadata database+owner+source only; Security overview / search never POST
 duplicate.
 Medium (must fix before this Partial is security-clean): ownership
 transfer `continue`s when catalog schema/relation/type/routine names fail
 HTTP QuoteIdentifier (`duplicate.go` formatAlter* + transfer loops). Silent
 skip is not the freeze skip list (protected / OwnerDenied / pg_*).
 Conditions: (1) fail closed or catalog-safe pgx.Identifier.Sanitize without
 the HTTP allow-list (reject empty/NUL); do not continue; (2) skip superuser
 object owners; do not GRANT … SET TRUE those roles.
 Lows (non-blocking for this Partial): leftover GRANT SET TRUE on
 non-skipped owners; pg_get_function_identity_arguments interpolated
 (freeze-allowed); compensated failures not audited; TEMPLATE datacl not
 stripped of source CONNECT.
 Keep PG-010 Partial. Do not mark Complete. UI pin `a10e0d9` stands.
Not pushed.
```

## PG-010 POST duplicate security correction (2026-08-25)

```text
Requirement: PG-010 Partial security conditions from pin `e10cc08`.
 Keep PG-010 Partial. Do not mark Complete.
Decision/ADR: ADR-004; freeze `376f11e` plus API.md step 18 catalog quoting.
 AUTH-006 does not apply.
Correction: QuoteCatalogIdentifier uses pgx.Identifier.Sanitize without
 HTTP ValidateIdentifier; empty and NUL fail closed (pgx Sanitize would
 strip NUL). formatAlter* and formatGrantCatalogRole use catalog quoting
 for clone object/current-owner names; HTTP new owner stays QuoteIdentifier.
 Transfer loops return quoting errors (no continue). Unknown relkind /
 prokind length fail closed. Clone SQLs LEFT JOIN pg_roles.rolsuper;
 skipCloneTransferOwner skips superuser before GRANT … SET TRUE.
Implementation files: internal/postgresadmin/{identifier.go,duplicate.go,
 identifier_test.go,duplicate_test.go}; docs/API.md; docs/SECURITY.md;
 AGENTS.md.
Commands executed locally (2026-08-25), go1.27.0 windows/amd64:
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin → ok 0.995s
 go test -count=1 ./internal/httpapi → ok 28.726s
 go test -count=1 ./... → all ok (httpapi 29.256s; postgresadmin 1.241s)
 go vet ./... → no findings
Reviewer/date: security re-review Approve Partial on `7d3b5a8` (pin
 `8cf383a`). UI pin `a10e0d9` unchanged. Evidence PASS Partial (pin
 `4223a8c`). Verifier PASS Partial (following pin).
Keep PG-010 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-006 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial.
 Keep AUTH-006 Partial.
Not pushed.
```

## PG-010 POST duplicate security re-review pin (2026-08-25)

```text
Requirement: PG-010 Partial (POST /api/v1/postgres/databases/{db}/duplicate
 TEMPLATE clone + unique owner + vault INSERT + clone-only compensation +
 inspector Duplicate). Keep PG-003 Partial. Keep PG-004 Partial. Keep
 PG-005 Partial. Keep PG-006 Partial.
Decision/ADR: ADR-004; freeze `376f11e` plus API.md step 18 catalog quoting.
 AUTH-006 does not apply.
Reviewer/date: security re-review (2026-08-25) on `7d3b5a8` (`376f11e..7d3b5a8`;
 correction `26a2a62..7d3b5a8`) Approve Partial. No Critical/High/Medium.
 Prior Medium closed: QuoteCatalogIdentifier (empty/NUL fail; Sanitize
 without HTTP allow-list); formatAlter* catalog names; transfer loops
 return quoting errors (no continue); Duplicate compensates. Condition 1
 closed. Condition 2 closed: clone SQLs LEFT JOIN pg_roles.rolsuper;
 skipCloneTransferOwner skips superuser before GRANT … SET TRUE.
 HTTP names still ValidateIdentifier + QuoteIdentifier. AUTH-006 does not
 apply. UI pin `a10e0d9` stands (no UI diff).
 Lows (non-blocking): leftover GRANT SET TRUE on non-skipped owners;
 pg_get_function_identity_arguments interpolated; unknown 1-char prokind
 defaults to ALTER FUNCTION (SQL error → compensate); compensated failures
 not audited; TEMPLATE datacl not stripped of source CONNECT.
Keep PG-010 Partial. Do not mark Complete. Evidence PASS Partial (pin
 `4223a8c`). Verifier PASS Partial (following pin).
 Gate 4, live PostgreSQL 17/18, Playwright, 202/operations remain Complete
 blockers.
Not pushed.
```

## PG-010 POST duplicate evidence pin (2026-08-25)

```text
Requirement: PG-010 Partial (POST /api/v1/postgres/databases/{db}/duplicate
 TEMPLATE clone + unique owner + vault INSERT + clone-only compensation +
 Databases inspector Duplicate). Keep PG-003 Partial. Keep PG-004 Partial.
 Keep PG-005 Partial. Keep PG-006 Partial.
Decision/ADR: ADR-004; freeze `376f11e`. AUTH-006 does not apply. No 202.
 No feature flag.
Reviewer/date: Evidence review (2026-08-25) on product tree `7d3b5a8` /
 pin `8cf383a` PASS Partial / reject-Complete. Freeze criteria map to
 implementation and recorded tests: gofmt empty; go test ./... ok at
 `0dff8d1` and after quoting (`httpapi` ~29s, `postgresadmin` ~1.2s);
 go vet clean; npm 309 passed at `0dff8d1` only (UI unchanged; not quoting
 evidence). Status row PG-010 Partial. Independent UI Approve Partial
 `26a2a62` (pin `a10e0d9`); security conditions `e10cc08`; correction
 `1c0ad73`; security re-review Approve Partial `7d3b5a8` (pin `8cf383a`).
 This reviewer did not re-run tests.
 Over-mapped (non-blocking): capability test proves export-denied not
 provision-wrapped handler; MemoryCatalog TransferCloneOwnership stub;
 package-wide REASSIGN scan; rotate 409 copy test in duplicate file.
 Do not mark Complete.
Keep PG-010 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-006 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial.
 Keep AUTH-006 Partial. Gate 4, live PostgreSQL 17/18, Playwright,
 202/operations, POST /api/v1/auth/reauth, ensure_vault, dual-secret ADR,
 production vault-secret probe remain Complete blockers.
Not pushed.
```

## PG-010 POST duplicate verifier PASS Partial (2026-08-25)

```text
Requirement: PG-010 Partial (POST /api/v1/postgres/databases/{db}/duplicate
 TEMPLATE clone + unique owner + vault INSERT + clone-only compensation +
 Databases inspector Duplicate). Keep PG-003 Partial. Keep PG-004 Partial.
 Keep PG-005 Partial. Keep PG-006 Partial.
Decision/ADR: ADR-004; freeze `376f11e`. AUTH-006 does not apply. No 202.
 No feature flag.
Verifier/date: independent verifier (2026-08-25) on product tree `7d3b5a8`
 / HEAD `4223a8c`. PASS Partial. Do not mark Complete.
Freeze: session + postgres.provision + CSRF requireMutation; body
 {database, owner} DisallowUnknownFields; unique owner; TEMPLATE clone
 execSimple no transaction; source terminate parameterized; no REASSIGN
 OWNED / SET ROLE; vault INSERT no ON CONFLICT; clone-only compensation;
 QuoteCatalogIdentifier empty/NUL fail closed; skip superuser object
 owners; 201 no-store one_time JSON false; audit metadata
 database+owner+source only; inspector Duplicate text-button; Security
 overview / search / login never POST duplicate; no AUTH-006; no 202;
 no REDGRES_FEATURE_POSTGRES_DUPLICATE.
Commands executed (2026-08-25), go1.27.0 windows/amd64, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/postgresadmin ./internal/httpapi
 → ok postgresadmin 1.029s; httpapi 27.618s
 go test -count=1 ./... → all ok (httpapi 31.953s; cmd/redgres 2.835s;
 postgresadmin 1.363s; secrets 0.687s; web 0.737s; migrations no tests)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 npm --prefix web run test:run → Tests 309 passed (309), 71.82s
Unexecuted: Gate 4, live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, Python Gate 2, 202/operations, POST /api/v1/auth/reauth,
 ensure_vault, dual-secret ADR, production vault-secret probe, CI, Node
 24.19.0, gitleaks, govulncheck, go test -race ./..., npm web build.
No secret artifacts in 376f11e..4223a8c. Prior UI `a10e0d9`, security
 `8cf383a`, evidence `4223a8c` pins stand.
 Keep PG-010 Partial. Keep PG-003 Partial. Keep PG-004 Partial. Keep PG-005
 Partial. Keep PG-006 Partial. Keep PG-012 Partial. Keep REDIS-008 Partial.
 Keep AUTH-006 Partial.
 Not pushed.
```

## PG-008 row delete + AUTH-006 Partial (2026-08-25)

```text
Requirement: PG-008 Partial + AUTH-006 Partial (GET
 /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key plus
 DELETE …/rows). Keep AUTH-006 Partial (Redis DELETE
 /api/v1/redis/users/{username} plus this DELETE). Keep PG-010 Partial.
 Do not mark Complete. Gate 4 does not apply.
Decision/ADR: freeze `a05da3d`. AUTH-006 is in-handler LookupOwnerByUsername
 + Verify on body owner_password (no POST /api/v1/auth/reauth). Feature
 flag REDGRES_FEATURE_POSTGRES_ROW_DELETE via envBool (unset=false;
 invalid Load names the env var and never echoes the value). Do not load
 truncate/drop keys. No TryLock. No 202. pgx stays v5.10.0.
Implementation: product tree fast-forward of feat/pg-008-row-delete-api
 `0e9f6fa` (`0e9f6fa2afad3c7666e07c5fcd015cb71fd8e7f0`) from freeze
 `a05da3d`. Writer worktree
 D:/code/github/Redgres-worktrees/pg-008-row-delete-api. GET primary-key:
 session + postgres.read, no CSRF, no flag; confirm BASE TABLE then
 information_schema.table_constraints JOIN key_column_usage on
 constraint_catalog/schema/name AND table_schema/table_name,
 constraint_type PRIMARY KEY, $1 schema $2 table, ORDER BY
 kcu.ordinal_position; QuoteCatalogIdentifier on catalog column names
 (empty/NUL fail closed); missing table 404; none []; composite all
 names. DELETE rows: session + postgres.destructive + CSRF + flag; flag
 off 403 "Row delete is turned off." before JSON decode, no audit, no
 PostgreSQL; DisallowUnknownFields (primary_key_column is unknown);
 table_confirmation exact path table; PK values 1–500
 string/number/bool; Reauthenticate; wrong password 403 reauth_required
 failure audit database+schema+table only, no SQL; protected 404 like
 GET rows, no DML; width ≠ 1 → 400 fields.primary_key "This table does
 not have a single-column primary key."; DELETE FROM quoted schema.table
 WHERE quoted pk IN ($1..$n); 200 {deleted, request_id} RowsAffected;
 success audit database+schema+table+deleted; audit-fail after DML →
 503. Timeout 30s. Sibling FastAPI no-CSRF / ENABLE_DESTRUCTIVE_ACTIONS /
 client PK column / ::regclass+LIMIT 1 / pg_index.indisprimary / HTTP 500
 str(e) not copied. Inspector UI not landed.
Commands executed (2026-08-25) on product tree after ff-merge, go1.27.0
 windows/amd64:
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok config 0.958s; postgresadmin 1.245s; httpapi
 37.844s
 go test -count=1 ./... → all ok (httpapi 39.949s; cmd/redgres 2.635s;
 postgresadmin 1.565s)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
Unexecuted: live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, go test -race, CI, Node 24.19.0, web UI checkboxes/dialog,
 POST /api/v1/auth/reauth, truncate/drop flags, Gate 4.
No secret artifacts in a05da3d..0e9f6fa.
Keep PG-008 Partial. Keep AUTH-006 Partial. Keep PG-010 Partial.
Not pushed.
```

## PG-008 row delete UI pin (2026-08-25)

```text
Requirement: PG-008 Partial UI + AUTH-006 Partial (this DELETE UI only).
 Keep PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
 Playwright is Complete-only.
Decision/ADR: freeze a05da3d. AUTH-006 is in-handler owner_password on
 DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows
 (REDIS-008 pattern). No POST /api/v1/auth/reauth. Capability
 postgres.destructive + CSRF + REDGRES_FEATURE_POSTGRES_ROW_DELETE.
 GET .../primary-key is postgres.read, no CSRF, no flag.
Implementation: product tree cherry-pick of feat/pg-008-row-delete-ui
 `b7ccacf` as `dad0c9f` (`dad0c9fcd0cc71165cca1cdf3f8670e0902c11e0`)
 after Go API `0e9f6fa`. Writer worktree
 D:/code/github/Redgres-worktrees/pg-008-row-delete-ui.
 web/src/api/postgres.ts — GET primary-key (no CSRF); DELETE /rows with
 CSRF and { table_confirmation, owner_password, primary_key_values }.
 web/src/features/postgres/DeleteSelectedRowsDialog.tsx — Redis Delete
 focus-trap pattern; title Delete selected rows.
 web/src/features/postgres/DatabasesPage.tsx — PK fetch with the row
 page (same abort as table change); checkboxes only when
 primary_key.length === 1; danger Delete selected; secret clearing on
 200/401/database/table/Back/logout.
 web/src/styles/globals.css — 44px row-select hit area; danger-button
 uses var(--danger) not --postgres.
 web/src/App.test.tsx — PG-008 UI coverage.
Commands executed (2026-08-25) on product tree after cherry-pick,
 Node v25.3.0 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI
 evidence):
 npm --prefix web run test:run → Tests 324 passed (324), 53.18s
 npm --prefix web run build → tsc --noEmit && vite build success
 (dist gitignored)
Unexecuted: Playwright viewports, live PostgreSQL 17/18, Gate 4,
 COMPATIBILITY.md §6, go test -race, CI, Node 24.19.0, gitleaks,
 govulncheck.
Known limitations: jsdom does not resolve CSS variables to computed
 RGB (danger vs postgres asserted via class + globals.css source).
 Flag-off 403 stays on dialog and does not clear password (only
 reauth_required does). Selection is not cleared on row paging/search.
 mutationBusy for Reveal/Rotate/Duplicate does not include deleting.
No secret artifacts in a05da3d..dad0c9f. Dist not committed.
Keep PG-008 Partial. Keep AUTH-006 Partial (Redis DELETE plus this
 DELETE). Keep PG-010 Partial. Do not mark Complete.
Not pushed.
```

## PG-008 row delete UI review pin (2026-08-25)

```text
Requirement: PG-008 Partial UI + AUTH-006 Partial (this DELETE UI only).
 Keep PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
 Playwright is Complete-only.
Decision/ADR: freeze a05da3d (still current at 47dfa4d). AUTH-006 is
 in-handler owner_password on DELETE
 /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows
 (REDIS-008 pattern). No POST /api/v1/auth/reauth.
Reviewer/date: UI review (2026-08-25) on code SHA dad0c9f / master
 47dfa4d. Diff range a05da3d..47dfa4d. Approve Partial. No freeze
 defects. Files: web/src/api/postgres.ts,
 web/src/features/postgres/DatabasesPage.tsx,
 web/src/features/postgres/DeleteSelectedRowsDialog.tsx,
 web/src/styles/globals.css, web/src/App.test.tsx. Compared to
 web/src/features/redis/DeleteAclUserDialog.tsx.
 Freeze holds: GET primary-key with row page no CSRF abort on table
 change; checkboxes only when primary_key.length === 1; danger Delete
 selected not --postgres; hidden while rows loading; disabled in-flight
 / ticket / zero selected; flag-off still shows control; 403 Row delete
 is turned off.; dialog role=dialog title Delete selected rows Redis
 focus trap; typed table + owner password; displayText schema.table;
 Cannot be undone.; autocomplete off / current-password; CSRF +
 encodeURIComponent + three frozen body fields; 200 close/clear/reload;
 reauth_required stay + clear password keep confirmation; 401
 session-expired clear secrets; selection clear on database/table/Back/
 logout; no setItem; search / login / Security overview never DELETE
 rows.
 Parent notes (flag-off 403 keeps password; mutationBusy omits deleting;
 selection persists on page/search; jsdom CSS vars) match the freeze.
 Explicitly NOT viewport/zoom/Playwright sign-off. This reviewer did not
 re-run npm tests; parent recorded 324 passed at dad0c9f (Node v25.3.0,
 not web/.nvmrc 24.19.0).
 Lows (non-blocking, not corrected here): off-page selection after
 paging/search; no focus restore to Delete selected after Cancel; unused
 errorId same as Redis Delete.
Keep PG-008 Partial. Keep AUTH-006 Partial (Redis DELETE plus this
 DELETE). Keep PG-010 Partial. Playwright 360×800 / 768×1024 / 1280×800 /
 1600×1000 + 200% zoom, live PostgreSQL 17/18, Gate 4 remain Complete
 blockers.
Not pushed.
```

## PG-008 row delete security pin (2026-08-26)

```text
Requirement: PG-008 Partial + AUTH-006 Partial
 (GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key
 + flagged DELETE …/rows). Keep AUTH-006 Partial (Redis DELETE plus this
 DELETE). Keep PG-010 Partial. Do not mark Complete. Gate 4 N/A.
Decision/ADR: freeze a05da3d. In-handler Reauthenticate. No POST
 /api/v1/auth/reauth. REDGRES_FEATURE_POSTGRES_ROW_DELETE via envBool.
Reviewer/date: security review (2026-08-26) on 47dfa4d
 (47dfa4ddf3488b44052f5c659094ae35a8536dbd); Go 0e9f6fa; UI dad0c9f;
 range a05da3d..47dfa4d. Approve Partial. Critical: none. High: none.
 Medium: none.
 Confirmed: GET primary-key session + postgres.read no CSRF no flag;
 DELETE session + postgres.destructive + CSRF; flag off 403 before JSON
 decode no audit no PostgreSQL; envBool unset false invalid names env
 never echoes value; DisallowUnknownFields primary_key_column unknown;
 table_confirmation exact path table; PK values 1–500 string/number/bool;
 AUTH-006 in-handler Reauthenticate no POST /api/v1/auth/reauth no 429
 no login_attempts; wrong password 403 reauth_required failure audit
 database+schema+table only no SQL; protected 404 no DML; information_schema
 PK join not ::regclass/indisprimary/LIMIT 1; width ≠ 1 400 no DELETE;
 quoted DELETE IN ($1..$n) QuoteCatalogIdentifier empty/NUL fail closed;
 success audit then 200; audit-fail after DML 503; UI CSRF +
 encodeURIComponent three frozen fields; no setItem; Search/login/
 Security overview never DELETE rows.
 Lows (non-blocking, not corrected here): L1 DML connection skips BASE
 TABLE re-confirm (TOCTOU; PrimaryKey in same Service.DeleteRows does
 confirm; not a freeze break). L2 pager/search retain selectedPks
 (freeze did not require clear).
 Parent notes verified non-defects: JSON numbers→float64; flag-off 403
 keeps password; audit target=database + schema/table metadata.
 UI pin b7e826c stands. Keep PG-008 Partial. Keep AUTH-006 Partial.
 Keep PG-010 Partial. Do not mark Complete.
Not pushed.
```

## PG-008 row delete evidence pin (2026-08-26)

```text
Requirement: PG-008 Partial + AUTH-006 Partial (GET
 /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key
 + flagged DELETE …/rows + inspector single-column PK checkboxes
 and danger Delete selected dialog). Keep AUTH-006 Partial (Redis
 DELETE /api/v1/redis/users/{username} plus this DELETE). Keep
 PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
 Gate 4 does not apply. Playwright is Complete-only.
Decision/ADR: freeze a05da3d. In-handler Reauthenticate. No POST
 /api/v1/auth/reauth. REDGRES_FEATURE_POSTGRES_ROW_DELETE via envBool.
Reviewer/date: Evidence review (2026-08-26) on product tree HEAD
 96ff6ff (96ff6ff3024f0ada1fece035b16bea1c0b399275). Diff
 a05da3d..96ff6ff. PASS Partial / reject-Complete.
 Freeze criteria map to implementation and recorded tests: GET
 primary-key session + postgres.read no CSRF no flag; DELETE
 session + postgres.destructive + CSRF; flag off 403 before JSON
 decode; envBool unset false; DisallowUnknownFields; AUTH-006
 in-handler; inspector checkboxes only when primary_key.length===1;
 danger Delete selected dialog. Status rows AUTH-006 / PG-001..012
 Partial. This reviewer did not re-run tests.
 Parent Go evidence at 0e9f6fa (TRACEABILITY 4157–4166): gofmt
 empty; go test ./internal/config ./internal/postgresadmin
 ./internal/httpapi ok (httpapi 37.844s); go test ./... all ok
 (httpapi 39.949s); go vet clean; go build success; go1.27.0
 windows/amd64.
 Parent UI evidence at dad0c9f (TRACEABILITY 4201–4206): npm
 test:run 324 passed (324), 53.18s; npm run build success; Node
 v25.3.0 not web/.nvmrc 24.19.0.
 Independent UI Approve Partial b7e826c on dad0c9f / 47dfa4d;
 security Approve Partial 96ff6ff on 47dfa4d; Critical/High/Medium
 none; L1/L2 freeze-aligned. No freeze defects. No secret/runtime
 artifacts. Stale “UI not landed” only in historical Go pin 4156.
 Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth,
 truncate/drop flags, gitleaks, govulncheck.
Keep PG-008 Partial. Keep AUTH-006 Partial. Keep PG-010 Partial.
 Keep PG-007 row-browse. Do not mark Complete.
Not pushed.
```

## PG-008 row delete verifier PASS Partial (2026-08-26)

```text
Requirement: PG-008 Partial + AUTH-006 Partial (GET
 /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key
 + flagged DELETE …/rows + inspector single-column PK checkboxes
 and danger Delete selected dialog). Keep AUTH-006 Partial (Redis
 DELETE /api/v1/redis/users/{username} plus this DELETE). Keep
 PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
 Gate 4 does not apply. Playwright is Complete-only.
Decision/ADR: freeze a05da3d. In-handler Reauthenticate. No POST
 /api/v1/auth/reauth. REDGRES_FEATURE_POSTGRES_ROW_DELETE via envBool.
Verifier/date: independent verifier (2026-08-26) on product tree HEAD
 b659e45 (b659e450232fd858b7d04917f6482ddc92b5f28c). Diff
 a05da3d..b659e45. Go 0e9f6fa; UI dad0c9f. PASS Partial. Do not mark
 Complete.
 Independently re-ran (go1.27.0 windows/amd64; Node v25.3.0 not
 web/.nvmrc 24.19.0):
 gofmt -l cmd internal migrations → empty (267 ms)
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok config 0.523s; postgresadmin 0.957s;
 httpapi 27.857s (32700 ms)
 go test -count=1 ./... → all ok (httpapi 32.018s; cmd/redgres
 2.412s; postgresadmin 1.400s) (36300 ms)
 go vet ./... → no findings (1260 ms)
 go build -o NUL ./cmd/redgres → success (2630 ms)
 npm --prefix web run test:run → Tests 324 passed (324), 52.93s
 npm --prefix web run build → tsc --noEmit && vite build success
 (dist gitignored)
 Freeze implemented: GET primary-key session + postgres.read no
 CSRF no flag; information_schema PK join not ::regclass /
 indisprimary / LIMIT 1; QuoteCatalogIdentifier empty/NUL fail
 closed; DELETE session + postgres.destructive + CSRF; flag off
 403 before JSON decode; envBool unset false; DisallowUnknownFields
 primary_key_column unknown; AUTH-006 in-handler; inspector
 checkboxes only when primary_key.length===1; danger Delete
 selected dialog. Negative cases inspected in source and covered
 by tests that passed this run. Status rows AUTH-006 / PG-001..012
 Partial. Freeze docs API.md + UX.md unchanged after a05da3d.
 Independent UI Approve Partial b7e826c; security Approve Partial
 96ff6ff; prior evidence PASS Partial / reject-Complete on 96ff6ff.
 No secret/runtime artifacts. Dist not committed. Worktree clean.
Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth,
 truncate/drop flags, gitleaks, govulncheck.
Keep PG-008 Partial. Keep AUTH-006 Partial. Keep PG-010 Partial.
 Keep PG-007 row-browse. Do not mark Complete.
Not pushed.
```

## PG-009 truncate compatibility pin (2026-08-26)

```text
Requirement: PG-009 Partial freeze facts (not implementation). Keep
 PG-008 Partial. Keep PG-010 Partial. Do not mark Complete.
Decision/ADR: freeze 74bf746 (SQL unchanged). Compatibility research
 (2026-08-26) on PostgreSQL 17 vs 18 official TRUNCATE and
 information_schema.tables (docs as of PG 17.11 / 18.6). No 17/18
 TRUNCATE syntax delta for CASCADE / RESTRICT / RESTART IDENTITY /
 CONTINUE IDENTITY / ONLY. Omit ONLY is required when a partitioned
 parent is listed (TRUNCATE ONLY on a partitioned table errors, 17
 and 18 §5.12.2.3). BASE TABLE lists partitioned parents and local
 partitions; foreign partitions are FOREIGN and omitted from the
 list but can still be truncated as descendants of a named parent
 (FDW failure → 503). Do not filter information_schema.enforced
 (not a column of tables). RESTART IDENTITY restarts sequences
 owned by truncated columns; standalone sequences unchanged.
 Sibling 1c3e8e2 per-table CASCADE + swallowed exceptions is
 do-not-copy. Live COMPATIBILITY.md §6 not executed. pgx stays
 v5.10.0.
Keep PG-009 Partial (writers in flight). Keep PG-008 Partial.
Not pushed.
```

## PG-009 truncate API (2026-08-26)

```text
Requirement: PG-009 Partial + AUTH-006 Partial (this POST
 /api/v1/postgres/databases/{db}/truncate). Keep PG-008 Partial.
 Keep PG-010 Partial. Keep AUTH-006 Partial (Redis DELETE
 /api/v1/redis/users/{username} plus PG-008 row DELETE plus this
 POST). Do not mark Complete. Do not start PG-011. Gate 4 N/A.
Decision/ADR: freeze 74bf746. AUTH-006 is in-handler
 LookupOwnerByUsername + Verify on body owner_password (no POST
 /api/v1/auth/reauth). Feature flag REDGRES_FEATURE_POSTGRES_TRUNCATE
 via envBool (unset=false; invalid Load names the env var and never
 echoes the value). Do not use envBoolDefaultFalse. Do not load DROP
 keys or ENABLE_DESTRUCTIVE_ACTIONS. In-process TryLock on database
 name only (dedicated map; not rotate/duplicate). No 202. pgx stays
 v5.10.0.
Implementation: product tree cherry-pick of feat/pg-009-truncate-api
 `9a2ce50` as `8f473e1` (`8f473e1da8f63dd9f1e2f3097d21e3c98b525824`)
 from freeze 74bf746 onto a4b2436. Writer worktree
 D:/code/github/Redgres-worktrees/pg-009-truncate-api.
 Route registered next to duplicate, before GET {db}:
 r.With(s.requireSession, s.requireCapability("postgres.destructive"),
 s.requireMutation).Post("/api/v1/postgres/databases/{db}/truncate",
 s.handlePostgresDatabaseTruncate).
 Session + postgres.destructive + CSRF. Flag off 403
 "Truncate is turned off." before JSON decode, no audit, no
 PostgreSQL. DisallowUnknownFields (confirmation / table_confirmation
 / password / tables / cascade unknown 400). database_confirmation
 exact path {db}; copy "Type the exact database name to confirm
 truncate". Wrong password 403 reauth_required, failure audit
 metadata database only, no SQL, no login_attempts, no 429.
 Protected/template/datallowconn=false/missing → 404 like GET tables,
 no TRUNCATE. listTablesSQL LIMIT 501; QuoteCatalogIdentifier
 empty/NUL fail closed; len>500 → 409 conflict "Table list is
 truncated. Truncate cannot run." no SQL. Zero tables: 200
 truncated 0, failed [], total_tables 0 after AUTH-006. One
 TRUNCATE TABLE {quoted schema.table, ...} RESTART IDENTITY (no
 CASCADE, no ONLY, no CONTINUE IDENTITY, no per-table loop).
 Success 200 {truncated, failed:[], total_tables, request_id};
 truncated is table count not list-cap flag; failed never null.
 Statement failure → 503, no partial 200. Audit-fail after TRUNCATE
 → 503. Audit action postgres.database.truncate; success metadata
 database + truncated + total_tables; failure database only.
 Timeout 30s. Cache-Control no-store. Sibling FastAPI DELETE /data /
 ENABLE_DESTRUCTIVE_ACTIONS / per-table CASCADE / swallowed
 exceptions / message envelope / HTTP 500 str(e) not copied.
Commands executed (2026-08-26) on product tree after cherry-picks,
 go1.27.0 windows/amd64:
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok config 2.620s; postgresadmin 1.685s;
 httpapi 42.632s
 go test -count=1 ./... → all ok (httpapi 47.802s; cmd/redgres
 3.408s; postgresadmin 1.512s; config 1.372s)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
Unexecuted: live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, go test -race, CI, Node 24.19.0, POST /api/v1/auth/reauth,
 DROP flag, Gate 4, gitleaks, govulncheck.
No secret artifacts in 74bf746..8f473e1.
Keep PG-009 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-010 Partial.
Not pushed.
```

## PG-009 truncate UI (2026-08-26)

```text
Requirement: PG-009 Partial UI + AUTH-006 Partial (this POST UI only).
 Keep PG-008 Partial. Keep PG-010 Partial. Do not mark Complete.
 Playwright is Complete-only.
Decision/ADR: freeze 74bf746. AUTH-006 is in-handler owner_password on
 POST /api/v1/postgres/databases/{db}/truncate (REDIS-008 / PG-008
 pattern). No POST /api/v1/auth/reauth. Capability
 postgres.destructive + CSRF + REDGRES_FEATURE_POSTGRES_TRUNCATE.
Implementation: product tree cherry-pick of feat/pg-009-truncate-ui
 `20bf56b` as `15e2468` (`15e246828354fa1acc444af507b024a86b4426bb`)
 after Go API `8f473e1`. Writer worktree
 D:/code/github/Redgres-worktrees/pg-009-truncate-ui.
 web/src/api/postgres.ts — POST /truncate with CSRF,
 encodeURIComponent(db), JSON { database_confirmation, owner_password }
 only.
 web/src/features/postgres/TruncateProjectDataDialog.tsx — Redis Delete /
 PG-008 focus-trap; title Truncate project data; Confirm Truncate.
 web/src/features/postgres/DatabasesPage.tsx — inspector danger Truncate
 when details loaded (not rotation-eligible); hidden while details
 loading; disabled while truncate/reveal/rotate/duplicate/create/
 row-delete in flight or a credential ticket is open; 200 reloads
 tables and the current row page if a table is selected; secret
 clearing on 200/401/database change/Back/logout.
 web/src/App.test.tsx — PG-009 UI coverage (danger-button, dialog
 title/copy/fields, confirm disabled until name+password, CSRF +
 encodeURIComponent + two body keys, 200 clear/reload, reauth_required
 password clear, 401 session-expired, 403 Truncate is turned off.,
 ticket disables Truncate, Search/login/Security overview never POST
 /truncate, no setItem).
Commands executed (2026-08-26) on product tree after cherry-picks,
 Node v25.3.0 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI
 evidence):
 npm --prefix web run test:run → Tests 338 passed (338), 73.75s
 npm --prefix web run build → tsc --noEmit && vite build success
 (dist gitignored)
Unexecuted: Playwright viewports, live PostgreSQL 17/18, Gate 4,
 COMPATIBILITY.md §6, go test -race, CI, Node 24.19.0, gitleaks,
 govulncheck.
Known limitations: jsdom does not resolve CSS variables to computed
 RGB (danger vs postgres asserted via class + globals.css source).
 Flag-off 403 stays on dialog and does not clear password (only
 reauth_required does). Delete selected button is not disabled during
 truncate in flight (handler refuses). 409 truncated-list covered;
 in-progress 409 uses the same stay-on-dialog branch.
No secret artifacts in 74bf746..15e2468. Dist not committed.
Keep PG-009 Partial. Keep AUTH-006 Partial (Redis DELETE plus PG-008
 DELETE plus this POST). Keep PG-008 Partial. Keep PG-010 Partial.
 Do not mark Complete.
Not pushed.
```

## PG-009 truncate UI review Approve Partial (2026-08-26)

```text
Requirement: PG-009 Partial UI + AUTH-006 Partial (this POST UI only).
 Keep PG-008 Partial. Keep PG-010 Partial. Keep PG-007. Do not mark Complete.
 Playwright is Complete-only and was not run.
Decision/ADR: freeze 74bf746 (`74bf7461f82471311c1d7571352bfa7c36735cac`).
 Product tree also has a4b2436 compatibility notes; SQL shape unchanged.
 AUTH-006 is in-handler owner_password on POST
 /api/v1/postgres/databases/{db}/truncate (REDIS-008 / PG-008 pattern).
 No POST /api/v1/auth/reauth.
Review: redgres-ui-reviewer Approve Partial on
 c80ebbf (`c80ebbf98f275d3ca25d2ba542aa279a5ce3aa6d`) master.
 UI SHA 15e2468 (`15e246828354fa1acc444af507b024a86b4426bb`).
 Go SHA 8f473e1 (`8f473e1da8f63dd9f1e2f3097d21e3c98b525824`).
 Diff range 74bf746..c80ebbf.
 Confirmed freeze defects: none.
 Parent notes matching freeze (not defects): flag-off 403 stays on
 dialog and does not clear password (only reauth_required does);
 Delete selected is not disabled during truncate in flight (handler
 refuses); jsdom danger vs postgres asserted via class + globals.css;
 409 stay-on-dialog uses errorMessage with generic unavailable fallback
 if body copy is missing.
 PG-008 Delete selected control unchanged in spirit.
Files reviewed: web/src/api/postgres.ts;
 web/src/features/postgres/TruncateProjectDataDialog.tsx;
 web/src/features/postgres/DatabasesPage.tsx; web/src/App.test.tsx;
 compared web/src/features/redis/DeleteAclUserDialog.tsx and
 web/src/features/postgres/DeleteSelectedRowsDialog.tsx.
Unexecuted: Playwright viewports 360×800, 768×1024, 1280×800,
 1600×1000, 200% zoom; live PostgreSQL 17/18; COMPATIBILITY.md §6;
 Node 24.19.0.
Keep PG-009 Partial. Keep AUTH-006 Partial (Redis DELETE plus PG-008
 DELETE plus this POST). Keep PG-008 Partial. Keep PG-010 Partial.
 Do not mark Complete.
Not pushed.
```

## PG-009 truncate security review Approve Partial (2026-08-26)

```text
Requirement: PG-009 Partial + AUTH-006 Partial (flagged POST
 /api/v1/postgres/databases/{db}/truncate + inspector Truncate dialog).
 Keep PG-008 Partial. Keep PG-010 Partial. Keep AUTH-006 Partial
 (Redis DELETE /api/v1/redis/users/{username} plus PG-008 row DELETE
 plus this POST). Do not mark Complete. Do not start PG-011.
Decision/ADR: freeze 74bf746 (`74bf7461f82471311c1d7571352bfa7c36735cac`).
 Compatibility pin a4b2436 (SQL unchanged). AUTH-006 in-handler
 LookupOwnerByUsername + Verify on body owner_password. No POST
 /api/v1/auth/reauth. Gate 4 N/A (no vault decrypt).
 Review: redgres-security-reviewer Approve Partial on
 c80ebbf (`c80ebbf98f275d3ca25d2ba542aa279a5ce3aa6d`) master.
 Go SHA 8f473e1 (`8f473e1da8f63dd9f1e2f3097d21e3c98b525824`).
 UI SHA 15e2468 (`15e246828354fa1acc444af507b024a86b4426bb`).
 Diff range 74bf746..c80ebbf.
 Confirmed defects: none (Critical/High/Medium).
 Sibling CASCADE/swallowed-exception path NOT copied (read-only
 database-app truncate_all_tables / DELETE /databases/{name}/data).
 Low/non-blocking: PoolCatalog Truncate Exec uses connectTarget 5s
 connectCtx (fail-closed 503, not a bypass); no dedicated HTTP
 statement-failure 503 test (service canary mapping exists).
 Files: internal/httpapi/server.go; internal/httpapi/postgres_routes.go
 (handlePostgresDatabaseTruncate); internal/httpapi/postgres_truncate_routes_test.go;
 internal/postgresadmin/truncate.go; internal/postgresadmin/truncate_test.go;
 internal/postgresadmin/adapter.go (listTablesSQL); internal/config/config.go;
 internal/config/truncate_test.go; internal/auth/reauth.go;
 web/src/api/postgres.ts; web/src/features/postgres/TruncateProjectDataDialog.tsx;
 web/src/features/postgres/DatabasesPage.tsx; web/src/App.test.tsx.
 Docs held: docs/SECURITY.md §3.1 / §6; docs/API.md POST truncate freeze;
 docs/DATA_AND_SECRETS.md; docs/CONFIGURATION.md.
 Unexecuted: live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright
 viewports, go test -race, CI, Node 24.19.0, gitleaks, govulncheck,
 this reviewer did not re-run product tests.
 Keep PG-009 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-010 Partial. Do not mark Complete.
 Not pushed.
```

## PG-009 truncate evidence PASS Partial (2026-08-26)

```text
Requirement: PG-009 Partial + AUTH-006 Partial (flagged POST
 /api/v1/postgres/databases/{db}/truncate plus inspector danger
 Truncate dialog, title Truncate project data). Keep PG-009 Partial.
 Keep AUTH-006 Partial (Redis DELETE /api/v1/redis/users/{username}
 plus PG-008 row DELETE plus this POST). Keep PG-008 Partial.
 Keep PG-010 Partial. Keep PG-007 row-browse. Reject Complete.
 Do not start PG-011. Gate 4 N/A. Playwright is Complete-only.
Decision/ADR: freeze 74bf746 (74bf7461f82471311c1d7571352bfa7c36735cac)
 plus compatibility pin a4b2436 (SQL unchanged). AUTH-006 in-handler
 LookupOwnerByUsername + Verify on body owner_password. No POST
 /api/v1/auth/reauth. Feature flag REDGRES_FEATURE_POSTGRES_TRUNCATE
 via envBool (unset=false). Do not use envBoolDefaultFalse.
Evidence review (2026-08-26) on product HEAD 8a20c99
 (8a20c9989982f06d1cd9d1a804317b42ff5c3929) master. Diff range
 74bf746..8a20c99. Go SHA 8f473e1 (cherry-pick of 9a2ce50).
 UI SHA 15e2468 (cherry-pick of 20bf56b). Combined code+impl
 TRACEABILITY c80ebbf. UI review pin 5845ce3 Approve Partial
 (reviewed c80ebbf). Security review pin 8a20c99 Approve Partial
 (reviewed c80ebbf). This reviewer did not re-run go test ./...
 or npm test (verifier-owned). Sandbox blocked git status/diff;
 HEAD log confirms the claimed SHA chain.
Freeze criteria mapped to implementation and recorded tests:
 POST registered next to duplicate before GET {db}
 (internal/httpapi/server.go); handlePostgresDatabaseTruncate
 flag-off 403 before decode; one TRUNCATE TABLE … RESTART IDENTITY
 (internal/postgresadmin/truncate.go formatTruncateSQL; no CASCADE,
 no ONLY, no per-table loop); listTablesSQL LIMIT 501; AUTH-006
 in-handler; inspector TruncateProjectDataDialog title Truncate
 project data. HTTP tests
 internal/httpapi/postgres_truncate_routes_test.go; domain
 internal/postgresadmin/truncate_test.go; config
 internal/config/truncate_test.go; jsdom web/src/App.test.tsx.
 Recorded parent commands (TRACEABILITY API/UI pins, not re-run):
 gofmt empty; go test ./internal/config ./internal/postgresadmin
 ./internal/httpapi ok (config 2.620s; postgresadmin 1.685s;
 httpapi 42.632s); go test ./... ok (httpapi 47.802s;
 cmd/redgres 3.408s); go vet; go build; npm test:run 338 passed
 (338) Node v25.3.0 not nvmrc 24.19.0; npm run build success.
 Security Lows (non-blocking): connectTarget 5s connectCtx vs
 handler 30s; no dedicated HTTP statement-failure 503 test.
 Sibling CASCADE/swallowed-exception not copied.
 Status rows AUTH-006 / PG-001..012 Partial; PG-011 not started.
 Historical “writers in flight” / PG-008 “Inspector UI not landed”
 left in historical pins only.
 No secret/runtime artifacts in inspected files. Dist not committed.
Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth, DROP flag,
 gitleaks, govulncheck.
Keep PG-009 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
Not pushed.
```

## PG-009 truncate verifier PASS Partial (2026-08-26)

```text
Requirement: PG-009 Partial + AUTH-006 Partial (flagged POST
 /api/v1/postgres/databases/{db}/truncate plus inspector danger
 Truncate dialog, title Truncate project data). Keep PG-009 Partial.
 Keep AUTH-006 Partial (Redis DELETE /api/v1/redis/users/{username}
 plus PG-008 row DELETE plus this POST). Keep PG-008 Partial.
 Keep PG-010 Partial. Keep PG-007 row-browse. Reject Complete.
 Do not start PG-011 from this pin. Gate 4 N/A. Playwright is
 Complete-only.
Decision/ADR: freeze 74bf746 (74bf7461f82471311c1d7571352bfa7c36735cac)
 plus compatibility pin a4b2436 (SQL unchanged). AUTH-006 in-handler
 LookupOwnerByUsername + Verify on body owner_password. No POST
 /api/v1/auth/reauth. Feature flag REDGRES_FEATURE_POSTGRES_TRUNCATE
 via envBool (unset=false). Do not use envBoolDefaultFalse.
Verification (2026-08-26) on product HEAD d78b33b
 (d78b33bffe3085c193ceabef97cd638039a3c045) master, clean worktree.
 Diff range 74bf746..d78b33b. Go SHA 8f473e1. UI SHA 15e2468.
 Combined code+impl TRACEABILITY c80ebbf. UI review pin 5845ce3
 Approve Partial (reviewed c80ebbf). Security review pin 8a20c99
 Approve Partial (reviewed c80ebbf). Prior evidence pin recorded
 as d78b33b. This verifier independently re-ran claimed commands.
Freeze criteria mapped to implementation and tests that passed:
 POST registered next to duplicate before GET {db}
 (internal/httpapi/server.go); handlePostgresDatabaseTruncate
 flag-off 403 before decode; one TRUNCATE TABLE … RESTART IDENTITY
 (internal/postgresadmin/truncate.go formatTruncateSQL; no CASCADE,
 no ONLY, no per-table loop); listTablesSQL LIMIT 501; AUTH-006
 in-handler; inspector TruncateProjectDataDialog title Truncate
 project data. HTTP tests
 internal/httpapi/postgres_truncate_routes_test.go; domain
 internal/postgresadmin/truncate_test.go; config
 internal/config/truncate_test.go; jsdom web/src/App.test.tsx.
Commands executed (2026-08-26), go1.27.0 windows/amd64,
 Node v25.3.0 (not web/.nvmrc 24.19.0):
 gofmt -l cmd internal migrations → empty (78 ms)
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok (config 0.637s; postgresadmin 1.059s;
 httpapi 38.408s; wall 40521 ms)
 go test -count=1 ./... → all ok (httpapi 36.650s; cmd/redgres
 2.238s; postgresadmin 1.453s; config 0.977s; wall 41588 ms)
 go vet ./... → no findings (965 ms)
 go build -o NUL ./cmd/redgres → success (3141 ms)
 npm --prefix web run test:run → Tests 338 passed (338), 55.65s
 npm --prefix web run build → tsc --noEmit && vite build success
 (dist gitignored at internal/web/dist/app/; not committed)
 Negative cases inspected in source + passing tests: flag off before
 JSON decode; CSRF; unknown fields including cascade/tables;
 confirmation mismatch no audit; reauth_required metadata database
 only not password; protected 404 no TRUNCATE; 501-table 409 no SQL;
 in-progress 409 dedicated lock map; empty tables 200 zeros; one
 TRUNCATE … RESTART IDENTITY no CASCADE no ONLY no per-table loop;
 canary password absent; POST 405; UI CSRF + two body fields;
 401/reauth_required password clearing; Search/login/Security
 overview never POST truncate.
 Status rows AUTH-006 / PG-001..012 Partial; PG-011 not started
 at verification time.
 No secret/runtime artifacts. Dist not committed.
Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth, DROP flag,
 gitleaks, govulncheck.
Keep PG-009 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-010 Partial. Keep PG-007 row-browse. Do not mark Complete.
Not pushed.
```


## PG-011 drop freeze (2026-08-26)

```text
Requirement: PG-011 Partial freeze + AUTH-006 Partial (DELETE
 /api/v1/postgres/databases/{db} contract only; not implemented).
 Keep PG-011 Partial (not implemented). Keep AUTH-006 Partial
 (Redis DELETE plus PG-008 row DELETE plus PG-009 truncate POST;
 drop reauth is this freeze, not yet code). Keep PG-008/009/010
 Partial. Reject Complete. Writers start after this pin. Gate 4 N/A.
Decision/ADR: this freeze commit. Product choice **BF-1**: HTTP does
 not check backups this Partial; UI discloses Recovery requires a
 valid external backup / Cannot be undone; no backup_confirmed body
 field; no checkbox-as-authorization; OPS-004 / SECURITY.md §6.7
 remain Complete. POST /api/v1/postgres/databases/{db} stays 405.
 Flag REDGRES_FEATURE_POSTGRES_DROP via envBool (unset=false).
 AUTH-006 in-handler LookupOwnerByUsername + Verify on body
 owner_password. Terminate with terminateDatabaseSQL (pid <>
 pg_backend_pid()); then quoted DROP DATABASE via execSimple; no
 WITH (FORCE); no 202; no migrations/002_operations.sql. Optional
 DROP ROLE + vault DELETE only when not OwnerDenied and
 OwnedDatabaseCount == 0. TryLock new database-name map; serialize
 with truncate map. Official DROP DATABASE 17/18 identical.
Implementation files: none (docs freeze).
Unit tests: none this pin.
Integration tests: none this pin.
Security tests: none this pin.
Deployment/migration impact: none. Phase 5 item 7 Partial is
 in-request; backup freshness skipped this Partial (BF-1).
Known limitations: PG-011 not implemented. Live PostgreSQL 17/18,
 Playwright, OPS-004 backup freshness, go test -race, CI, Node
 24.19.0, gitleaks, govulncheck remain Complete blockers.
Reviewer/date: parent freeze 2026-08-26. Writers blocked until this
 pin exists.
Keep PG-011 Partial freeze (not implemented). Keep PG-008/009/010
 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop API (2026-08-26)

```text
Requirement: PG-011 Partial + AUTH-006 Partial (this DELETE
 /api/v1/postgres/databases/{db} only). Keep PG-011 Partial.
 Keep AUTH-006 Partial (Redis DELETE plus PG-008 row DELETE plus
 PG-009 truncate POST plus this DELETE). Keep PG-008/009/010
 Partial. Reject Complete. Gate 4 N/A. Playwright Complete-only.
Decision/ADR: freeze b4674fb (b4674fb0d74a52a96dde5e27f15550f661de65b9).
 Product choice **BF-1**: HTTP does not check backups. AUTH-006
 in-handler LookupOwnerByUsername + Verify on body owner_password.
 No POST /api/v1/auth/reauth. Feature flag REDGRES_FEATURE_POSTGRES_DROP
 via envBool (unset=false). Do not use envBoolDefaultFalse.
Implementation: writer worktree
 D:/code/github/Redgres-worktrees/pg-011-drop-api branch
 feat/pg-011-drop-api 8fb1d72 (8fb1d72db4329c5de5df560c834dbdf4aa249fc1).
 Cherry-pick onto master 85372a6.
 internal/config/config.go — FeaturePostgresDrop via envBool.
 internal/config/drop_test.go — unset false; truthy/falsey; invalid
 names env never echoes value; DROP does not enable truncate/row-delete;
 ENABLE_DESTRUCTIVE_ACTIONS ignored.
 internal/postgresadmin/drop.go — Service.Drop; TerminateSessions then
 quoted DROP DATABASE (no IF EXISTS, no FORCE); optional DROP ROLE +
 vault DELETE; lock order truncateMu then dropMu.
 internal/postgresadmin/truncate.go — serialize with drop map.
 internal/postgresadmin/errors.go — DropInProgress / RoleDropFailed /
 VaultDeleteFailed.
 internal/httpapi/server.go — DELETE {db} with GET {db} (method-distinct)
 after suffix routes.
 internal/httpapi/postgres_routes.go — flag-off 403 before decode;
 AUTH-006; 30s; no-store; writePostgresError mappings.
 internal/httpapi/postgres_drop_routes_test.go — HTTP coverage.
Parent re-ran (2026-08-26) on integrated master 604a956 then this
 TRACEABILITY pin, go1.27.0 windows/amd64:
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok (config 1.448s; postgresadmin 1.173s;
 httpapi 36.429s)
 go test -count=1 ./... → all ok after dist rebuild (httpapi 49.409s;
 cmd/redgres 3.112s; postgresadmin 1.868s; config 2.313s)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
Known limitations: no live PG 17/18. DROP DATABASE succeeded + later
 503 (role/vault/audit) is fail-closed; database stays dropped.
 POST /api/v1/postgres/databases/{db} stays 405. BF-1 honored.
No secret artifacts. Dist gitignored (not committed).
Keep PG-011 Partial. Keep AUTH-006 Partial. Keep PG-008/009/010
 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop UI (2026-08-26)

```text
Requirement: PG-011 Partial UI + AUTH-006 Partial (this DELETE UI only).
 Keep PG-008 Partial. Keep PG-009 Partial. Keep PG-010 Partial.
 Do not mark Complete. Playwright is Complete-only.
Decision/ADR: freeze b4674fb (b4674fb0d74a52a96dde5e27f15550f661de65b9).
 AUTH-006 is in-handler owner_password on
 DELETE /api/v1/postgres/databases/{db} (REDIS-008 / PG-008 / PG-009
 pattern). No POST /api/v1/auth/reauth. Capability
 postgres.destructive + CSRF + REDGRES_FEATURE_POSTGRES_DROP.
 No backup_confirmed body field. No checkbox-as-authorization.
 Product choice BF-1: UI discloses Recovery requires a valid
 external backup / Cannot be undone.
Implementation: writer worktree
 D:/code/github/Redgres-worktrees/pg-011-drop-ui branch
 feat/pg-011-drop-ui 452a395 (452a3951eb6c695981be439e80131a96fad2bfb2).
 Cherry-pick onto master 604a956.
 web/src/api/postgres.ts — DELETE /api/v1/postgres/databases/{db}
 with CSRF, encodeURIComponent(db), JSON
 { database_confirmation, owner_password } only (no /drop suffix).
 web/src/features/postgres/DropDatabaseDialog.tsx — Truncate /
 Redis Delete focus-trap; title Drop database; Confirm Drop;
 permanently deletes / connections terminated / role only if no
 other database / Cannot be undone / Recovery requires a valid
 external backup; no checkbox.
 web/src/features/postgres/DatabasesPage.tsx — inspector danger
 Drop when details loaded (not rotation-eligible); hidden while
 details loading; disabled while drop/truncate/reveal/rotate/
 duplicate/create/row-delete in flight or a credential ticket is
 open; Truncate disabled while drop in flight; 200 refreshes list
 and clears inspector selection; secret clearing on
 200/401/database change/Back/logout.
 web/src/App.test.tsx — isPostgresDatabaseDrop (DELETE item path
 without /truncate /duplicate /connection /tables); PG-011 UI
 coverage (danger-button, Drop database vs Truncate project data,
 dialog copy/fields, confirm disabled until name+password, CSRF +
 encodeURIComponent + two body keys, 200 clear selection/refresh
 list, reauth_required password clear, 401 session-expired, 403
 Drop is turned off., Truncate/Drop disable each other, ticket
 disables Drop, Search/login/Security overview never DELETE a
 database, no setItem).
Parent re-ran (2026-08-26) on integrated master 604a956, Node v25.3.0
 (not web/.nvmrc 24.19.0; local npm is not nvmrc/CI evidence):
 npm --prefix web run test:run → Tests 353 passed (353), 62.56s
 npm --prefix web run build → tsc --noEmit && vite build success
 (dist gitignored at internal/web/dist/app/; not committed)
Unexecuted: Playwright viewports, live PostgreSQL 17/18, Gate 4,
 COMPATIBILITY.md §6, go test -race, CI, Node 24.19.0, gitleaks,
 govulncheck.
Known limitations: jsdom does not resolve CSS variables to computed
 RGB (danger vs postgres asserted via class + globals.css source).
 Flag-off 403 stays on dialog and does not clear password (only
 reauth_required does). Delete selected button is not disabled
 during drop in flight (handler refuses). Viewports were not
 inspected. Playwright was not run.
No secret artifacts in b4674fb..604a956. Dist not committed.
Keep PG-011 Partial. Keep AUTH-006 Partial (Redis DELETE plus
 PG-008 DELETE plus PG-009 POST plus this DELETE). Keep
 PG-008/009/010 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop UI review Approve Partial (2026-08-26)

```text
Requirement: PG-011 Partial UI + AUTH-006 Partial (this DELETE UI only).
 Keep PG-008 Partial. Keep PG-009 Partial. Keep PG-010 Partial.
 Do not mark Complete. Playwright is Complete-only and was not run.
Decision/ADR: freeze b4674fb (`b4674fb0d74a52a96dde5e27f15550f661de65b9`).
 AUTH-006 is in-handler owner_password on
 DELETE /api/v1/postgres/databases/{db}. No POST /api/v1/auth/reauth.
 Product choice BF-1: UI discloses Recovery requires a valid external
 backup / Cannot be undone; no checkbox-as-authorization.
Review: redgres-ui-reviewer Approve Partial on
 ee295fe (`ee295feade67f6c32bd0ef7dcb69b13482007434`) master.
 UI SHA 604a956 (`604a956c7171b8ad021054f64eb28210d4ab00e8` cherry-pick of
 `452a3951eb6c695981be439e80131a96fad2bfb2`).
 Go SHA 85372a6 (`85372a6df90099102864b304eba8d09daa622c3a` cherry-pick of
 `8fb1d72db4329c5de5df560c834dbdf4aa249fc1`).
 Diff range b4674fb..ee295fe.
 Confirmed freeze defects: none. Required UI changes: none.
 Optional polish (not Partial blockers; Truncate/Redis Delete family):
 useFocusTrap without restoreFocusRef; consequence copy not
 aria-describedby; Delete selected not disabled during drop in flight
 (handler refuses).
Files reviewed: web/src/api/postgres.ts;
 web/src/features/postgres/DropDatabaseDialog.tsx;
 web/src/features/postgres/DatabasesPage.tsx; web/src/App.test.tsx.
Unexecuted: Playwright viewports 360x800, 768x1024, 1280x800,
 1600x1000, 200% zoom; this reviewer did not re-run jsdom; live
 PostgreSQL 17/18; COMPATIBILITY.md §6; Node 24.19.0.
Keep PG-011 Partial. Keep AUTH-006 Partial (Redis DELETE plus PG-008
 DELETE plus PG-009 POST plus this DELETE). Keep PG-008/009/010
 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop security review Approve Partial (2026-08-26)

```text
Requirement: PG-011 Partial + AUTH-006 Partial (flagged DELETE
 /api/v1/postgres/databases/{db} plus inspector Drop dialog).
 Keep PG-008 Partial. Keep PG-009 Partial. Keep PG-010 Partial.
 Keep AUTH-006 Partial (Redis DELETE plus PG-008 row DELETE plus
 PG-009 truncate POST plus this DELETE). Do not mark Complete.
Decision/ADR: freeze b4674fb (`b4674fb0d74a52a96dde5e27f15550f661de65b9`).
 AUTH-006 in-handler LookupOwnerByUsername + Verify on body
 owner_password. No POST /api/v1/auth/reauth. Gate 4 N/A (vault
 DELETE is existence-row only; no decrypt). Product choice BF-1:
 HTTP does not check backups; OPS-004 remains Complete.
Review: redgres-security-reviewer Approve Partial on
 ee295fe (`ee295feade67f6c32bd0ef7dcb69b13482007434`) master.
 Go SHA 85372a6 (`85372a6df90099102864b304eba8d09daa622c3a`). UI SHA 604a956 (`604a956c7171b8ad021054f64eb28210d4ab00e8`). Diff range b4674fb..ee295fe.
 Confirmed defects: none (Critical/High/Medium). Required changes:
 none for this Partial.
 Low/non-blocking: flag-off 403 leaves owner_password in the dialog
 (reauth_required clears it; memory only); canary coverage is HTTP +
 audit, not slog (reviewer did not execute journald canary).
 Sibling session-only / no CSRF / swallowed DROP ROLE / HTTP 500
 str(e) / checkbox-as-authorization NOT copied.
 Unexecuted: live PostgreSQL 17/18, Playwright, gitleaks,
 govulncheck, go test / go test -race / npm (read-only; tests
 inspected not rerun).
Keep PG-011 Partial. Keep AUTH-006 Partial. Keep PG-008/009/010
 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop evidence PASS Partial (2026-08-26)

```text
Requirement: PG-011 Partial + AUTH-006 Partial (flagged DELETE
 /api/v1/postgres/databases/{db} plus inspector danger Drop dialog,
 title Drop database). Keep PG-011 Partial. Keep AUTH-006 Partial
 (Redis DELETE /api/v1/redis/users/{username} plus PG-008 row DELETE
 plus PG-009 truncate POST plus this DELETE). Keep PG-008 Partial.
 Keep PG-009 Partial. Keep PG-010 Partial. Reject Complete.
 Gate 4 N/A (vault DELETE is existence-row only; no decrypt).
 Playwright is Complete-only.
Decision/ADR: freeze b4674fb (b4674fb0d74a52a96dde5e27f15550f661de65b9).
 AUTH-006 in-handler LookupOwnerByUsername + Verify on body
 owner_password. No POST /api/v1/auth/reauth. Feature flag
 REDGRES_FEATURE_POSTGRES_DROP via envBool (unset=false). Product
 choice BF-1: HTTP does not check backups; UI discloses Recovery
 requires a valid external backup / Cannot be undone; no
 backup_confirmed; no checkbox-as-authorization. OPS-004 remains
 Complete.
Evidence review (2026-08-26) on claimed HEAD 040ae69
 (040ae69786a90228f625d64d1d4b35c837093f40) master. Diff range
 b4674fb..040ae69. Product tree ee295fe
 (ee295feade67f6c32bd0ef7dcb69b13482007434). Go SHA 85372a6
 (85372a6df90099102864b304eba8d09daa622c3a, cherry-pick of 8fb1d72).
 UI SHA 604a956 (604a956c7171b8ad021054f64eb28210d4ab00e8, cherry-pick
 of 452a395). UI review pin 040ae69 Approve Partial (reviewed ee295fe;
 no C/H/M; no required changes). Security review pin 040ae69 Approve
 Partial (reviewed ee295fe; no C/H/M; no required changes). This
 reviewer did not re-run go test ./... or npm test (verifier-owned)
 and did not execute git log this turn; SHA chain matches TRACEABILITY
 pins + independent UI/security transcripts.
Freeze criteria mapped to implementation and recorded tests:
 DELETE registered with GET {db} after suffix routes
 (internal/httpapi/server.go); handlePostgresDatabaseDrop flag-off
 403 before decode; terminateDatabaseSQL pid <> pg_backend_pid()
 then quoted DROP DATABASE no FORCE no IF EXISTS
 (internal/postgresadmin/drop.go); optional DROP ROLE + vault DELETE
 only when not OwnerDenied and OwnedDatabaseCount == 0; AUTH-006
 in-handler; inspector DropDatabaseDialog title Drop database.
 HTTP tests internal/httpapi/postgres_drop_routes_test.go; domain
 internal/postgresadmin/drop_test.go; config
 internal/config/drop_test.go; jsdom web/src/App.test.tsx.
 Recorded parent commands (TRACEABILITY API/UI pins, not re-run):
 gofmt empty; go test ./internal/config ./internal/postgresadmin
 ./internal/httpapi ok (config 1.448s; postgresadmin 1.173s;
 httpapi 36.429s); go test ./... ok after dist rebuild (httpapi
 49.409s; cmd/redgres 3.112s; postgresadmin 1.868s; config 2.313s);
 go vet; go build; npm test:run 353 passed (353) Node v25.3.0 not
 nvmrc 24.19.0; npm run build success.
 Security Lows (non-blocking): flag-off 403 leaves owner_password
 in the dialog (reauth_required clears it; memory only); canary is
 HTTP + audit, not slog.
 UI optional polish (non-blocking; Truncate/Redis Delete family):
 useFocusTrap without restoreFocusRef; consequence copy not
 aria-describedby; Delete selected not disabled during drop in
 flight (handler refuses).
 Sibling session-only / no CSRF / swallowed DROP ROLE / HTTP 500
 str(e) / checkbox-as-authorization NOT copied.
 Status rows AUTH-006 / PG-001..012 Partial; PG-008/009/010 stay
 Partial. Canonical API/SECURITY/CONFIGURATION/UX already frozen;
 this pin is TRACEABILITY-only. AGENTS.md current-truth names
 flagged DELETE drop (implementation, not Complete).
 No secret/runtime artifacts in inspected files. Dist not committed.
Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth, OPS-004
 backup freshness, gitleaks, govulncheck.
Keep PG-011 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-009 Partial. Keep PG-010 Partial. Do not mark Complete.
Not pushed.
```

## PG-011 drop verifier PASS Partial (2026-08-26)

```text
Requirement: PG-011 Partial + AUTH-006 Partial (flagged DELETE
 /api/v1/postgres/databases/{db} plus inspector danger Drop dialog,
 title Drop database). Keep PG-011 Partial. Keep AUTH-006 Partial
 (Redis DELETE /api/v1/redis/users/{username} plus PG-008 row DELETE
 plus PG-009 truncate POST plus this DELETE). Keep PG-008 Partial.
 Keep PG-009 Partial. Keep PG-010 Partial. Reject Complete.
 Gate 4 N/A (vault DELETE is existence-row only; no decrypt).
 Playwright is Complete-only.
Decision/ADR: freeze b4674fb (b4674fb0d74a52a96dde5e27f15550f661de65b9).
 AUTH-006 in-handler LookupOwnerByUsername + Verify on body
 owner_password. No POST /api/v1/auth/reauth. Feature flag
 REDGRES_FEATURE_POSTGRES_DROP via envBool (unset=false). Product
 choice BF-1: HTTP does not check backups; UI discloses Recovery
 requires a valid external backup / Cannot be undone; no
 backup_confirmed; no checkbox-as-authorization. OPS-004 remains
 Complete as SECURITY.md §6.7 policy (not an HTTP gate this Partial).
Verification (2026-08-26) on HEAD 98fe882
 (98fe8822d1bb8bc47c365a81de71befc9d42b507) master. Diff range
 b4674fb..98fe882. Product tree ee295fe
 (ee295feade67f6c32bd0ef7dcb69b13482007434). Go SHA 85372a6
 (85372a6df90099102864b304eba8d09daa622c3a, cherry-pick of 8fb1d72).
 UI SHA 604a956 (604a956c7171b8ad021054f64eb28210d4ab00e8, cherry-pick
 of 452a395). UI review pin 040ae69 Approve Partial (reviewed ee295fe;
 not viewport/Playwright). Security review pin 040ae69 Approve Partial
 (reviewed ee295fe; no C/H/M). Evidence pin 98fe882 PASS Partial /
 reject-Complete (tests not re-run by evidence reviewer). This
 verifier re-ran the Partial suite on 98fe882 (go1.27.0 windows/amd64;
 Node v25.3.0 not nvmrc 24.19.0):
 gofmt -l cmd internal migrations → empty
 go test -count=1 ./internal/config ./internal/postgresadmin
 ./internal/httpapi → ok (config 1.204s; postgresadmin 1.144s;
 httpapi 43.906s)
 go test -count=1 ./... → all ok without dist rebuild (embed emptyFS;
 httpapi 38.818s; cmd/redgres 2.519s; postgresadmin 1.581s;
 config 1.273s)
 go vet ./... → no findings
 go build -o NUL ./cmd/redgres → success
 npm --prefix web run test:run → Tests 353 passed (353), 68.96s
 npm --prefix web run build → tsc --noEmit && vite build success
 (wrote gitignored internal/web/dist/app/; not committed)
Freeze criteria mapped to implementation and tests that passed:
 DELETE registered with GET {db} after suffix routes
 (internal/httpapi/server.go); handlePostgresDatabaseDrop flag-off
 403 before decode; terminateDatabaseSQL pid <> pg_backend_pid()
 then quoted DROP DATABASE no FORCE no IF EXISTS
 (internal/postgresadmin/drop.go); optional DROP ROLE + vault DELETE
 only when not OwnerDenied and OwnedDatabaseCount == 0; AUTH-006
 in-handler; inspector DropDatabaseDialog title Drop database.
 HTTP tests internal/httpapi/postgres_drop_routes_test.go; domain
 internal/postgresadmin/drop_test.go; config
 internal/config/drop_test.go; jsdom web/src/App.test.tsx.
 Success audit includes dropped_role when the role was dropped
 (matches freeze/API; failure metadata is database only).
 Status rows AUTH-006 / PG-001..012 Partial; PG-008/009/010 stay
 Partial. Canonical API/SECURITY/CONFIGURATION/UX already frozen;
 this HEAD is TRACEABILITY-only after product tree ee295fe.
 No secret/runtime artifacts. Dist not committed.
Unexecuted Complete blockers: live PostgreSQL 17/18,
 COMPATIBILITY.md §6, Playwright viewports, go test -race, CI,
 Node 24.19.0, Gate 4 (N/A), POST /api/v1/auth/reauth, OPS-004
 backup freshness HTTP gate, gitleaks, govulncheck.
Keep PG-011 Partial. Keep AUTH-006 Partial. Keep PG-008 Partial.
 Keep PG-009 Partial. Keep PG-010 Partial. Do not mark Complete.
Not pushed.
```

## Local skippable live-test image pins (2026-08-26)

```text
Requirement: OPS-006 / NFR live-matrix seam — freeze first-cell
 artifacts only. Keep OPS-001..007 Planned. Keep PG-001..012 Partial.
 Keep REDIS-001..008 Partial. Do not mark COMPATIBILITY.md §6 Complete.
 Do not mark any TRACEABILITY row Complete. No live tests in this pin.
Decision/ADR: ADR-008 (matrix stays PostgreSQL 17/18 × Redis 8.2/8.8;
 Redis 8.8 remains local latest-tested default, not Hub latest).
 ADR-002 (production PgBouncer host-native). Product: first skippable
 cell is PostgreSQL 18 × Redis 8.8 plaintext loopback; omit PgBouncer;
 do not add Redis 8.10 to the matrix.
Evidence: redgres-compatibility-researcher (2026-08-26) on HEAD
 ca8f428, no docker pull, no tests. Official sources: PostgreSQL 18.6
 news + https://www.postgresql.org/docs/release/18.6/ ;
 Docker Official Image postgres; Redis 8.8 00-RELEASENOTES (8.8.2
 SECURITY 2026-08-17); Docker Official Image redis; pgbouncer.org
 downloads; Hub library/pgbouncer 404.
Pins frozen in docs/COMPATIBILITY.md §8 and linked from
 docs/TESTING.md merge-gate row:
 postgres:18.6 (reject latest and floating :18)
 redis:8.8.2 (reject latest — Hub grouped latest with 8.10.1 on
 2026-08-26 — and floating :8.8)
 PgBouncer omitted (no Docker Official Image).
 Official images do not start TLS by default; this cell cannot
 satisfy §6 TLS. Hub index digest snapshot recorded as non-authoritative
 until first successful docker pull RepoDigests plus runtime
 SHOW server_version / server_version_num and Redis INFO redis_version.
 Canonical COMPATIBILITY.md §2 no longer calls Redis 8.8 the current
 newest GA series.
 Live harness not implemented this pin. Docker Desktop daemon was
 not running on the parent Windows host (no pull).
Keep all status rows unchanged. Do not mark Complete.
Not pushed.
```

## OPS-001 dispatcher + CI live/Playwright harnesses (2026-08-26)

```text
Requirement: OPS-001 Partial (dispatcher + --dry-run stage print, no
 mutation) + OPS-006 Partial (fail-closed 17|18 × 8.2|8.8 selection;
 no UI; no live detection). NFR-011 Partial extra: skippable
 integration/ Open/Ping against disposable GHA service containers
 postgres:18.6@sha256:1957b2ff3137e4ef7f3bc813e74fff50b1e1ffddc85c8b9d6f14ade972be8687
 and redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3.
 NFR-012 / AUTH login Partial extra: Playwright Chromium login
 viewports 360×800, 768×1024, 1280×800, 1600×1000, 200% zoom, no
 credential persistence. Keep OPS-002..005/007 Planned. Keep PG/REDIS
 rows Partial. Reject Complete. Not COMPATIBILITY.md §6 (no PG 17 pin,
 no Redis 8.2 pin, no PgBouncer, no TLS, no backup/restore). Not
 production/DNS/Cloudflare.
Decision/ADR: ADR-002/008/009; INSTALLER_SPEC flags; COMPATIBILITY.md §8
 first-cell pins. Disposable GHA service containers are development
 evidence only.
Implementation files: deploy/install.sh, deploy/lib/common.sh,
 deploy/tests/run.sh, deploy/README.md, integration/*,
 web/playwright.config.ts, web/e2e/login.spec.ts, web/package.json
 (@playwright/test 1.62.1), web/vite.config.ts (exclude e2e from
 vitest), .github/workflows/ci.yml (installer, integration,
 playwright jobs), Makefile, CONTRIBUTING.md, docs/TESTING.md,
 docs/REPOSITORY_STRUCTURE.md (deploy/tests/), AGENTS.md current-truth.
Tests (parent):
 Git Bash deploy/tests/run.sh → 31 passed, 0 failed
 go test -count=1 ./integration → allow-list pass; live SKIP
  "live integration env not set"
 go test -count=1 ./... → all ok (httpapi 54.265s)
 npm --prefix web run test:run → 353 passed (e2e excluded from vitest)
 npm --prefix web run build → success
 npx playwright install chromium → FAILED locally (Chrome for Testing
 download timeout/ECONNRESET). CI playwright job is the browser path.
Known limitations: no host preflight/packages; no Ubuntu staging;
 Playwright browsers not installed on this Windows host; remaining
 matrix cells wait for PG 17 / Redis 8.2 official patch pins; no
 production secrets/DNS/Cloudflare.
Do not mark Complete.
Pushed `a4295d7` to origin/master (run 32897889759). Disposable CI:
 installer, integration, playwright, frontend, embedded-build,
 secret-scan, vulnerability, cross-compile passed; backend failed
 on `go test -race` (go-redis SetLogger vs leftover pool dial).
 Local follow-up after operator Playwright install: `npx playwright
 test` in web/ → 6 passed (14.5s); vite preview still proxies
 /api/v1 to 127.0.0.1:8790 (ECONNREFUSED; login page still renders).
```

## go-redis SetLogger race (2026-08-26)

```text
Requirement: NFR race CI for REDIS Open. Keep REDIS/OPS Partial.
 Do not mark Complete.
Decision: call redis.SetLogger once from redisadmin init, not Open.
Evidence: GHA run 32897889759 backend failed go test -race
 (SetLogger write vs pool.dialConn read). Local:
 go test -race -count=10 ./internal/redisadmin → ok 11.456s.
Implementation files: internal/redisadmin/adapter.go;
 docs/ARCHITECTURE.md (SetLogger at init).
Do not mark Complete.
```

## Disposable CI green after SetLogger fix (2026-08-26)

```text
Requirement: NFR CI jobs. Keep all TRACEABILITY rows Partial.
 Do not mark COMPATIBILITY.md §6 Complete. Not production.
Evidence: origin/master `47d481b` GHA run 32898588361 success.
 Jobs: installer, integration (postgres:18.6 + redis:8.8.2
 service containers), playwright, backend (including
 go test -race), frontend, embedded-build, cross-compile,
 secret-scan, vulnerability. Local `npx playwright test`
 (web/) 6 passed after operator Chromium install.
Do not mark Complete.
```

## Remaining skippable live-test pins PG 17 / Redis 8.2 (2026-08-26)

```text
Requirement: OPS-006 / NFR live-matrix seam — freeze remaining
 §8 cells only. Keep OPS Planned except OPS-001/006 Partial.
 Keep PG/REDIS Partial. Do not mark §6 Complete.
Evidence: redgres-compatibility-researcher (2026-08-26),
 no docker pull. postgres:17.11@sha256:0b657ff48d7f76a1e907f381b1693eb4f2bf54c1d2df4feb6743d7dc601768dd
 redis:8.2.9@sha256:7d1e4ce8b9395088377ab382d1f6cfdbd13b3690795198a0399ab8d683064d6d
 PgBouncer still omitted. Do not add Redis 8.10.
Implementation files: docs/COMPATIBILITY.md §8; docs/TESTING.md
 merge-gate note (GHA integration job still first-cell only).
Do not mark Complete.
```

## OPS-002 PATH host --version inventory (2026-08-26)

```text
Requirement: OPS-002 Partial (PATH host --version inventory on
 --non-interactive --dry-run only). OPS-006 Partial extra
 (unsupported/unparseable/expect-mismatch fail-closed before
 mutation). OPS-007 skip-only for fresh/disabled. Keep OPS-001
 dispatcher. Keep OPS-003..005/007 Planned. Reject Complete.
 Not COMPATIBILITY.md §6. Not SQL SHOW / Redis INFO /
 PgBouncer SHOW VERSION. Not backup/safety-gate. Not production.
Decision/ADR: ADR-008/009; INSTALLER_SPEC stage 2 Partial;
 COMPATIBILITY.md §3 installer PATH --version until a non-eval
 credential path exists; runtime remains SHOW/INFO/SHOW VERSION.
 §8 pins unchanged (postgres:17.11 / redis:8.2.9).
Source characterization: PostgreSQL docs --version/-V print and
 exit; Redis 8.2 server.c --version prints then exit(0) before
 conf load; PgBouncer usage -V/--version with no ini (parse
 shape only).
Implementation files: deploy/install.sh; deploy/lib/inventory.sh;
 deploy/tests/run.sh; deploy/tests/fixtures/*; deploy/README.md;
 docs/INSTALLER_SPEC.md (stage 2 Partial); docs/COMPATIBILITY.md
 §3 one paragraph only; AGENTS.md current-truth;
 docs/REPOSITORY_STRUCTURE.md (inventory.sh).
Unit tests: parent re-ran Git Bash deploy/tests/run.sh on
 integrated master `868b3a0` → 41 passed, 0 failed
 (PATH stubs; mutation STUB_NAMES unchanged; --config canary
 not sourced; no Docker/live services). Writer claimed the
 same on `a04b2e8`. After security Low (unquoted PgBouncer
 token split): glob-safe parse + unparseable pgbouncer fixture;
 parent re-ran Git Bash deploy/tests/run.sh → 42 passed, 0 failed.
Integration tests: none — PATH --version only; not §6.
Security tests: canary env/config not printed; --config not
 sourced/eval'd; no passwords/URLs-with-passwords in output.
 Independent redgres-security-reviewer Approve Partial on
 `aedcc8e` (no Critical/High/Medium; Low globbing corrected
 before verifier). Independent redgres-evidence-reviewer
 Approve Partial / reject Complete on `aedcc8e` (41 pass()
 sites then; 42 after Low fix). Independent redgres-verifier
 Approve Partial / reject Complete on `ddd0581`; this-run
 Git Bash deploy/tests/run.sh → 42 passed, 0 failed.
Deployment/migration impact: none deployed. Dry-run only.
Known limitations: PATH-only; binary ≠ running cluster; no
 SHOW/INFO; no backup; no cluster identity/listeners/datadir;
 missing /usr/lib/postgresql/N/bin not on PATH fails closed;
 PgBouncer recorded without a support allow-list.
Do not mark Complete.
Local commits: cherry-pick `868b3a0` of writer `a04b2e8`;
 pushed through `ddd0581`.
```

## OPS-003 verify skip matrix (2026-08-26)

```text
Requirement: OPS-003 Partial (fail-closed verify --non-interactive
 --dry-run --config PATH skip matrix). Keep OPS-001/002/006
 dispatcher + PATH --version inventory. Keep OPS-004/005/007
 Planned. Reject Complete. Not COMPATIBILITY.md §6. Not DNS.
 Not Cloudflare Tunnel/Access/routes. Not public TLS. Not live
 GET /api/v1/healthz or GET /api/v1/status. Not live sockets.
 Not cluster SHOW/INFO. Not backup (no named backup keys;
 OPS-004). Not production.
Decision/ADR: ADR-002; INSTALLER_SPEC stage 12 This Partial;
 SECURITY.md §7 health reveals no versions/hostnames/secrets;
 CONFIGURATION.md REDGRES_ADDRESS 127.0.0.1:8790; API.md
 healthz/status; DEPLOYMENT.md §3 bindings; BACKUP_RECOVERY.md
 §6 skipped (no /var/backups fail-closed).
Source characterization: none (no live probes this Partial).
Implementation files: deploy/install.sh; deploy/lib/verify.sh;
 deploy/tests/run.sh; deploy/README.md;
 docs/INSTALLER_SPEC.md (stage 12 This Partial only);
 docs/REPOSITORY_STRUCTURE.md (lib/verify.sh);
 AGENTS.md current-truth; docs/TESTING.md installer note.
Unit tests: parent re-ran Git Bash deploy/tests/run.sh on
 integrated master `e256ced` → 49 passed, 0 failed (PATH
 stubs; mutation STUB_NAMES unchanged including curl;
 --config canary not sourced/printed; verify with no flags → 1;
 verify without --dry-run → 2 and no inventory header;
 missing/non-file/directory --config → 1; --mode on verify → 1;
 backup still 2; install dry-run still inventories). Writer
 claimed the same on `853eb73`. No Docker/live services.
Integration tests: none — skip matrix only; curl not invoked;
 do not assert live 200 healthz.
Security tests: canary env/config not printed; --config not
 sourced; no secret dump; no curl/wget/cloudflared/certbot.
 Independent redgres-security-reviewer Approve Partial /
 reject Complete on `72ff9b4` (no Critical/High/Medium).
 Independent redgres-evidence-reviewer Approve Partial /
 reject Complete on `72ff9b4` (49 pass() sites match parent
 49/0 on `e256ced`). Independent redgres-verifier
 Approve Partial / reject Complete on `2b27935`; this-run
 Git Bash deploy/tests/run.sh → 49 passed, 0 failed.
Limitations: DNS/Cloudflare/public TLS remain skipped.
 result=partial is required so exit 0 is not Complete.
Do not mark Complete.
```

## OPS-005 update/rollback skip matrices (2026-08-26)

```text
Requirement: OPS-005 Partial (fail-closed update --non-interactive
 --dry-run --release PATH and rollback --non-interactive --dry-run
 --to VERSION skip matrices). Keep OPS-001/002/003/006 dispatcher +
 PATH --version inventory + verify skip matrix. Keep OPS-004 Planned
 (backup still exit 2). Reject Complete. Not COMPATIBILITY.md §6.
 Not extract/symlink/sqlite_migrate/systemd/health_gate. Not
 postgres packages on update. Rollback never reverses
 PostgreSQL/Redis/vault/credentials/DNS/schema. Not production.
Decision/ADR: ADR-002; INSTALLER_SPEC command interface + stage 9
 This Partial + Rollback limits; DEPLOYMENT.md §2–4 FHS/release
 model; OPERATIONS.md Release/update + Application rollback;
 CONFIGURATION.md has no backup/release checksum/digest keys
 (checksum skipped); SECURITY.md no secret print; invariant 8.
Source characterization: none (no extract, no /opt/redgres, no curl).
Implementation files: deploy/install.sh; deploy/lib/release.sh;
 deploy/tests/run.sh; deploy/README.md;
 docs/INSTALLER_SPEC.md (update/rollback command lines, stage 9,
 Rollback limits This Partial only);
 docs/REPOSITORY_STRUCTURE.md (lib/release.sh listed; target tree
 still names deploy/update.sh; this Partial did not create it);
 AGENTS.md current-truth; docs/TESTING.md installer note.
Unit tests: parent re-ran Git Bash deploy/tests/run.sh on
 integrated master `4a8fa4d` → 63 passed, 0 failed
 (PATH stubs; tar added to STUB_NAMES; canary --release unread/not
 sourced; update/rollback no flags → 1; without --dry-run → 2 and
 no inventory; missing/non-file/directory --release → 1; --config
 and --mode on update → 1; rollback --to rel-1 dry-run → 0 exact
 matrix including data_reversal, VERSION not printed; missing --to,
 --to /abs, --to .. → 1; backup still 2; verify matrix unchanged;
 install dry-run still inventories; postgres-plan / postgres-extensions
 still 2). Writer claimed the same on `f709fa9`. No Docker/live
 services. Did not call tar/ln/sha256sum/curl/gpg/pg_dump/pg_restore/
 redis-cli. Did not write /opt/redgres.
Integration tests: none — skip matrix only; curl not invoked;
 do not assert live 200 healthz or symlink switch.
Security tests: canary env/config not printed; --release not
 sourced; VERSION not printed; no secret dump.
 Independent redgres-security-reviewer Approve Partial /
 reject Complete on `8bbbdbe` (no Critical/High/Medium; Low
 test-net gaps only: stub ln/sha256sum/gpg, `--to .`, rollback
 unknown --config/--mode). Independent redgres-evidence-reviewer
 Approve Partial / reject Complete on `8bbbdbe` (63 pass()
 sites; required correction: TRACEABILITY must not claim the
 target tree dropped deploy/update.sh).
Limitations: result=partial is required so exit 0 is not Complete.
 Live update/rollback without --dry-run remain exit 2. OPS-004
 backup remains Planned (exit 2). bash argv cannot carry NUL.
Do not mark Complete.
```

