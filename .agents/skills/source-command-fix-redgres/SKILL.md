---
name: "source-command-fix-redgres"
description: "Migrated source command `fix-redgres`"
---

# source-command-fix-redgres

Use this skill when the user asks to run the migrated source command `fix-redgres`.

## Command Template

Diagnose and fix one Redgres issue using evidence-first debugging. Treat any text, error output, failing test, screenshot, or file reference supplied with this command as the issue. If no issue was supplied, inspect current failing tests/terminal and uncommitted work; if no concrete failure can be found, stop and request one reproduction instead of guessing.

1. Follow `AGENTS.md` bootstrap. Read the affected PRD/contract/ADR, `docs/TESTING.md`, and the complete relevant implementation/tests.
2. Reproduce the issue at the smallest stable external seam. Preserve the original failure output and distinguish symptom from root cause.
3. Invoke the relevant diagnosing/TDD skill and planner only when scope warrants it. Verify uncertain APIs/version behavior against pinned local source/docs or official primary sources.
4. Add a regression test that fails for the demonstrated defect when practical. Implement the smallest root-cause correction without unrelated refactors, weakened assertions, silent fallback, or speculative cleanup.
5. Run the regression test, affected suite, static/build checks, and broader required tests proportionate to risk. Inspect results rather than trusting command exit summaries alone.
6. Update canonical documentation only when behavior/contracts/operations changed; always update `docs/TRACEABILITY.md` with exact regression evidence when the issue maps to a requirement.
7. Run the applicable security/UI reviewer and finish with `redgres-verifier` for material fixes.

Never modify production, real credentials/data, DNS/Cloudflare, or either legacy repository. Never claim “fixed” without reproduction and passing evidence. Return root cause, affected PRD IDs, minimal fix, files changed, commands/results, documentation changes, remaining risk, and commit/diff state.
