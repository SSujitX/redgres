# Cursor development workflow

## Open the correct workspace

Open `Redgres.code-workspace` in Cursor. It exposes:

- Redgres as the only writable implementation repository.
- `database-app` and `redis-ui` as read-only legacy references.

Cursor automatically loads project skills from `.agents/skills/`, project rules from `.cursor/rules/*.mdc`, and custom subagents from `.cursor/agents/`.

Normal use is one command in Agent mode, for both clean starts and recovery:

```text
/start-redgres
```

The command is stored at `.cursor/commands/start-redgres.md`; do not copy the long orchestration prompt into every chat.

Human copy/paste commands for starting, resuming partial work, checking status, and fixing a specific issue are in [../CURSOR_CODING.md](../CURSOR_CODING.md).

## Automatic context routing

The always-applied core rule forces every agent to read `AGENTS.md`, `CONTEXT.md`, the docs index, and charter first. The always-applied continuous-orchestration rule (`.cursor/rules/06-continuous-orchestration.mdc`) then keeps the parent advancing dependency-ready slices after each local checkpoint, even when `/start-redgres` was not typed. File-scoped rules attach backend, frontend, deployment, or testing context. Intelligent rules are pulled when multi-agent work is requested.

This intentionally does not force every document into every context. Agents read the canonical document map and then only the documents relevant to their task.

Rules are durable instructions, not model memory. The parent reconstructs status from Git, tests, roadmap, traceability, and the local tracker. Skills are discovered automatically and loaded when relevant.

Subagents do **not** share the parent conversation. Each starts with a clean context window. The parent must send a complete context packet containing the requirement, accepted decisions, exact docs/source to inspect, owned files, interfaces, tests, restrictions, fixed commit, and handoff contract. This is more reliable than asking several agents to “use the same chat context.”

At every slice, the planner runs a parallelism gate. The parent freezes shared contracts and keeps shared route/config/dependency/migration/traceability files. A dynamic wave may contain at most ten total participants: one integration master, up to three isolated writers, and a rotating pool of compatibility research, security review, UI review, evidence review, and final verification agents. Ten is a ceiling, not a target; only dependency-ready non-duplicative packets run. Each agent reports its fixed input, branch/commit where applicable, files, tests, evidence, limitations, and integration order to the parent. If a slice uses fewer writers, the planner records the exact contract or file dependency that prevents safe concurrency.

There is one integration master. Domain agents may lead a bounded packet but cannot merge, alter parent-owned contracts, or approve their own work. Security and UI reviewers inspect a fixed integrated commit in parallel, corrections return to an owning writer, and the verifier evaluates the corrected commit. Missing Docker, live-service, browser, vulnerability-scan, staging, or production evidence remains an explicit blocker to the corresponding acceptance claim while unrelated work continues.

## Before parallel coding

1. Review all current uncommitted specification/skill/Cursor files.
2. Create the initial Git commit. Cursor worktrees require a committed baseline.
3. Complete `setup-matt-pocock-skills` before using its tracker-based workflows; this is not a blocker for sequential Wave 0 driven by PRD/roadmap/traceability.
4. Establish the public/private Git remote before using cloud agents.
5. Create one bounded implementation context packet/spec for the first vertical slice; a tracker ticket is required once the chosen tracker is configured.

## Correct implementation order

### Wave 0 — sequential foundation

One agent only:

- establish Go module path and source provenance;
- scaffold `cmd/redgres`, internal package boundaries, SQLite migration mechanism, React build/embed, Makefile, and CI skeleton;
- create compiling placeholder interfaces without copying runtime artifacts;
- pass build/test baseline.

Do not parallelize this wave because nearly every later task depends on its files and contracts.

### Wave 1 — up to three isolated editing worktrees

After Wave 0 is merged:

- Auth/control-state slice: `internal/auth`, `internal/database`, auth HTTP routes/tests.
- Redis parity slice: `internal/redisadmin` plus Redis integration tests; no shared HTTP wiring until contract handoff.
- Frontend shell slice: `web/` responsive sidebar/icon rail/drawer, topbar search/owner menu, login, auth/status shell, and shared tokens against agreed mock/API contracts; load `redgres-ui-design` and run `redgres-ui-reviewer` before handoff.

The parent owns shared API wiring, `main.go`, dependency files, migration numbering, and integration after branches return. Security, UI, compatibility, evidence, and verification agents join as read-only or test-only packets when their inputs are ready; no implementation agent approves its own work.

### Wave 2 and later

Port PostgreSQL read-only behavior, then prove vault compatibility, then add mutations in the exact order from `docs/MIGRATION.md`. PostgreSQL installer/capability work must additionally read `docs/POSTGRESQL_PROVISIONING.md` and ADR-009 and must not invent package, preload, restart or extension behavior. Security and verification agents review every security-sensitive slice independently.

## Checkpointed continuation loop

`/start-redgres` and the always-applied continuous-orchestration rule run the same recovery-first loop:

1. Reconstruct current state from Git, roadmap, traceability, tests, and configured tracker items.
2. Recover an unfinished PRD slice before selecting new work.
3. Plan, implement, review, test, document, and create a focused local commit for one bounded slice.
4. Treat the completed slice as a checkpoint and immediately repeat from state reconstruction.

The orchestrator does not ask whether to continue after a green slice. Before a voluntary stop caused by a normal session/tool/context limit, it records the active PRD slice, verified commits, passing and unrun checks, dirty-worktree state, and exact next action in its handoff. Durable state remains Git plus canonical roadmap/traceability evidence; no separate status ledger is created.

## Autonomous continuation boundaries

`/start-redgres` tells the parent to continue through dependency-ready local slices without asking routine questions already answered by accepted documentation. It may create reviewed local commits and isolated worktrees, but it never pushes or changes live infrastructure.

No coding agent can literally run forever or guarantee uninterrupted execution. Cursor sessions have context, time, plan, tool, and cost limits; agents may stop after completion or when blocked. Durable progress therefore lives in commits, roadmap/traceability evidence, and subagent handoffs rather than one chat.

The orchestrator stops for a genuinely new product/architecture choice, unresolved contract conflict, missing access/secret, repeated failing safety gate, destructive action, or external/production/DNS/Cloudflare operation. These stops protect the project; suppressing them would be unsafe automation.

If a session stops for a normal limit, open a new Agent chat and run `/start-redgres` again. The recovery gate resumes from repository evidence instead of requiring the old conversation. `/resume-redgres` is available when you want to explicitly emphasize recovery, but it is not required for normal continuation.

## Useful Cursor controls

- Normal start/resume: `/start-redgres`.
- Recover unfinished work: `/resume-redgres`.
- Read-only state report: `/status-redgres`.
- Root-cause bug fix: `/fix-redgres <issue or error>`.
- Explicit subagent: `/redgres-planner`, `/redgres-implementer`, `/redgres-security-reviewer`, `/redgres-ui-reviewer`, `/redgres-verifier`.
- Parallel local work: ask for subagents “in isolated worktrees.”
- Agents Window multitasking: `/multitask` when available.
- One isolated experiment: `/worktree <task>`.
- Compare alternative models/approaches: `/best-of-n ...` for a bounded problem, not whole-project implementation.

Parallel agents cost more context/tokens and can increase merge risk. Use them for independent bounded slices, independent research, and verification—not for tightly coupled foundation work.
