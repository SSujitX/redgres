# Project charter

## Mission

Redgres gives a small operator team one secure, understandable control plane for provisioning and administering PostgreSQL project databases and Redis project ACL users on a single server, while preserving expert tools and avoiding hidden infrastructure magic.

Tagline: **One secure control plane for PostgreSQL and Redis.**

## Users

- Primary: the infrastructure owner/operator who provisions credentials and handles incidents.
- Secondary: trusted developers who receive project-specific connection URLs but do not administer the platform.
- Future: multiple operators with scoped roles; not required for the first migration release.

## Product principles

1. Safe defaults beat convenience for destructive or credential-bearing actions.
2. Every security-sensitive action is attributable and auditable without storing its secret.
3. The browser never connects directly to PostgreSQL or Redis.
4. Redgres is a focused control plane; pgAdmin and RedisInsight remain separate expert/data explorers.
5. Simple operations on one VPS are more valuable than premature orchestration complexity.
6. Backward compatibility is proven through tests, especially for encrypted credentials.
7. Installation, update, verification, backup, and rollback are deterministic and documented.
8. “Deployed” means observed on the real host; “recoverable” means restored in a test environment.

## Scope for migration release

- One Redgres instance managing one PostgreSQL cluster and one Redis instance.
- One owner account with secure server-side sessions.
- PostgreSQL project database/role lifecycle and safe data inspection.
- Redis ACL user lifecycle with key-prefix and command isolation.
- Audit event viewer, system health, and links to protected expert tools.
- One-server deployment automation and runbooks.

## Explicit non-goals

- Multi-tenant public SaaS or untrusted customer self-service.
- High-availability cluster orchestration, failover, sharding, or Kubernetes.
- Arbitrary SQL execution or arbitrary Redis command execution.
- Arbitrary browser-based PostgreSQL package/extension management. The installer may safely adopt/install the approved capabilities in [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
- Redis key browsing/editing (use RedisInsight).
- Replacing pgAdmin’s advanced administration features.
- Billing, quotas, organizations, teams, or full RBAC in the first migration release.
- Automatically managing Cloudflare billing/account policy or guessing DNS zones and credentials.

## Success measures

- All source-system capabilities marked “must preserve” have passing parity tests.
- Existing PostgreSQL vault records decrypt in Go without mutation.
- Credential responses are one-time/no-store and absent from logs, audit, SQLite, and frontend persistence.
- Fresh selections and existing-service adoption pass the complete release-owned matrix in [COMPATIBILITY.md](COMPATIBILITY.md), including version/capability detection and exact artifact reporting.
- Existing PostgreSQL adoption proves preserve-by-default behavior; every claimed optional PostgreSQL capability has exact package, named-database, restart and restore evidence under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
- PostgreSQL, Redis, and Redgres state can be restored into an isolated test host from documented backups.
- Legacy apps can remain available during observation and can be restored by a binary/config rollback.
- A new operator can understand topology, deploy, verify, back up, and recover using only this repository and explicitly supplied secrets.

## Decision ownership

The repository maintainers approve PRD changes and ADRs. Operators approve live-host changes, destructive actions, DNS changes, credential rotations, and legacy retirement. Documentation never grants an agent permission to mutate production.
