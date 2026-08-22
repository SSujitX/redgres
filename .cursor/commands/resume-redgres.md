Resume unfinished Redgres local work from repository evidence. Do not depend on the previous chat.

1. Load all always-applied rules and read `AGENTS.md`, `CONTEXT.md`, `docs/ROADMAP.md`, `docs/TRACEABILITY.md`, `docs/CURSOR_WORKFLOW.md`, then route only the task-relevant documents.
2. Inspect `git status`, branch/worktrees, recent commits, uncommitted diff, implementation/tests, generated artifacts, and available terminal/test evidence. Preserve every user/unrelated change.
3. Identify the exact in-progress PRD slice, its accepted plan/contracts, completed portions, failing or unrun checks, documentation state, and safest recovery point. Do not start a new slice until current partial work is either completed, safely reverted with explicit authorization, or reported blocked.
4. If the recovered work is substantial or its contract is unclear, invoke `redgres-planner` with a recovery context packet. Do not redesign already accepted architecture or broaden scope.
5. Continue implementation with focused tests. Use `redgres-implementer` only for newly separable, non-overlapping packets and isolated worktrees only from a committed baseline. Review all returned diffs before integration.
6. Update canonical documentation and `docs/TRACEABILITY.md` in the same change. Run required focused/full checks and the applicable UI/security/verifier agents before claiming completion.
7. After completing the recovered slice, continue with `/start-redgres` behavior only if the next slice is dependency-ready and no genuine decision/safety blocker exists.

Never invent state, discard/overwrite partial work, weaken tests, expose secrets, edit legacy repositories, push, or perform production/external/destructive actions. Report what was recovered, changed, tested, documented, committed, still uncertain, and the next safe action.
