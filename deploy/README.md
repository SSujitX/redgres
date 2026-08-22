# Deployment automation placeholder

This directory will contain the implementation described in [../docs/INSTALLER_SPEC.md](../docs/INSTALLER_SPEC.md). No production installer exists yet.

Do not paste commands from the legacy A-to-Z runbooks into a single script without live inventory, idempotency, secret-handling, and rollback tests.

Planned entry points:

- `install.sh`
- `verify.sh`
- `backup.sh`
- `update.sh`
- `rollback.sh`

The public contract belongs in the installer specification, while supported service choices belong in [../docs/COMPATIBILITY.md](../docs/COMPATIBILITY.md). PostgreSQL/PgBouncer existing/fresh behavior and optional extension plans belong in [../docs/POSTGRESQL_PROVISIONING.md](../docs/POSTGRESQL_PROVISIONING.md). Scripts and tests must stay synchronized with all three and must never use floating database-service artifacts, arbitrary extension packages/SQL, or an unapproved PostgreSQL restart.
