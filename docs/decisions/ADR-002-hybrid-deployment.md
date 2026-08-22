# ADR-002: Hybrid host-native and container deployment

Status: Accepted
Date: 2026-08-23

## Context

PostgreSQL may be an existing stateful host service with PgBouncer and mature host backup/config procedures or a fresh supported installation on a clean server. In either case, keeping PostgreSQL/PgBouncer host-native avoids an unnecessary data-layout/container migration. Supported Redis releases have a clean Docker deployment model. Redgres/cloudflared need direct systemd lifecycle and protected host credentials. Service-version selection is governed by [ADR-008](ADR-008-service-version-policy.md); PostgreSQL adoption/extensions/PgBouncer lifecycle is governed by [ADR-009](ADR-009-postgres-adoption-and-extensions.md).

## Decision

- The selected supported PostgreSQL release and PgBouncer remain host-native systemd services.
- The selected supported Redis release and optional RedisInsight run with Docker Compose and explicit persistent mounts.
- Redgres and cloudflared run as systemd services.
- pgAdmin remains an optional separately managed loopback service.

## Consequences

- Avoids risky PostgreSQL data migration solely for uniformity.
- Keeps Redis lifecycle reproducible and isolated.
- Requires documentation/verification across systemd and Docker.
- One top-level installer orchestrates both without pretending all services share one runtime.
- Kubernetes and “everything in Docker” are not current supported production paths.
