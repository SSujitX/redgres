# User experience and information architecture

The visual tokens, responsive breakpoints, shell geometry, login composition, search behavior, and review viewports are normative in [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md). This document defines information architecture and workflow behavior.

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

On desktop this hierarchy uses the persistent left sidebar; tablet uses the compact icon rail; mobile uses an accessible navigation drawer. The sticky topbar contains current context, global search, help/health, and the authenticated owner menu at the top-right. Primary create actions remain in the relevant page header.

## Global search

- Opens from the topbar, `/`, or `Ctrl/Cmd+K`, with a full-screen mobile treatment.
- Groups results by PostgreSQL databases, Redis ACL users, navigation, and documentation; every result has explicit service/type context.
- PostgreSQL hits come from authenticated `GET /api/v1/search` (manageable database names only). Redis ACL user hits come from the same endpoint (non-protected usernames). Navigation and documentation stay on the client `filterNav`. Degraded redis or postgres groups use `not_configured` / `unavailable` like each other — never “not available yet” for implemented statuses, and never a fake empty match list.
- Selecting a PostgreSQL hit opens Databases and selects that name in memory. Selecting a Redis ACL user hit opens ACL users and inspects that username in memory. Neither writes `location.search` or browser storage.
- Is read-only discovery/navigation. It never directly executes destructive actions or exposes credentials, protected resources, secret values, or raw audit metadata.
- Supports keyboard traversal, screen-reader result counts, loading/degraded/no-results states, bounded results, cancellation, and safe focus restoration.
- Residual: URL deep links, a documentation corpus, and command-palette actions.

## Login and session entry

- The unauthenticated route has no application sidebar/topbar and reveals no service health.
- Wide layouts place a restrained login panel in the upper-right region beside the Redgres dual-service identity field; narrow layouts use one centered/full-width column.
- Login supports keyboard/password-manager use, password visibility, generic failure messages, lockout/retry guidance, and an optional organization-access-policy note.
- Successful login uses only a validated safe return route. Session expiry clears sensitive UI state before showing login.

## Overview

- Live independent status cards for Redgres state, PostgreSQL direct, PgBouncer, Redis, and tool links, loaded from authenticated `GET /api/v1/status` on mount and Refresh (no polling). Refresh also refetches authenticated `GET /api/v1/redis/status` with no query. One component failure does not blank the page; envelope failures on `/status` (session expired, network, malformed payload) show an alert and no cards. Failure of `/redis/status` alone keeps the `/status` cards and degrades only the Redis metrics area.
- Redis headline is a live Ping from `/status`: Reachable, Unavailable, or Not configured. Compact labeled metrics (version, uptime, clients, used/max memory, ops/s, DB size, latency) and distinct auth/permission/connectivity copy come from `/redis/status`. `not_configured` omits metric rows. `max_memory_bytes` of `0` displays as Unlimited. If Ping is Reachable but `/redis/status` is not ok, keep Reachable and show “Metrics unavailable” plus the typed reason — never fake zeros. Version is server-supplied text: `displayText()` plus bidi isolate. Development default (no Redis URL file) is **Not configured**. PgBouncer stays honest **Not connected** (`not_implemented`). Tool links stay not configured.
- Backups card, recent audit events, and quick actions for PostgreSQL database / Redis ACL user creation remain not this slice.

## PostgreSQL workflow

- Database ledger/list with owner, size, connections, saved credential state, security warnings.
- Inspector/detail panel for connection URLs, tables, rows, and safe actions.
- Selecting a table in the Databases inspector loads that table’s rows from the bounded rows API into a horizontally scrollable grid with a sticky header inside a bounded pane, Previous/Next pagination taken from the response `offset`, `limit`, and `total`, and an optional `q` search applied on submit (maximum 128 Unicode code points; empty `q` is omitted). The row grid is placed immediately after the selected table. A Back to tables control clears the selected table and row state. Below 1024px, other table names are hidden while a table is selected. An existing table with zero matching rows shows “No rows.” A missing or non-table target shows a not-found alert, never an empty healthy grid. PostgreSQL unavailability shows the API or “PostgreSQL is unavailable” alert, never “No rows.” Changing the selected database clears the selected table and all row state. Row values, `q`, and row payloads stay in memory only and are not written to `localStorage`, `sessionStorage`, or the location URL.
- Creation wizard separates database name, new/existing project role, generated password behavior, and direct/pooled explanation.
- Destructive operations live in a distinct danger area and disclose source connection termination/role cleanup.

