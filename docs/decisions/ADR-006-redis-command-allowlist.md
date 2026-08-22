# ADR-006: Explicit Redis command allow-list

Status: Accepted
Date: 2026-08-23

## Context

Redis gains commands and command semantics over time. The legacy custom preset rejects a finite dangerous deny-list but accepts other arbitrary names, which can accidentally grant unsafe current/future capabilities.

## Decision

Every managed Redis user starts with `-@all`. Presets and custom permissions expand only to a versioned explicit allow-list. Unknown commands/categories fail closed. Command sets have real Redis integration tests and representative framework/workload tests.

## Consequences

- Safer upgrades and reviewable privilege grants.
- Some valid application commands require an intentional allow-list change and release.
- Redis version changes trigger command-set review.
- Externally managed category rules may be read-only until Redgres can interpret them exactly.
- There is no arbitrary command endpoint.
