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
- Successful login uses only a validated safe return route. Session expiry clears sensitive UI state before showing login. POST `/api/v1/auth/login` has no `tool_links`. After login 200, App GET `/api/v1/session` and uses that CSRF plus `tool_links` before rendering the shell. Overview never GET `/session` (that rotation would desync App CSRF). Login never fetches `/status` or `/healthz`.

## Overview

- Live independent status cards for Redgres state, PostgreSQL direct, PgBouncer, Redis, and tool links, loaded from authenticated `GET /api/v1/status` on mount and Refresh (no polling). Refresh also refetches authenticated `GET /api/v1/redis/status` with no query. One component failure does not blank the page; envelope failures on `/status` (session expired, network, malformed payload) show an alert and no cards. Failure of `/redis/status` alone keeps the `/status` cards and degrades only the Redis metrics area.
- Redis headline is a live Ping from `/status`: Reachable, Unavailable, or Not configured. Compact labeled metrics (version, uptime, clients, used/max memory, ops/s, DB size, latency) and distinct auth/permission/connectivity copy come from `/redis/status`. `not_configured` omits metric rows. `max_memory_bytes` of `0` displays as Unlimited. If Ping is Reachable but `/redis/status` is not ok, keep Reachable and show “Metrics unavailable” plus the typed reason — never fake zeros. Version is server-supplied text: `displayText()` plus bidi isolate. Development default (no Redis URL file) is **Not configured**. PgBouncer headline is a live Ping from `/status` with the same Reachable / Unavailable / **Not configured** states (no metrics). Development default (no `REDGRES_POSTGRES_POOLED_PORT`) is **Not configured**, not “Not connected”. Tool links headline is config presence from `/status`, not a live probe: both optional URLs empty → **Not configured**; one or both set → **Reachable**. When GET `/session` includes `tool_links.pgadmin` and/or `tool_links.redisinsight`, the card renders those as `pgAdmin` and/or `RedisInsight` `<a href>` with `target="_blank"` and `rel="noopener noreferrer"`. Missing or `{}` `tool_links` paints no anchors. No iframe or embed. Refresh does not refetch session (the URLs are process-static). Login never shows tool links.
- Backups card, recent audit events, and quick actions for PostgreSQL database / Redis ACL user creation remain not this slice.

## PostgreSQL workflow

- Database ledger/list with owner, size, connections, saved credential state, security warnings.
- Inspector/detail panel for connection URLs, tables, rows, and saved-credential existence: **Saved**, **Not saved**, or **Not available**. Reason strings (`vault_unavailable`, `vault_not_implemented`, `ok`, `present`, `missing`) are never rendered. Passwords are not revealed. Reveal, rotate, and create controls are not offered. Selecting a manageable database loads authenticated `GET /api/v1/postgres/databases/{db}/connection` (session cookie, no CSRF) with the same abort-on-selection-change as details. Labels **Direct URL** / **Pooled URL** appear only when `masked_direct_url` / `masked_pooled_url` are present; copy uses the existing `text-button` ticket pattern (no auto-copy, no toast of the URL). Absent keys render no URL rows and do not invent “not configured”, `YOUR_PASSWORD`, or a standalone `********`. Loading copy is “Loading connection.” HTTP 503 is “PostgreSQL is unavailable.” HTTP 401 is session-expired and paints no URLs. URLs use `displayText()` plus bidi isolate. Connection state stays in memory only (no `localStorage` / `sessionStorage` / location URL) and clears on selection change, Back, and logout. Login never GETs `/connection`. POST reveal is not this slice.
- Selecting a table in the Databases inspector loads that table’s rows from the bounded rows API into a horizontally scrollable grid with a sticky header inside a bounded pane, Previous/Next pagination taken from the response `offset`, `limit`, and `total`, and an optional `q` search applied on submit (maximum 128 Unicode code points; empty `q` is omitted). The row grid is placed immediately after the selected table. A Back to tables control clears the selected table and row state. Below 1024px, other table names are hidden while a table is selected. An existing table with zero matching rows shows “No rows.” A missing or non-table target shows a not-found alert, never an empty healthy grid. PostgreSQL unavailability shows the API or “PostgreSQL is unavailable” alert, never “No rows.” Changing the selected database clears the selected table and all row state. Row values, `q`, and row payloads stay in memory only and are not written to `localStorage`, `sessionStorage`, or the location URL.
- Creation wizard separates database name, new/existing project role, generated password behavior, and direct/pooled explanation.
- Destructive operations live in a distinct danger area and disclose source connection termination/role cleanup.
- Security overview loads authenticated `GET /api/v1/postgres/security` (session cookie, no CSRF, no query). HTTP 200 shows PUBLIC CONNECT, owner privilege flags, protected labels, and connection groups. When cluster `saved_credential.status` is `ok`, the page shows fact **Missing vault entries** as `summary.missing_password_count` (including 0) and does not show Saved credential / Not available. When status is `not_available` or missing, it shows **Saved credential** = Not available and does not invent a 0 count. The header does not say credentials are not loaded; it states passwords are not revealed. Reason strings are never rendered. HTTP 503 uses the same “PostgreSQL is unavailable” copy as Databases and is never an empty healthy overview. HTTP 401 is session-expired and never renders `summary`, `databases`, or `connections`. Protected databases are listed. This page does not rotate, create, or reveal credentials. Residual: vault decrypt, rotation eligibility, and create.

