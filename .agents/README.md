# Repository-local agent skills

Parent-agent workflows the current chat can load. They are not Cursor subagents.

Spawnable Redgres roles live in [`.cursor/agents/`](../.cursor/agents/). Human slash commands live in [`.cursor/commands/`](../.cursor/commands/).

## Keep here

- `redgres-ui-design` — repository-owned React/UI workflow (not upstream).
- Upstream (pinned `5b15a47` from https://github.com/mattpocock/skills, MIT): `tdd`, `diagnosing-bugs`, `code-review`, `codebase-design`, `improve-codebase-architecture`, `grilling`, `grill-with-docs`, `domain-modeling`, `research`, `prototype`, `handoff`, `wizard`, `resolving-merge-conflicts`, `writing-for-agents`, `wait-what`.

Do not update from a floating `main`. Review an exact upstream commit, inspect the diff, and update [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

## Do not recreate

Tracker flows (`to-spec`, `to-tickets`, `triage`, `wayfinder`, `setup-matt-pocock-skills`, `ask-matt`) and `source-command-*` copies of `.cursor/commands`. Redgres planning/implementation uses `/start-redgres` plus `.cursor/agents/`.
