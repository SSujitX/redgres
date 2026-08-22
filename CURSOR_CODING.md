# Cursor coding quick start

This is the only day-to-day Cursor cheat sheet. Do not paste the complete Redgres documentation into chat.

## Open the project

Open `D:\code\github\Redgres\Redgres.code-workspace` in Cursor and use Agent mode. The workspace keeps Redgres writable and exposes `database-app` and `redis-ui` only as legacy references.

If project commands do not appear after pulling/creating them, reload the Cursor window and type `/` again.

## Start or continue the roadmap

Use this on the first day or whenever no specific unfinished issue needs priority:

```text
/start-redgres
```

It reconstructs state, chooses the next dependency-ready slice, plans, implements, verifies, updates documentation, and continues through safe local work.

## Resume after stopping yesterday

Use this when files, a branch, worktree, or tests may be unfinished:

```text
/resume-redgres
```

It inspects Git and evidence first, recovers the current PRD slice, and finishes/verifies that work before selecting anything new. The previous Cursor chat is not required.

You may add a short constraint:

```text
/resume-redgres Finish and verify the current frontend shell slice before starting another slice.
```

## Check status without changing anything

```text
/status-redgres
```

Use this when you only want to know what is complete, partial, blocked, unverified, and what command should run next.

## Fix a specific bug or failing test

```text
/fix-redgres Redis ACL creation returns 500 when the username already exists. Reproduce it and fix the root cause.
```

Or paste an error after the command:

```text
/fix-redgres

Paste the failing command, error output, reproduction steps, screenshot, or affected route here.
```

The fixer reproduces first, adds a regression test where practical, makes the smallest correction, runs affected/broader checks, updates traceability and any changed contract, and verifies the result.

## Which command should I choose?

| Situation | Command |
|---|---|
| New session; continue building the planned product | `/start-redgres` |
| You stopped with unfinished changes | `/resume-redgres` |
| You only want a report | `/status-redgres` |
| You know a bug/error/test failure | `/fix-redgres <issue>` |

Do not run multiple writing commands against the same checkout simultaneously. `/start-redgres` and `/resume-redgres` already coordinate safe parallel work using isolated worktrees when appropriate.

## What remains automatic

Every command uses the persistent Cursor rules, `AGENTS.md`, Git, roadmap, traceability, relevant skills, and complete context packets for subagents. Implementation agents must update canonical documentation in the same change; independent reviewers reject unsupported claims, stale docs, invented APIs, missing tests, and unsafe scope expansion.

The commands never authorize pushing, production changes, real credential use, DNS/Cloudflare changes, destructive data operations, or edits to either legacy repository. Cursor may still stop for a genuinely undecided architecture/product choice, missing access, repeated safety failure, or normal session/tool/context limits. After a normal stop, run `/resume-redgres` again.
