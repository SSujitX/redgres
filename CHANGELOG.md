# Changelog

All notable changes to Redgres will be documented here. The project intends to follow Semantic Versioning after its first public release.

## Unreleased

### Added

- Wave 0 compiling foundation: `github.com/SSujitX/redgres` on Go 1.27.0, Chi v5.3.2, `modernc.org/sqlite` v1.57.0, fail-closed Core configuration, checksummed SQLite migrations, `/api/v1/healthz`, embedded React 19.2.8 boot document, and a SHA-pinned CI skeleton.
- Owner auth (AUTH-001–005, login/logout audit): CLI `create-owner`, Argon2id PHC hashes (`golang.org/x/crypto` v0.55.0), hashed sessions/CSRF, origin+CSRF mutations, username+IP lockout, and `/api/v1` login/logout/session.
- Initial product, architecture, security, migration, deployment, backup, testing, operations, and open-source specifications.
- Agent instructions, architecture decision records, contribution templates, and requirements traceability skeleton.

### Important

- PostgreSQL, Redis, installer, and the browser login shell are not implemented. Redgres is not production-supported.
