Resume unfinished Redgres local work from repository evidence. Do not depend on the previous chat. This is the explicit recovery form of `/start-redgres`. The same recovery-first loop is always applied by `.cursor/rules/06-continuous-orchestration.mdc`.

1. Load all always-applied rules and read `AGENTS.md`, `CONTEXT.md`, `docs/ROADMAP.md`, `docs/TRACEABILITY.md`, `docs/CURSOR_WORKFLOW.md`, then route only the task-relevant documents.
2. Inspect `git status`, branch/worktrees, recent commits, uncommitted diff, implementation/tests, generated artifacts, and available terminal/test evidence. Preserve every user/unrelated change.
3. Identify the exact in-progress PRD slice, its accepted plan/contracts, completed portions, failing or unrun checks, documentation state, and safest recovery point. Do not start a new slice until current partial work is either completed, safely reverted with explicit authorization, or reported blocked.
4. If the recovered work is substantial or its contract is unclear, invoke `redgres-planner` with a recovery context packet. Do not redesign already accepted architecture or broaden scope.
5. Continue implementation with focused tests. Use `redgres-implementer` only for newly separable, non-overlapping packets and isolated worktrees only from a committed baseline. Review all returned diffs before integration.
6. Update canonical documentation and `docs/TRACEABILITY.md` in the same change. Run required focused/full checks and the applicable UI/security/verifier agents before claiming completion.
7. After completing the recovered slice, enter the `/start-redgres` continuation loop: create a focused local commit for reviewed green work, select the next dependency-ready slice, and continue without asking whether to proceed.

A completed slice, reviewer approval, local commit, or clean worktree is not a stopping condition. Stop only for a genuine undecided product/architecture choice, repeated failing gate with no safe correction, missing secret/access, destructive/external/production work, completion of the approved roadmap, or an unavoidable session/tool/context limit. Before a voluntary limit stop, leave a recoverable checkpoint and report the active PRD slice, last passing checks, unfinished work, and `/start-redgres` as the normal next command.

Never invent state, discard/overwrite partial work, weaken tests, expose secrets, edit legacy repositories, push, or perform production/external/destructive actions. Report what was recovered, changed, tested, documented, committed, still uncertain, and the next safe action.
