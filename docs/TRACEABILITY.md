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

### Redis Insight origin reliability (2026-08-31)

OPS-001 / PLAT-002 / NFR-006 Partial: the full-stack installer now candidate-pins Python 3 before service mutation, then uses descriptor-pinned `O_DIRECTORY|O_NOFOLLOW` creation plus `fchown`/`fchmod` for the pinned expert-tool images' persistent data mounts. pgAdmin receives private `5050:5050` ownership and Redis Insight receives private `1000:1000` ownership without a root pathname race through the app-writable parent. This matches the immutable image runtimes and Redis's documented UID requirement. It prevents the prior `root:root 0755` Redis Insight mount from denying `/data/logs`, crash-looping the container, resetting the Redgres tool-gate upstream, and surfacing Cloudflare 502. Evidence: the installer regression was red before the ownership fix and green after it; Git Bash `bash deploy/tests/run.sh` passed (209 passed, 0 failed); `bash -n` and `git diff --check` passed; independent ruthless review returned SHIP with no remaining findings. Live Ubuntu 26.04 reproduced incrementing restart counts, failed direct health, `EACCES` on `/data/logs`, and an upstream reset at the reported timestamp. After the bounded ownership repair, 12 consecutive `GET /api/health/` probes returned 200 with no restart-count change; the no-follow helper independently passed ownership, mode, idempotence, and symlink-target preservation checks. The checksummed v1.1.8 release then upgraded the authorized host from v1.1.7 with Redgres, Docker, and cloudflared active; Redgres and Redis Insight returned 200 locally; Redis Insight stayed running with restart count 453 before and after a 40-second gate; post-upgrade `EACCES` and proxy/reset counts were zero; and repeated public console and Redis Insight requests reached the expected Cloudflare Access 302 boundary. Do not mark Complete because the broader OPS-001, PLAT-002, and NFR-006 groups retain unrelated partial acceptance work.
