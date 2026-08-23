# Source baseline

This file pins the exact source revisions used for Redgres parity work. Fill it immediately before implementation starts and update it only through review.

| System | Local path | Git remote | Commit | Working tree | Recorded at |
|---|---|---|---|---|---|
| Redgres | `D:\code\github\Redgres` | `https://github.com/SSujitX/redgres.git` | `4aea8e5` table-list API on `master` (`b2992df` inventory UI; `52930bd` inventory API) | AUTH-001–005, login/shell, PG-001/002, PG-007 table-list API; no rows/UI for tables; AUTH-006, vault, Redis, OPS not started | 2026-08-23 |
| PostgreSQL console | `D:\code\github\database-app` | `https://github.com/onelifeproject/database-app` | `1c3e8e2fe77345e6a40955a22a28b7bafe6fc4ad` | Clean when recorded | 2026-08-23 |
| Redis console | `D:\code\github\redis-ui` | No `.git` directory observed during initial review | Establish/import provenance | Runtime artifacts present; source baseline not yet immutable | 2026-08-23 |

Initial inspection notes (2026-08-23):

- `database-app` reported `master...origin/master` with no short-status changes.
- `redis-ui` was not a Git repository at the supplied path and contained `redact.exe`, `redact.db`, `redact.db-wal`, and `redact.db-shm`. These are runtime/build artifacts and must never be imported into Redgres.
- `Redgres` was empty and not a Git repository before this specification was created.

Provisional Redis anchor hashes from the inspected folder:

| File | SHA-256 |
|---|---|
| `go.mod` | `0E1E8E4D99651744BA148665B05AE271FF5E86439D9074BA92826BBDDB496FB7` |
| `go.sum` | `478C54A8E3B827AAE1CE8F02C88C2C59EFFACE29D3279BD5701B7A6B3950289B` |
| `cmd/redact/main.go` | `BC1423E0BDAB2D824C03142FC67BCF14145E08CCE9C382B49068E76C943E3C94` |
| `internal/httpapi/server.go` | `85ADDB1296F3D1A6A6815A9B0B142FB02AF96626F4776EF386A7A255DF9E3F45` |
| `internal/redisadmin/presets.go` | `592E75FB475A90DC79231DCE2EB7EEDFFC06651AD5E8210FDB50C5D6823C7929` |

These hashes help detect accidental drift in critical files, but they are not a substitute for importing the complete source with verified authorship, license, and Git history.

Wave 0 (2026-08-23) consulted `redis-ui` read-only for project layout, Makefile targets, embed/migration *approach*, and DSN pragma *shape*. No file was copied. SQLite URI parameters follow `modernc.org/sqlite` (`_pragma=busy_timeout(5000)`, `_pragma=foreign_keys(1)`, `_pragma=journal_mode(WAL)`). Redis source provenance remains unresolved and blocks copying into the Wave 1 Redis parity slice; it does not block Wave 0.

Do not claim reproducible source parity until the Redis source has provenance and both baselines are pinned.
