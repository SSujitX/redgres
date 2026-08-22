# ADR-007: Staged migration with legacy fallback

Status: Accepted
Date: 2026-08-23

## Context

The two current applications are functioning systems with distinct security and data behavior. A direct rewrite/cutover risks credential loss and destructive-operation regressions.

## Decision

Migrate in phases: stabilize/baseline, build platform+Redis parity, add PostgreSQL reads, prove vault compatibility, add PostgreSQL mutations, stage/shadow, cut over, observe, then retire. Legacy applications remain separately reachable through protected hostnames until retirement gates pass.

## Consequences

- More temporary services/hostnames and a coexistence port plan are required.
- Rollback is realistic because legacy behavior remains available.
- Duplicate production mutations are prohibited during shadowing.
- Retirement requires restore, parity, observation, and operator acceptance evidence.
