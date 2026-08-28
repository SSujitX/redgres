# Production orchestration (multi-agent loop)

**Purpose:** Repeatable parent-agent playbook until OPS-009 Complete and (separately) whole-product production sign-off. Read with [AGENTS.md](../../AGENTS.md), [.cursor/rules/50-multi-agent-orchestration.mdc](../../.cursor/rules/50-multi-agent-orchestration.mdc), and [OPS-009-LIVE-ACCEPTANCE.md](OPS-009-LIVE-ACCEPTANCE.md).

---

## What agents can and cannot do

| Agents CAN | Agents CANNOT |
|---|---|
| Plan slices, write code/tests/docs | Push to GitHub unless you explicitly ask |
| Run local `go test`, vitest, `deploy/tests/run.sh` | Run live Cloudflare/certbot/UFW on your zone |
| Ruthless/security/verifier reviews on diffs | Invent live evidence or mark Complete without proof |
| Draft runbooks and evidence templates | Hold or log your API tokens |

**You** run gates G1–G5 on Ubuntu; **you** authorize push and production DNS.

---

## Integration master loop

```text
1. Read AGENTS.md + TRACEABILITY latest slice + baseline SHA
2. redgres-planner → context packets (max 3 concurrent writers)
3. Implement → focused gates (see TESTING.md)
4. ruthless-reviewer → fix until APPROVE (no Critical/High)
5. security-review (if auth/secrets/network touched)
6. redgres-verifier (before Complete claims)
7. Operator live gate (OPS-009 runbook)
8. TRACEABILITY + ACCEPTANCE_CHECKLIST update
9. Next slice
```

Stop when: roadmap slice done, blocker needs your decision, or live evidence missing (explicit — not silent skip).

---

## Phase map (whole product)

| Phase | Goal | Primary owner |
|---|---|---|
| **0** | Runbook, ADR-013, evidence scaffold | ✅ Done in repo |
| **1** | OPS-009 gates G1–G9 | You (live) + agents (fixes, Playwright) |
| **2** | Installer live (`install.sh` without `--dry-run`) | Agents + you on throwaway VM |
| **3** | Backup/DR (pg_dump, off-host, restore test) | Agents + you |
| **4** | Full `ci.yml` manual + COMPATIBILITY §6 | Agents + GitHub Actions |
| **5** | ACCEPTANCE_CHECKLIST sign-offs | You + reviewers |

OPS-009 Complete ≠ Redgres v1 production (installer, DR, canary still Partial).

---

## Packet template (give every subagent)

```markdown
## Objective
<one slice>

## Non-goals
<explicit>

## PRD / ADR
OPS-009, ADR-013, …

## Baseline commit
<SHA>

## Allowed files
<list>

## Forbidden
Production DNS, secrets, push, unrelated refactors

## Required tests
<exact commands>

## Handoff
Files changed, commands run, pass/fail, risks, integration order
```

---

## Suggested agent roster

| Role | When |
|---|---|
| `redgres-planner` | New slice or failed live gate needs decomposition |
| `redgres-implementer` / general implementer | Code in allowed files (max 3 parallel) |
| `ruthless-reviewer` | Every integrated diff before you push |
| `security-review` | OAuth, tokens, confirm-reachable, backup, installer mutation |
| `redgres-verifier` | Before TRACEABILITY Complete line |
| `redgres-compatibility-researcher` | Cloudflare scope/API pin disputes |

---

## Copy-paste parent prompt (start Phase 1)

```text
Read AGENTS.md, docs/agents/OPS-009-LIVE-ACCEPTANCE.md, docs/agents/PRODUCTION-ORCHESTRATION.md.
Baseline: <SHA>.
Operator will run Gate G1 on Ubuntu; report failures with request_id only.
Your job: keep codebase ready; if no failure report, implement Gate G6 Playwright domain wizard e2e only.
Loop: implement → go test / npm run test:e2e → ruthless-reviewer until APPROVE.
Do not push. Do not touch Cloudflare.
```

---

## Push checklist (when you say push)

- [ ] Working tree clean or only intended commits
- [ ] `go test ./...` and `npm run build` passed locally
- [ ] `bash deploy/tests/run.sh` or install.yml dry-run path green
- [ ] Ruthless APPROVE on integrated SHA
- [ ] No secrets in diff
- [ ] install.yml matches intended CI (install.sh dry-run vs run.sh — document in TESTING.md)

After push: trigger GitHub **install** workflow; on Ubuntu clone same SHA and start **G1**.
