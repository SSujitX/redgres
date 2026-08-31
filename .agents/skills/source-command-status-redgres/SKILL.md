---
name: "source-command-status-redgres"
description: "Migrated source command `status-redgres`"
---

# source-command-status-redgres

Use this skill when the user asks to run the migrated source command `status-redgres`.

## Command Template

Report the current Redgres development state without editing files, creating commits, or starting implementation.

Follow `AGENTS.md` bootstrap. Read `docs/ROADMAP.md`, `docs/TRACEABILITY.md`, and only other documents needed to interpret current state. Inspect `git status`, current branch/worktrees, recent commits, uncommitted diff, implementation directories, and available test evidence.

Distinguish:

- implemented and verified;
- implemented but not fully verified;
- partially implemented/uncommitted;
- specification-only;
- blocked and the exact blocker;
- next dependency-ready slice.

Do not infer completion from documentation or test files existing. Do not run destructive/live commands, expose secrets, modify either legacy repository, or claim tests that were not executed. Return a concise status with current branch/diff, active PRD IDs if discoverable, last trustworthy evidence, unfinished work, risks, and the exact recommended next command (`/resume-redgres`, `/fix-redgres ...`, or `/start-redgres`).