## Redis workflow

- The ACL users page lists Redis ACL users and inspects one user’s modeled rules without secrets. The title is “ACL users” from navigation. Create ACL user is a page-header action (not the topbar) and is hidden when the list is `not_configured` or `unavailable`.
- Authenticated `GET /api/v1/redis/users` uses the session cookie, no query string, and no CSRF. Protected users are listed and inspectable. Ledger rows are buttons showing username (Protected badge), Enabled or Disabled, preset, key prefix, and Limited when `rule_fidelity` is `limited`.
- `state: not_configured` is not an empty healthy list. `state: unavailable` with `unreachable`, `auth_failed`, or `permission_denied` uses copy distinct from “No ACL users.” HTTP 200 `state: ok` with `users: []` is “No ACL users.” Truncation is disclosed. Envelope 401 uses “Your session has expired. Sign in again to continue.”
- Selecting a row loads `GET /api/v1/redis/users/{username}` with AbortController; a slower first selection is ignored. Below 1024px, sibling ledger rows hide while one user is inspected (Back to users restores the list) and focus moves into the inspector. The inspector shows commands, categories, optional queue kind, and limited-rule copy when Redgres cannot model the rules exactly. It does not rewrite, rotate, or delete. A 404 is a not-found alert, never an empty healthy inspector. Detail `unavailable` uses the same typed Redis copy as the list.
- Truncation is disclosed as an alert above the ledger. Painted identifiers use `displayText()` plus bidi isolate. The Redis service rail is wayfinding; healthy and limited states do not use danger red. Login never fetches or POSTs `/api/v1/redis/users`. List, inspector, and credential-ticket state stay in memory (no `localStorage` / `sessionStorage`) and clear on logout.
- Create uses username and key prefix only. The prefix is suggested from the username until the prefix field is edited. The permission preset is read-only Cache read/write. There is no command editor. The POST body is `{ username, key_pattern }` with CSRF and never includes a password. Search remains inspect-only.
- A successful create refreshes the list and opens a one-time Redis credential ticket. The new user is inspected only after the ticket is dismissed, so the password does not linger in inspect state. The ticket shows username, password, and URL copy only when `credential.urls.primary` is present; it does not auto-copy. Extra secret fields in the response are ignored.
- Residual: other presets, queue-kind selection, custom allow-list editing, enable/disable/rotate/delete, Overview quick-create.

## Audit history

- The authenticated owner reviews security-relevant audit events newest first, one server page at a time, with no total count and no filtering in this slice.
- Older, Newer, Newest, and Refresh use opaque server cursors only. The client never constructs, decodes, increments, or sorts cursors, and never sorts events by `id` or `created_at`. Refresh and Newest return to the first page.
- Correlate incidents using the full `request_id`. Timestamps are the stored UTC strings; they are not converted to local time.
- Source address is the address Redgres observed on the connection. Behind Cloudflare Tunnel this is the tunnel connector, not the browser’s public address. That disclosure stays visible in the page header; it is not hover-only.
- Empty actor, target, or source address values show an em dash with an accessible “Not recorded” name. Errors (session expired, unusable cursor, control-plane storage unavailable, or a network failure) take precedence over an empty log. An empty state appears only after HTTP 200 with `events: []`.
- Actor, action, target, outcome, request ID, source address, and timestamp are sanitized at render time: bidirectional and other format/control characters become U+FFFD, then the visible value is isolated LTR. The stored row is unchanged. Homoglyphs are not detected.
- Below 768px each event is a labeled label/value list. At 768px and above, a bounded table with a sticky header owns horizontal/vertical scroll so the viewport does not scroll sideways. Audit uses neutral ink, not a PostgreSQL or Redis service rail.

## Credential ticket

- Opens only from successful create/rotate/reveal response.
- Clearly labels PostgreSQL revealable vs Redis one-time semantics. Redis create uses “shown now” one-time copy; Redgres cannot show the password again.
- Separate copy controls for username/password/direct/pooled URL as applicable. Redis create shows URL copy only when `credential.urls.primary` is present.
- Does not auto-copy, auto-download, include QR codes, or persist. Do not write secrets to `localStorage` or `sessionStorage`.
- On dismissal, overwrite/clear component state; clear on logout, section change, inspect of another user, or session expiry.
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
- Core workflows work at 320 CSS px, 200% zoom, touch, keyboard, and pointer; only bounded data grids may scroll horizontally.
