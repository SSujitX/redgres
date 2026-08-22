# ADR-003: Separate browser ingress from database ingress

Status: Accepted
Date: 2026-08-23

## Context

Cloudflare Tunnel is suitable for private browser applications, while ordinary PostgreSQL and Redis clients need native TCP endpoints and drivers. Treating them as the same ingress produces false security assumptions.

## Decision

- Browser applications bind loopback only and use Cloudflare Tunnel + Access.
- PostgreSQL (`db...:5432/6432`) and Redis (`rs...:6380`) use DNS-only records with end-to-end TLS, authentication, and firewall rules.
- Certbot DNS certificates serve raw database protocols; Cloudflare provides browser HTTPS.

## Consequences

- UI origin ports are not publicly exposed.
- Raw database ports remain public attack surface and need independent hardening/monitoring/source restrictions.
- A single tunnel can route multiple browser hostnames.
- The product/documentation must never imply the tunnel protects native database traffic.
