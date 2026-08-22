# Deployment automation placeholder

This directory will contain the implementation described in [../docs/INSTALLER_SPEC.md](../docs/INSTALLER_SPEC.md). No production installer exists yet.

Do not paste commands from the legacy A-to-Z runbooks into a single script without live inventory, idempotency, secret-handling, and rollback tests.

Planned entry points:

- `install.sh`
- `verify.sh`
- `backup.sh`
- `update.sh`
- `rollback.sh`

The public contract belongs in the installer specification; scripts and tests must stay synchronized with it.
