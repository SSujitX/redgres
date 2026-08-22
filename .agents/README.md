# Repository-local agent skills

These skills are vendored for agents working on Redgres. They are ordinary project files and should be reviewed and updated deliberately.

## Source and pin

- Upstream: https://github.com/mattpocock/skills
- Upstream commit: `5b15a47f2d7150f545fbcacbfe381787fc0230dc`
- Installed path: `.agents/skills/`
- Upstream license: MIT; see [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)
- Installed on: 2026-08-23

Do not update directly from a moving `main` branch. Review upstream changes, choose an exact commit, reinstall/update, inspect the diff, validate referenced skills/resources, and update the pin above.

## Installed upstream workflow skills

- Setup/routing: `setup-matt-pocock-skills`, `ask-matt`
- Requirements/planning: `grill-with-docs`, `grilling`, `domain-modeling`, `to-spec`, `to-tickets`, `wayfinder`, `research`
- Implementation quality: `tdd`, `diagnosing-bugs`, `code-review`, `codebase-design`, `improve-codebase-architecture`, `implement`
- Supporting work: `prototype`, `triage`, `resolving-merge-conflicts`, `wizard`, `writing-for-agents`, `handoff`
- Communication/decision support: `grill-me`, `to-questionnaire`, `wait-what`

The unrelated multi-session teaching skill was intentionally not vendored.

## Redgres project skills

- `redgres-ui-design` — repository-owned visual/responsive implementation and review workflow for the React application. It is not part of the pinned Matt Pocock upstream bundle.

## Required one-time setup

On the next agent turn, invoke `setup-matt-pocock-skills`. It will configure:

- where Redgres issues/tickets live;
- triage label names;
- how the existing glossary/ADR documentation is exposed to these skills.

Redgres currently has no Git remote. The recommended future choice is GitHub Issues after the public/private GitHub repository and remote exist. Until then, choose the local Markdown tracker if work must begin immediately.

Do not use `to-spec`, `to-tickets`, `triage`, `wayfinder`, or `code-review` as a complete workflow until `docs/agents/issue-tracker.md` and the domain-layout configuration have been created by setup.
