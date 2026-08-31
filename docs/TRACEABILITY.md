# Requirements traceability matrix

Status only. Empty evidence means incomplete. Do not mark Complete from prose alone.

History: `git log -p -- docs/TRACEABILITY.md` (and `git show ee659b7:docs/TRACEABILITY.md`). Do not preload it.

New work replaces **one** current-slice block. Do not re-copy older slices.

| Requirement group | Design source | Planned implementation | Test evidence | Status |
|---|---|---|---|---|
| AUTH-001..006 | PRD, Security, ADR-005 | `internal/auth`, `internal/httpapi` | Unit/HTTP/CLI; AUTH-005 Partial (fail-closed lockout, loopback `CF-Connecting-IP`); AUTH-006 Partial (in-handler reauth on Redis delete + flagged PG row-delete/truncate/drop) | Partial |
| PLAT-001..004 | PRD, Architecture, UX, UI DS | `internal/platform`, `internal/audit`, `web/` | `healthz` + `status` Overview/System cards; Expert tools Open + Reveal (ADR-014); audit read + Overview compact page; search palette (Partial docs catalog) | Partial |
| PG-001..012 | PRD, Source Systems, ADR-004/010/011 | `postgresadmin`, `secrets`, `operations`, `backup` | List/inspect/create/rotate/reveal/duplicate(202)/security Partial with live matrix on 17.11/18.6 where noted; row-delete/truncate/drop flagged + AUTH-006; backup gate on DROP; Gate 4 outstanding | Partial |
| PG-011 artifact integrity | Architecture, API, Security, Backup, ADR-011 | `DropAfterValidation`, `VerifyPostgresDatabaseArtifact` | Jail identity/size/SHA-256 before DROP; secret-safe 503 on verify fail | Partial |
| REDIS-001..008 | PRD, Source Systems, ADR-006 | `internal/redisadmin` | Status/metrics; ACL create/presets/custom/enable-disable/rotate/delete Partial; live matrix on 8.8.2/8.2.9; no TLS/Playwright | Partial |
| OPS-001..007 | Deploy, Installer, Compat, ADR-005/008/009/011 | `deploy/`, `platform`, `backup`, `database` | Dry-run stages; live fresh install Partial (pins, Redis/PG health, expert tools, release tarball, finish box); update/rollback Partial; backup/extension-plan dry-run only; not §6 Complete | Partial |
| OPS-004 artifact integrity | Backup, Security, ADR-011 | `internal/backup`, DROP gate | Bounded manifest + artifact verify; CLI/off-host/retention missing | Partial |
| OPS-004 SQLite recovery | Backup, ADR-005 | `internal/database` | Online snapshot + in-memory restore verify; installer wiring missing | Partial |
| NFR-001..012 | PRD, Architecture, Testing, Compat | cross-cutting | Wave 0 local; SQLite snapshot Partial; release guard + skippable `./integration`; race/cross-compile CI-only | Partial |

## Current slice

### Uninstall repairs interrupted dpkg before package purge (2026-09-01)

OPS-001 Partial: full uninstall now runs `dpkg --configure -a` and `apt-get -f install -y` before PostgreSQL/Redis/PgBouncer/cloudflared purge and again before autoremove, then retries purge once if targeted packages remain. Finish text no longer claims “fully removed” when `dpkg-query` still lists those packages; it prints the leftover names and recovery commands. Cloudflare incomplete cleanup still continues the local purge (prior slice). Evidence: `deploy/tests/run.sh` asserts repair/list helpers and stubbed `dpkg --configure -a` / `apt-get -f install -y` ordering. Do not mark Complete: live VPS confirmation after an interrupted-dpkg uninstall remains outstanding.
