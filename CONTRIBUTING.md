# Contributing to Redgres

Redgres is currently specification-first. Contributions must preserve the migration safety and security invariants in [AGENTS.md](AGENTS.md).

## Workflow

1. Open an issue describing the problem, affected requirement, risk, and proposed acceptance criteria.
2. Keep each pull request focused. Include tests and documentation with behavior changes.
3. Use Conventional Commit-style subjects (`feat:`, `fix:`, `docs:`, `test:`, `build:`, `chore:`).
4. Never include real infrastructure addresses beyond already documented public hostnames, credentials, certificate material, `.env` files, runtime databases, or backups.
5. Complete the pull-request checklist and provide test output.

## Local checks

`.github/workflows/ci.yml` is authoritative. The `Makefile` mirrors it for Unix contributors; this workstation does not have `make`. From the repository root:

```text
go mod verify
gofmt -l cmd internal migrations
go vet ./...
go test ./...
go build ./cmd/redgres
cd web
npm ci
npm run test:run
npm run build
```

`go test -race`, linux/amd64 and linux/arm64 cross-compiles, `govulncheck`, and gitleaks run in CI. Frontend release evidence is the CI job on Node 24.19.0; a local Node 25.x run is development feedback only.

## Definition of done

- The implementation maps to an accepted PRD requirement.
- Unit, integration, frontend, security, and build checks required by [docs/TESTING.md](docs/TESTING.md) pass.
- Secret-bearing responses and logs are specifically tested where relevant.
- Migration, backup, and rollback behavior are documented.
- Accessibility and keyboard behavior are tested for UI changes.
- [docs/TRACEABILITY.md](docs/TRACEABILITY.md) links the requirement to code and tests.

Changes to security boundaries, persistence, vault cryptography, deployment topology, domain naming, or destructive-operation rules require an ADR.
