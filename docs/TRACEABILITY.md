# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” Add code/test links as work lands. Empty evidence means incomplete.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | AUTH-001–005 unit/HTTP/CLI tests; AUTH-006 not started | Partial |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | `GET /api/v1/healthz`; authenticated `GET /api/v1/status` + Overview live cards (PLAT-001 Partial); PLAT-003 audit read API + history UI; PLAT-004 `GET /api/v1/search` + grouped palette (Partial: Redis ACL hits unimplemented) | Partial |
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
 UI review (2026-08-25) rejected two Highs (status ignored postgres hits;
 palette not a scroll container) plus Mediums (same-name refocus, stale hits,
 muted degraded copy). Parent remediated those in this tree; UI re-review pending.
 Verifier pending. Explicitly NOT viewport sign-off.
```
