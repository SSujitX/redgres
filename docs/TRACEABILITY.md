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

### Cloudflare uninstall inventory fallback (2026-08-31)

OPS-001 / NFR-006 Partial: full uninstall no longer aborts solely because `/var/lib/redgres/redgres.db` is missing. When SQLite `domain_deployment` is absent, step 0 reconstructs a minimal Cloudflare inventory only from Redgres-owned local evidence: decode `account_id`/`tunnel_id` from `cloudflared-tunnel-token` (never print token/`s`/`TunnelSecret`), and exact hostnames from root TLS evidence (`issue.result` / `tls-lineage`)—not app-writable `redgres.env` alone. Tunnel DELETE from a decoded token also requires a root Redgres footprint (TLS marker or installer systemd unit). DNS/Access use stored IDs when present; otherwise exact hostname GET match after exact zone-name discovery; Access matches app `domain` only. Account-wide tunnel-name search remains forbidden. API-token-only or tunnel-token-without-footprint stays fail-closed (`no_state` / `insufficient_evidence`) and preserves local evidence. Tunnel-token+footprint cleanup may confirm tunnel deletion (`SCOPE:tunnel_only`) and still prints dashboard links for residual DNS/Access. Hostnames without resolvable zone/account block tunnel deletion. Evidence: `deploy/tests/uninstall_cloudflare_mock_test.py` covers prior SQLite success/failure/404 matrix plus fallback tunnel-only (with footprint), tunnel-without-footprint `no_state`, token-only `no_state`, root-hostname zone/Access/DNS exact-match success (non-matching Access app and name-only Access ignored), env-hosts-ignored tunnel-only, zone-miss `insufficient_evidence`, and OAuth access-token canary absent from output. `deploy/tests/uninstall_cloudflare_test.sh` and Git Bash `deploy/tests/run.sh` include the `insufficient_evidence` status matrix. Docs: `docs/INSTALLER_SPEC.md` uninstall bullet. Do not mark Complete: live Ubuntu/Cloudflare confirmation of the missing-SQLite fallback path remains outstanding.
