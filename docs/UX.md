# User experience and information architecture

## Navigation

```text
Overview
PostgreSQL
  ├── Databases
  ├── Create database
  └── Security overview
Redis
  ├── ACL users
  └── Permission presets
Audit
System
Documentation
```

PostgreSQL and Redis share a design language but never share ambiguous labels such as “user,” “database,” or “connection” without the product context.

## Overview

- Independent status cards for Redgres state, PostgreSQL direct, PgBouncer, Redis, tunnel/tool links, and backups.
- Recent security-relevant audit events.
- Quick actions for PostgreSQL database and Redis ACL user creation.
- Degraded components remain actionable; one failure does not blank the whole dashboard.

## PostgreSQL workflow

- Database ledger/list with owner, size, connections, saved credential state, security warnings.
- Inspector/detail panel for connection URLs, tables, rows, and safe actions.
- Creation wizard separates database name, new/existing project role, generated password behavior, and direct/pooled explanation.
- Destructive operations live in a distinct danger area and disclose source connection termination/role cleanup.

## Redis workflow

- ACL user ledger with enabled state, key prefix, preset, protected/imported status.
- Form defaults to safe preset and a project-specific prefix.
- Queue preset requires Lists, Streams, or Sorted Sets selection.
- Custom permissions show only allowed commands/categories and their effective expansion.
- Externally managed rules Redgres cannot model exactly are labeled and not silently rewritten.

## Credential ticket

- Opens only from successful create/rotate/reveal response.
- Clearly labels PostgreSQL revealable vs Redis one-time semantics.
- Separate copy controls for username/password/direct/pooled URL as applicable.
- Does not auto-copy, auto-download, include QR codes, or persist.
- On dismissal, overwrite/clear component state; clear on logout, route/target change, or session expiry.
- Warn that copied data remains in OS clipboard history outside Redgres control.

## Dangerous actions

- Explain exact target and impact in plain language.
- Require typing exact target and owner password where PRD requires.
- Confirmation button remains disabled until server-relevant fields are complete.
- Server revalidates every condition; UI state is not trusted.
- Long action shows operation ID/progress and supports safe refresh.

## Accessibility

- Semantic headings/landmarks, labels and descriptions.
- Focus moves into dialogs and returns to trigger; destructive confirmation errors are announced.
- All operations keyboard accessible; no hover-only information.
- Status includes icon/text, not color alone.
- Respect `prefers-reduced-motion`; avoid motion on credential/destructive surfaces.
- Tabular data supports responsive list/card alternatives without losing headers/context.