## Redis workflow

- The ACL users page lists Redis ACL users and inspects one user’s modeled rules without secrets. The title is “ACL users” from navigation. Create ACL user is a page-header action (not the topbar) and is hidden when the list is `not_configured` or `unavailable`.
- Authenticated `GET /api/v1/redis/users` uses the session cookie, no query string, and no CSRF. Protected users are listed and inspectable. Ledger rows are buttons showing username (Protected badge), Enabled or Disabled, preset, key prefix, and Limited when `rule_fidelity` is `limited`.
- `state: not_configured` is not an empty healthy list. `state: unavailable` with `unreachable`, `auth_failed`, or `permission_denied` uses copy distinct from “No ACL users.” HTTP 200 `state: ok` with `users: []` is “No ACL users.” Truncation is disclosed. Envelope 401 uses “Your session has expired. Sign in again to continue.”
- Selecting a row loads `GET /api/v1/redis/users/{username}` with AbortController; a slower first selection is ignored. Below 1024px, sibling ledger rows hide while one user is inspected (Back to users restores the list) and focus moves into the inspector. The inspector shows commands, categories, optional queue kind, and limited-rule copy when Redgres cannot model the rules exactly. For a non-protected user when the list is `ok`, the inspector offers Disable when enabled and Enable when not, plus Rotate and Edit permissions next to those actions. Enable/Disable are hidden for protected users, hidden when the list is `not_configured` or `unavailable`, hidden while detail is loading, and disabled while the POST is in flight. They use CSRF and an empty body; they do not open a credential ticket or use danger red. Rotate is a text-button, not danger red, with the same visibility rules, and is disabled while rotate is in flight or a credential ticket is open. Rotate opens a confirm dialog (`role=dialog`, title “Rotate password?”) stating that this issues a new password and the previous credential stops working immediately and cannot be recovered. The actions are Cancel and Rotate now (primary, not danger). Confirmation is not authorization. POST `/api/v1/redis/users/{username}/credentials/rotate` uses CSRF, `encodeURIComponent(username)`, and an empty body with no password. HTTP 200 closes the confirm dialog and opens the existing one-time credential ticket via `parseCredential`; extra secret fields are ignored and nothing is auto-copied. After dismiss, the inspector stays on that user and refreshes the list and detail without keeping the password in inspect state. Rotate 401 is session-expired with no ticket; 404 is not-found; 403 is the protected copy; 503 is Redis unavailable. Edit permissions is a text-button, not danger red, with the same visibility rules, and is disabled while a PATCH is in flight, while enable/rotate is in flight, or while a credential ticket is open. It opens a dialog (`role=dialog`, title “Edit permissions”, same focus trap as create). Username is shown read-only and is not sent in the body. The form prefills key prefix, preset, and queue type from the inspected user. Edit offers Custom in addition to the named presets. If the inspected preset is `custom` (or otherwise not a named preset), the select is Custom rather than Cache read/write. Queue type is shown only for Queue/worker. Choosing Custom (on open, or when the operator switches to Custom) loads `GET /api/v1/redis/commands` with the session cookie and no CSRF. That catalog is a stacked checklist of labeled checkboxes (one command per row, 44px hit area, shared `--line` / `--radius-surface` chrome); the client never invents a fallback list and never uses a free-text ACL box. Catalog 401 or 503 errors stay on the dialog alert and do not mark Key prefix invalid. Prefill is inspect `commands` ∩ catalog; unknown names are dropped (including limited rules). Switching from a named inspect preset to Custom prefills that named command set against the catalog. Catalog 401 or 503 disables Save and shows the same session-expired or Redis-unavailable copy as named PATCH, with no invented checkboxes. The client never GETs `/presets` for this dialog and never sends a password. PATCH `/api/v1/redis/users/{username}` uses CSRF, `encodeURIComponent(username)`, and body `{ key_pattern, preset }` with `queue_kind` only when preset is `queue-worker`, or `{ key_pattern, preset: "custom", commands }` when Custom (no `queue_kind`). HTTP 200 closes the dialog and applies `user` to the inspector and matching ledger row; it does not open a credential ticket. PATCH 401 is session-expired with no ticket; 404 is not-found; 403 is the protected copy; 503 is Redis unavailable. State stays in memory only and clears on logout, section change, or inspect of another user. A 404 is a not-found alert, never an empty healthy inspector. Detail `unavailable` uses the same typed Redis copy as the list. The inspector does not offer delete.
- Truncation is disclosed as an alert above the ledger. Painted identifiers use `displayText()` plus bidi isolate. The Redis service rail is wayfinding; healthy and limited states do not use danger red. Login never fetches or POSTs `/api/v1/redis/users`, never PATCHes users, and never GETs `/api/v1/redis/commands`. List, inspector, and credential-ticket state stay in memory (no `localStorage` / `sessionStorage`) and clear on logout.
- Create uses username, key prefix, and a permission preset. The prefix is suggested from the username until the prefix field is edited. The preset select defaults to Cache read/write and also offers Read only, Queue/worker, and Custom. Queue type (Lists, Streams, Sorted sets) is shown only for Queue/worker. Choosing Custom loads `GET /api/v1/redis/commands` with the session cookie and no CSRF. That catalog is a stacked checklist of labeled checkboxes (one command per row, 44px hit area, shared `--line` / `--radius-surface` chrome); the client never invents a fallback list, never prefills inspect commands (new user), and never uses a free-text ACL box. Empty selection, catalog loading, or catalog 401/503 disables Create. Catalog 401 or 503 errors stay on the dialog alert and do not mark Key prefix invalid. The catalog fetch aborts when leaving Custom. Named POST body is `{ username, key_pattern, preset }` with CSRF; `queue_kind` is included only when preset is `queue-worker`; named bodies omit `commands`. Custom POST is `{ username, key_pattern, preset: "custom", commands }` with CSRF and no `queue_kind`. The body never includes a password, categories, or enabled. Search remains inspect-only.
- A successful create refreshes the list and opens a one-time Redis credential ticket. The new user is inspected only after the ticket is dismissed, so the password does not linger in inspect state. The ticket shows username, password, and URL copy only when `credential.urls.primary` is present; it does not auto-copy. Extra secret fields in the response are ignored.
- Residual: delete, Overview quick-create. Permission presets navigation stays a placeholder.

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
- Clearly labels PostgreSQL revealable vs Redis one-time semantics. Redis create and rotate use “shown now” one-time copy; Redgres cannot show the password again.
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
