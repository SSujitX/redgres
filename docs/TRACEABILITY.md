# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” Add code/test links as work lands. Empty evidence means incomplete.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | TODO | Planned |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | `GET /api/v1/healthz` unit tests only; no `/status` or search | Partial |
| PG-001..012 | PRD, Source Systems, ADR-004 | `internal/postgresadmin` | TODO | Planned |
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
