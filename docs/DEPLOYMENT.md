# Deployment architecture

Status: target and provisional until verified against the live VPS.

## 1. Supported production profile

- Ubuntu Server 24.04 LTS.
- systemd for Redgres, PostgreSQL 17, PgBouncer, cloudflared, timers, and log lifecycle.
- Docker Engine + Compose plugin for Redis 8 and optional RedisInsight.
- PostgreSQL and PgBouncer host-native to preserve existing cluster integration and simplify direct backup/recovery.
- Cloudflare Tunnel + Access for browser consoles.
- Certbot DNS challenge for raw database TLS certificates.
- UFW plus provider firewall/security group where available.

Kubernetes is out of scope. A fully containerized alternative may be documented later, but production should have one supported path, not two half-tested paths.

## 2. Filesystem hierarchy

```text
/opt/redgres/
├── releases/<version>/          # immutable binary, web assets if external, release metadata
├── current -> releases/<version>
└── deploy/                      # versioned operational entry points

/etc/redgres/
├── redgres.env                  # non-secret configuration
└── secrets/                     # credentials/token files or systemd credential sources

/var/lib/redgres/
├── redgres.db                   # SQLite state
├── redis/                       # Redis persistent bind mounts (service ownership may differ)
└── operations/                  # bounded non-secret operation artifacts

/var/backups/redgres/
├── postgres/
├── redis/
├── sqlite/
└── manifests/
```

`journald` is primary logging. `/var/log/redgres` is optional only for tools that require files.

Recommended permissions, adjusted for the selected systemd credential mechanism:

| Path | Owner/mode | Notes |
|---|---|---|
| `/opt/redgres` | `root:root 0755` | Releases immutable to service user |
| `/etc/redgres` | `root:redgres 0750` | Directory traversal limited |
| `redgres.env` | `root:redgres 0640` | No secrets where avoidable |
| secret source files | `root:root 0600` | Inject via systemd credentials; otherwise narrowly `root:redgres 0640` |
| `/var/lib/redgres` | `redgres:redgres 0700` | SQLite/application state |
| `/var/backups/redgres` | `root:root 0700` | Backup job controls access |

Redis data ownership follows the container UID/GID and must be explicitly validated; do not recursively chown it to `redgres`.

## 3. Services and bindings

| Service | Manager | Bind/listener |
|---|---|---|
| Redgres | systemd | `127.0.0.1:8790` during migration |
| Legacy database app | existing systemd/container | `127.0.0.1:6969` |
| Legacy Redact | existing systemd | `127.0.0.1:8787` |
| PostgreSQL 17 | systemd | local + required external interface on 5432 |
| PgBouncer | systemd | local + required external interface on 6432 |
| Redis 8 plaintext | Docker | loopback/container network 6379 only |
| Redis 8 TLS | Docker/proxy design | external 6380 |
| RedisInsight | Docker | `127.0.0.1:5540` |
| pgAdmin | Apache/container | loopback selected port |
| cloudflared | systemd | outbound tunnel; no inbound listener required |

Ports are discovered and checked before installation. The installer fails on unexplained conflicts.

## 4. Release model

Each release directory contains:

- `redgres` binary;
- `VERSION` with semantic version, Git commit, build time, Go version;
- `SHA256SUMS` and optionally signature/provenance;
- migration files embedded or checksummed;
- operator release notes.

Deployment extracts to a new immutable release, verifies checksums, runs compatibility/preflight checks, applies forward-safe SQLite migrations, switches `current` atomically, restarts, and runs health verification. Keep at least two prior compatible application releases.

Rollback flips the symlink to a prior binary only if its declared schema compatibility includes the current SQLite schema. It does not modify PostgreSQL/Redis data, vault records, credentials, or DNS.

## 5. Cloudflare

- One remotely managed tunnel may publish all browser hostnames.
- Store the tunnel token as a bearer secret, not in shell history, unit command text, repository, or general environment dumps.
- Cloudflare Access policies should use least privilege, MFA/identity requirements, short sessions, and deny-by-default.
- Route changes require origin health checks before and external Access tests after.
- Direct/raw database DNS records are grey-cloud/DNS-only in this architecture.

## 6. TLS

- PostgreSQL and Redis public names receive certificates through DNS-01.
- Services use full chain and private key with least-readable ownership.
- PostgreSQL remote clients should prefer hostname verification (`verify-full`) with a trusted CA; `require` encrypts but does not necessarily verify identity in all client configurations.
- Renewal hook validates certificate dates/names, copies atomically, validates service config, reloads service, and verifies a real TLS connection.
- Alert well before expiry.

## 7. Firewall

Default deny inbound. Permit only expected listeners. Where application egress addresses are stable, create narrow source rules for 5432/6432/6380. SSH is source-limited when feasible. UI ports are neither published nor allowed.

Verification checks both firewall policy and actual socket bindings; a firewall rule alone does not prove a service is private.

## 8. Host resource guidance

Redis UI is not the deciding resource cost; PostgreSQL, Redis data, backup jobs, and cache policy are. Establish sizing from live inventory. As a conservative starting point for both databases and admin tools on one host, use at least 4 vCPU, 8 GB RAM, adequate SSD/NVMe, and separate/off-host backup capacity; production data size/workload may require much more. Configure Redis `maxmemory` and eviction policy intentionally. Avoid memory overcommit between PostgreSQL, Redis, Docker, and backup compression.

## 9. Pre-production evidence

- Complete hardware/disk/RAM/service/DNS/port inventory.
- Existing PostgreSQL cluster/version/data/config backup and restore rehearsal.
- Redis persistence mode, volume, ACL file, memory policy, and TLS inventory.
- Certificate names/renewal and Cloudflare routes/Access policies.
- Real application source IPs and connection pooling requirements.
- Conflict-free loopback ports.
- Tested installer in both modes on disposable hosts.
