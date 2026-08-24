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
