# ADR-009: Separate PostgreSQL adoption, extension enablement, and PgBouncer lifecycle

Status: Accepted
Date: 2026-08-23

## Context

Redgres must work both when PostgreSQL/PgBouncer already exist on the server and when an operator wants a fresh installation. PostgreSQL capabilities also have different scopes: host packages make extension files available, `CREATE EXTENSION` enables SQL objects in one database, preload libraries alter cluster startup, and PgBouncer is a separate service. Treating them as one “install all” switch would cause unapproved restarts, package upgrades, schema mutations, excessive resource use, or damage to an existing cluster.

## Decision

- PostgreSQL supports explicit `existing` and `fresh` modes. PgBouncer independently supports `existing`, `fresh`, and development-only `disabled` modes.
- Existing mode defaults to inventory-and-preserve. It never initializes, upgrades, restarts, installs packages, edits preload configuration, or runs extension DDL without an approved desired-state plan.
- Fresh mode installs only a release-supported PostgreSQL major and explicitly selected extension capabilities. The default optional-extension set is empty.
- Extension selection uses canonical capability IDs and explicitly named databases. Release metadata derives exact packages, SQL names, preload libraries, companion components and verification checks.
- A required PostgreSQL restart is planned and separately approved. PgBouncer is drained/paused and verified around the restart.
- Redgres never enables optional extensions in `template1`, never enables every extension in every database, and never upgrades/drops extensions as a side effect of application install/update/rollback.
- PostgreSQL administrative and extension operations always use the direct connection, not PgBouncer.
- The complete operational contract and initial registry live in [../POSTGRESQL_PROVISIONING.md](../POSTGRESQL_PROVISIONING.md).

## Consequences

- Existing production clusters remain the safest adoption path and can be connected without mutation.
- Fresh installations can offer pgvector, PostGIS, TimescaleDB and other capabilities without bloating every deployment.
- Some selections require packages, additional repositories, memory/worker review, PostgreSQL restart, per-database DDL and expanded restore testing.
- The release matrix grows by PostgreSQL major and capability; Redgres must not claim support where exact packages or tests are unavailable.
- Ongoing arbitrary extension management in the browser remains out of scope for the first release. The versioned installer/CLI owns approved capability changes.

## Rejected alternatives

- **Install and enable all listed extensions:** unnecessary attack surface/resource use and unsafe restarts/schema mutations.
- **Treat package installation as extension enablement:** false because SQL extensions are database-local.
- **Put extensions in `template1`:** silently changes every future database and complicates restore/ownership policy.
- **Send extension DDL through PgBouncer:** administration depends on direct session/cluster semantics and must bypass pooling.
- **Automatically tune existing clusters:** workload and capacity decisions cannot be inferred safely.
