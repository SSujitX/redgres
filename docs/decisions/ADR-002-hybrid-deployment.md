# ADR-002: Hybrid host-native and container deployment

Status: Accepted
Date: 2026-08-23

## Context

PostgreSQL is an existing stateful host service with PgBouncer and mature host backup/config procedures. Redis 8 has a clean Docker deployment model. Redgres/cloudflared need direct systemd lifecycle and protected host credentials.

## Decision

- PostgreSQL 17 and PgBouncer remain host-native systemd services.
- Redis 8 and optional RedisInsight run with Docker Compose and explicit persistent mounts.
- Redgres and cloudflared run as systemd services.
- pgAdmin remains an optional separately managed loopback service.

## Consequences

- Avoids risky PostgreSQL data migration solely for uniformity.
- Keeps Redis lifecycle reproducible and isolated.
- Requires documentation/verification across systemd and Docker.
- One top-level installer orchestrates both without pretending all services share one runtime.
- Kubernetes and “everything in Docker” are not current supported production paths.
