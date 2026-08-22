# ADR-008: Tested service-version matrix and operator selection

Status: Accepted
Date: 2026-08-23

## Context

The original Redgres specification fixed the deployment and test language to PostgreSQL 17 and generic Redis 8. A fresh installation should be able to use PostgreSQL 18, while existing PostgreSQL 17 deployments must not be forced through a major upgrade. Redis uses feature-bearing minor release series with different lifecycle profiles, so “Redis 8” is too broad to be a support promise. Conversely, claiming compatibility with any version or following an upstream `latest` tag would allow untested catalog, ACL, persistence, packaging, or upgrade behavior to enter production.

## Decision

- Each Redgres release owns an explicit, tested compatibility matrix documented in [../COMPATIBILITY.md](../COMPATIBILITY.md).
- The initial target matrix is PostgreSQL 17 and 18 crossed with Redis 8.2 and 8.8. PostgreSQL 18 is the fresh/local default. Redis 8.2 is the conservative production default and Redis 8.8 is the local latest-tested default.
- Fresh installation allows selection only from the release matrix. Existing mode detects the actual server versions and optionally compares them with operator-provided expected versions.
- Supported versions are release metadata/code, not an environment-variable allow-list that an operator can widen.
- Exact package versions and container digests are pinned in release metadata. `latest` and prerelease service versions are not production inputs.
- Version compatibility includes capability checks and integration evidence, not only parsing a version string.
- PostgreSQL major upgrades and Redis series upgrades are explicit, separately approved workflows; application install/update/rollback does not perform them implicitly.

## Consequences

- Operators can choose modern or compatibility versions without Redgres making an unlimited support claim.
- Existing PostgreSQL 17 installations can be adopted without a forced upgrade, while new installations can start on PostgreSQL 18.
- CI cost grows with the compatibility matrix; supported rows must remain deliberately small.
- Adding a version requires package, behavior, installer, backup, and restore evidence.
- Documentation and release manifests must distinguish target support from versions actually proven by a published release.

## Rejected alternatives

- **Support any PostgreSQL or Redis version:** impossible to test honestly and unsafe for administrative/destructive behavior.
- **Always install upstream latest:** changes behavior without a Redgres change and can select prerelease or unverified artifacts.
- **Keep one hard-coded service version forever:** blocks safe adoption of existing installations and unnecessarily delays supported stable upgrades.
- **Allow an environment variable to redefine supported versions:** turns an operator assertion into a false compatibility claim.
