# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” Add code/test links as work lands. Empty evidence means incomplete.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | AUTH-001–005 unit/HTTP/CLI tests; AUTH-006 not started | Partial |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | `GET /api/v1/healthz` unit tests only; no `/status` or search | Partial |
| PG-001..012 | PRD, Source Systems, ADR-004 | `internal/postgresadmin` | PG-001/002 unit+HTTP+UI; PG-007 table-list API+UI + row-browse API+UI; no DELETE; PG-003–006/008–012 not started | Partial |
| REDIS-001..008 | PRD, Source Systems, ADR-006 | `internal/redisadmin` | TODO | Planned |
| OPS-001..007 | Deployment, Installer, PostgreSQL Provisioning, Backup, Compatibility, ADR-008/009 | `deploy/`, `internal/platform` | TODO | Planned |
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
 L4/L5 accepted residuals. Not viewport sign-off. Not COMPATIBILITY.md §6.
```
