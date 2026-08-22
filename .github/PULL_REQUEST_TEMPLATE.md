## Change

Describe the outcome and affected PRD requirement IDs.

## Risk and compatibility

- Security/data/credential impact:
- Migration/rollback impact:
- Source-system behavior inspected:
- ADR added/updated (if required):

## Verification

- [ ] Go unit/HTTP tests
- [ ] Go race tests and vet
- [ ] Frontend tests/build
- [ ] UI viewport/zoom/keyboard review and `redgres-ui-reviewer` evidence (if applicable)
- [ ] Applicable PostgreSQL 17/18 matrix jobs, with detected full versions/artifacts
- [ ] Applicable Redis 8.2/8.10 matrix jobs, with detected full versions/image digests
- [ ] Version selection/detection and unsupported-version rejection (if applicable)
- [ ] New/changed dependency APIs and version-sensitive claims verified against pinned source or official primary documentation
- [ ] Dependencies/toolchains use latest stable compatible reviewed versions with exact pins; release/security notes and migration impact recorded
- [ ] No unrelated features/refactors, invented APIs/results, or unlicensed copied code
- [ ] Secret/log/audit checks
- [ ] Deployment/backup/rollback checks (if applicable)
- [ ] Documentation and `docs/TRACEABILITY.md` updated
- [ ] No runtime DB, WAL, binary, `.env`, certificate, token, backup, or real credential in diff

Paste concise reproducible evidence and state anything not tested.
