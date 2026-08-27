# Cursor coding quick start

This is the only day-to-day Cursor cheat sheet. Do not paste the complete Redgres documentation into chat.

## Open the project

Open `D:\code\github\Redgres\Redgres.code-workspace` in Cursor and use Agent mode. The workspace keeps Redgres writable and exposes `database-app` and `redis-ui` only as legacy references.

If project commands do not appear after pulling/creating them, reload the Cursor window and type `/` again.

## One command for normal development

Use this every time you open a new Cursor Agent chat, whether the repository is clean or has unfinished work:

```text
/start-redgres
```

It reconstructs state, recovers the current unfinished slice first when necessary, then chooses dependency-ready slices, plans, implements, verifies, updates documentation, creates reviewed local checkpoint commits, and continues through safe local work. You do not need the previous chat.

## Resume after stopping yesterday

This is an optional explicit recovery command when you want to emphasize that files, a branch, worktree, or tests are unfinished:

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
| New session, clean or unfinished; continue building the product | `/start-redgres` |
| You specifically want a recovery-only emphasis | `/resume-redgres` |
| You only want a report | `/status-redgres` |
| You know a bug/error/test failure | `/fix-redgres <issue>` |

Do not run multiple writing commands against the same checkout simultaneously. `/start-redgres` and `/resume-redgres` already coordinate safe parallel work using isolated worktrees when appropriate.

## What remains automatic

The compact always-on core routes implementation chats to `.cursor/rules/06-continuous-orchestration.mdc`, so status/explanation turns avoid paying for the implementation loop while coding still continues safe local slices without prompting. `/start-redgres` remains the explicit human entry point. Commands use `AGENTS.md`, Git, roadmap, traceability, relevant routed sections/skills, and complete subagent packets. Implementation changes synchronize canonical docs; independent reviewers reject unsupported claims, stale docs, invented APIs, missing tests, and unsafe scope expansion.

The commands never authorize pushing, production changes, real credential use, DNS/Cloudflare changes, destructive data operations, or edits to either legacy repository. Cursor may still stop for a genuinely undecided architecture/product choice, missing access, repeated safety failure, or normal session/tool/context limits. No prompt can make one Cursor session run forever. Progress survives through Git, roadmap/traceability evidence, and the visible working tree; after a normal limit stop, open a new Agent chat and run `/start-redgres` again.
