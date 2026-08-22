# Requirements traceability matrix

This file prevents “documented” from being mistaken for “implemented.” Add code/test links as work lands. Empty evidence means incomplete.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | TODO | Planned |
| PLAT-001..004 | PRD, Architecture, UX, UI Design System | `internal/platform`, `internal/audit`, `web/` | TODO | Planned |
| PG-001..012 | PRD, Source Systems, ADR-004 | `internal/postgresadmin` | TODO | Planned |
| REDIS-001..008 | PRD, Source Systems, ADR-006 | `internal/redisadmin` | TODO | Planned |
| OPS-001..007 | Deployment, Installer, PostgreSQL Provisioning, Backup, Compatibility, ADR-008/009 | `deploy/`, `internal/platform` | TODO | Planned |
| NFR-001..012 | PRD, Architecture, Testing, Compatibility, UI Design System | cross-cutting | TODO | Planned |

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
