# ADR-001: Go modular monolith with embedded React

Status: Accepted
Date: 2026-08-23

## Context

The PostgreSQL console is Python/FastAPI/Jinja; the Redis console is Go/Chi/SQLite with an embedded React frontend. One-server operation benefits from one deployable control-plane process, while PostgreSQL and Redis concerns still need strong code boundaries.

## Decision

Build Redgres as one Go modular monolith with an embedded React/TypeScript application. Start from reviewed Redis-console patterns for auth/session/audit/build, then port PostgreSQL behavior into a distinct `postgresadmin` module. Node.js is build-time only.

## Consequences

- One binary/unit and one owner session simplify deployment and UX.
- Go’s static binary and existing Redis implementation reduce migration risk.
- One process increases blast radius; domain boundaries, capabilities, timeouts, and partial-health behavior are mandatory.
- Python behavior is ported and characterized, not mechanically translated.
- Microservices/Kubernetes are rejected for v1 because they add operational complexity without solving the current one-host problem.
