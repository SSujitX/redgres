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

### Cloudflare uninstall connector cleanup (2026-08-31)

OPS-001 / NFR-006 Partial: full uninstall now records and quiesces the `cloudflared-redgres.service` connector plus its path watcher before destructive Cloudflare cleanup. It explicitly removes the stored tunnel's connector registrations before deleting the tunnel, matching Cloudflare's no-active-connections precondition. Access/DNS failure prevents connector and tunnel deletion; connector-cleanup failure prevents tunnel deletion. Confirmed tunnel removal suppresses later connector compensation. Other failures request nonblocking restart only for previously active units and report failed or unconfirmed restoration after a bounded check without replacing the original failure; local SQLite state, credentials, TLS evidence, and data remain preserved for retry. Access applications and DNS records remain idempotent through accepted 404 responses. Native Redis cleanup includes the separately installed `redis-tools` package. Evidence: the authorized Ubuntu host reproduced the original fail-closed state after Access/DNS deletion with a healthy tunnel and one connector; the focused regression changed from `redgres_uninstall_quiesce_cloudflare: command not found` to `uninstall_cloudflare_cleanup=pass`. The executable embedded-Python mock covers all-success, Access/DNS 403, connector 400, tunnel 400, and all-404 retry sequences without exposing its canary token; the shell mock covers stopped connector/path ordering, committed-tunnel compensation suppression, immediate restart failure, and never-ready bounded restoration. Git Bash `deploy/tests/run.sh` passed 214/214 with the expanded failure matrix; changed-shell syntax and `git diff --check` passed. The corrected immutable script then completed all eight destructive stages on the authorized Ubuntu host, confirmed Cloudflare cleanup, removed managed paths, units, containers, and listeners, and exposed one remaining installer-owned `redis-tools` package that this slice now covers. Do not mark Complete until the follow-up immutable script is reviewed, pushed, and live verification confirms the package is absent.
