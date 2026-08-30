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

### Domain TLS issuance recovery (2026-08-31)

OPS-005 / OPS-008 / OPS-009 / NFR-006 Partial: the root TLS helper now claims bounded requests, snapshots DNS credentials root-only, reuses only exact DNS-SAN lineages, persists host-bound cooldowns, installs PostgreSQL/PgBouncer certificate state transactionally, verifies served leaf fingerprints, and emits only allow-listed result/log data. Domain apply is durable before queueing; activity, retry cooldown, and disconnect recovery are explicit; Redis is truthfully `certificate_prepared`. Update/rollback/uninstall preserve or remove trusted TLS evidence fail-closed. The responsive Domain UI shows safe recent activity, retry time, endpoint-specific state, and pending-disconnect recovery. Evidence: `go test ./... -count=1` (all packages passed); `cd web && npm run test:run && npm run build` (473 tests, build passed); Git Bash `bash deploy/tests/run.sh` (208 passed); `bash -n` and `git diff --check` passed; independent security, UI, and ruthless reviews were clean. Live Ubuntu 26.04: Redgres, TLS path watcher, and cloudflared active; loopback health `ok`; Cloudflare Access returned its login redirect; root result is `0644`, credential/targets are `0600`, and journals contain no raw Certbot failure. An isolated Let's Encrypt staging DNS-01 issuance for both raw hostnames succeeded without mutating service configuration. Production issuance is still externally rate-limited until `2026-08-31T10:43:35Z`; the served PostgreSQL/PgBouncer leaf remains the temporary `CN=test`, Redis TLS application is not implemented, and public database listeners/firewall remain intentionally closed under the current installer Partial. Production leaf verification after cooldown, Redis TLS, public listener/firewall work, destructive disconnect/uninstall rehearsal, and Compatibility section 6 remain outstanding. Do not mark Complete.
