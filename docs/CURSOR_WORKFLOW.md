# Cursor development workflow

## Open the correct workspace

Open `Redgres.code-workspace` in Cursor. It exposes:

- Redgres as the only writable implementation repository.
- `database-app` and `redis-ui` as read-only legacy references.

Cursor automatically loads project skills from `.agents/skills/`, project rules from `.cursor/rules/*.mdc`, and custom subagents from `.cursor/agents/`.

## Automatic context routing

The always-applied core rule forces every agent to read `AGENTS.md`, `CONTEXT.md`, the docs index, and charter first. File-scoped rules then attach the appropriate backend, frontend, deployment, or testing context. Intelligent rules are pulled when multi-agent work is requested.

This intentionally does not force every document into every context. Agents read the canonical document map and then only the documents relevant to their task.

## Before parallel coding

1. Complete `setup-matt-pocock-skills` and select an issue tracker.
2. Review all current uncommitted specification/skill/Cursor files.
3. Create the initial Git commit. Cursor worktrees require a committed baseline.
4. Establish the public/private Git remote before using cloud agents.
5. Create one approved implementation spec/ticket for the first vertical slice.

## Correct implementation order

### Wave 0 — sequential foundation

One agent only:

- establish Go module path and source provenance;
- scaffold `cmd/redgres`, internal package boundaries, SQLite migration mechanism, React build/embed, Makefile, and CI skeleton;
- create compiling placeholder interfaces without copying runtime artifacts;
- pass build/test baseline.

Do not parallelize this wave because nearly every later task depends on its files and contracts.

### Wave 1 — up to three isolated worktrees

After Wave 0 is merged:

- Auth/control-state slice: `internal/auth`, `internal/database`, auth HTTP routes/tests.
- Redis parity slice: `internal/redisadmin` plus Redis integration tests; no shared HTTP wiring until contract handoff.
- Frontend shell slice: `web/` navigation/auth/status shell against agreed mock/API contracts.

The parent owns shared API wiring, `main.go`, dependency files, migration numbering, and integration after branches return.

### Wave 2 and later

Port PostgreSQL read-only behavior, then prove vault compatibility, then add mutations in the exact order from `docs/MIGRATION.md`. Security and verification agents review every security-sensitive slice independently.

## Cursor kickoff prompt

Paste this into Cursor Agent after opening the workspace and creating the initial baseline commit:

```text
Act as the lead Redgres orchestrator.

First read AGENTS.md, CONTEXT.md, docs/INDEX.md, docs/PROJECT_CHARTER.md,
docs/PRD.md, docs/ARCHITECTURE.md, docs/MIGRATION.md, docs/TESTING.md,
docs/TRACEABILITY.md, and docs/ROADMAP.md. Treat ../database-app and
../redis-ui as read-only references. Never copy secrets, runtime databases,
WAL files, binaries, .env files, or generated artifacts.

Do not start by building the entire product. Invoke the redgres-planner
subagent to plan Wave 0: the smallest compiling Redgres foundation described
in docs/CURSOR_WORKFLOW.md and M1 of docs/ROADMAP.md. Map every deliverable to
PRD requirements and define a test seam. Show me the plan, file ownership,
commands, risks, and completion gate before editing.

After I approve the Wave 0 plan, implement it with TDD where behavior exists.
Use one writer for Wave 0. Run focused tests continuously and the full available
suite at the end. Then run redgres-security-reviewer and redgres-verifier in
parallel. Do not push, modify production, or edit either legacy repository.
```

## Parallel-wave prompt

Use only after the foundation is committed and the planner proves file ownership does not overlap:

```text
Plan the next approved Redgres wave with redgres-planner. Create at most three
independent context packets. Run editing subagents concurrently in isolated Git
worktrees/branches; never share writable files, migrations, API schemas, or
dependency manifests. Each agent must read its routed rules and required docs,
implement only its assigned PRD IDs, run its focused tests, and commit only to
its own branch. The parent must review and integrate in declared dependency
order, run the complete suite, update docs/TRACEABILITY.md, then invoke
redgres-security-reviewer and redgres-verifier independently. Stop on contract
conflicts or failing gates; do not paper over them.
```

## Useful Cursor controls

- Explicit subagent: `/redgres-planner`, `/redgres-security-reviewer`, `/redgres-verifier`.
- Parallel local work: ask for subagents “in isolated worktrees.”
- Agents Window multitasking: `/multitask` when available.
- One isolated experiment: `/worktree <task>`.
- Compare alternative models/approaches: `/best-of-n ...` for a bounded problem, not whole-project implementation.

Parallel agents cost more context/tokens and can increase merge risk. Use them for independent bounded slices, independent research, and verification—not for tightly coupled foundation work.
