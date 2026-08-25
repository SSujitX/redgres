# Redgres UI design system

Status: target. This is the visual and responsive implementation contract for the Redgres web application. It complements [UX.md](UX.md), the functional requirements in [PRD.md](PRD.md), and the security boundaries in [SECURITY.md](SECURITY.md).

## 1. Product and design thesis

Redgres is an operator console for one PostgreSQL cluster and one Redis instance. Its primary user is a technical owner who needs to understand state, find a resource quickly, and perform a precise action without confusing the two engines or exposing a credential.

The visual direction is a **dual-engine operations ledger**: a quiet, high-clarity workspace with database facts aligned like an operational record. A paired PostgreSQL-blue and Redis-red service rail is the signature element. It identifies service context in navigation, headers, search results, and status summaries; it is functional wayfinding, not decoration.

The interface must feel deliberate and recognizable without becoming theatrical. Do not use dashboard-template gradients, glassmorphism, glowing cards, oversized marketing statistics, decorative charts, excessive pills, or animation on sensitive workflows.

## 2. Visual foundation

### Color tokens

Use semantic tokens in CSS/Tailwind; feature code must not scatter raw color values.

| Token | Light value | Purpose |
|---|---:|---|
| `canvas` | `#F4F6F8` | App background; cool neutral, not warm cream |
| `surface` | `#FFFFFF` | Primary work surfaces |
| `ink` | `#17202A` | Sidebar and strongest text |
| `muted` | `#667085` | Secondary text |
| `line` | `#D8DEE6` | Borders, table rules, separators |
| `postgres` | `#336791` | PostgreSQL service identity only |
| `redis` | `#C9362B` | Redis service identity only; not the destructive token |
| `success` | `#157A55` | Healthy/success with icon and text |
| `warning` | `#A15C00` | Warning/degraded with icon and text |
| `danger` | `#B42318` | Destructive/error semantics |
| `focus` | `#2563EB` | Keyboard focus ring |
| `overlay` | `color-mix(in srgb, ink 32%, transparent)` | Drawer and search dimming |

PostgreSQL blue and Redis red identify service ownership; they do not replace success, warning, or danger semantics. Every status includes text/iconography and passes WCAG 2.2 AA contrast. A future dark theme is a separate tested deliverable, not automatic color inversion.

### Typography

- **Interface and headings:** Manrope, locally bundled variable font when licensing/package review is complete; fallback `Segoe UI`, sans-serif.
- **Identifiers, versions, ports, sizes, timestamps, commands:** IBM Plex Mono, locally bundled; fallback `Cascadia Mono`, monospace.
- Use tabular numbers for metrics and aligned values.
- Body text defaults to 14–16px with at least 1.45 line height. Dense tables may use 13px only when zoom, contrast, and row targets remain usable.
- Page titles are restrained (24–32px), sentence case, and never marketing headlines inside the authenticated console.

Do not fetch fonts from a third-party origin in production. Bundle approved font files or use the documented system fallbacks.

### Shape, depth, and spacing

- Base spacing unit: 4px; common gaps: 8, 12, 16, 24, 32.
- Controls: 8px radius. Work surfaces: 10–12px radius. Pills are reserved for compact state/category labels.
- Prefer borders and spacing over shadows. Floating menus/dialogs may use one restrained shadow token.
- Minimum interactive target: 44×44 CSS px on touch layouts; dense desktop controls may be visually smaller while retaining an equivalent hit area.
- Icons come from one reviewed icon family, use consistent stroke weight, and never carry meaning without a label or accessible name. Until a licensed webfont/package is adopted, the shell uses the local 24×24, 1.75-stroke set in `web/src/components/icons.tsx`.

## 3. Authenticated application shell

```text
Wide desktop (>= 1024)
┌──────────────────────┬──────────────────────────────────────────────────┐
│ Redgres + service rail│ Breadcrumb / context   Search      Help  Owner ▾ │
│                      ├──────────────────────────────────────────────────┤
│ Overview             │ Page title + service context + primary action   │
│ PostgreSQL           │                                                  │
│ Redis ACL            │ Main workspace: lists, inspectors, forms        │
│ Audit                │                                                  │
│ System               │                                                  │
│ Documentation        │                                                  │
│                      │                                                  │
│ health · version     │                                                  │
└──────────────────────┴──────────────────────────────────────────────────┘

Small screen (< 768)
┌───────────────────────────────────────────────┐
│ Menu  Redgres/context     Search   Owner      │
├───────────────────────────────────────────────┤
│ Page title                         Action     │
│                                               │
│ Single-column workspace                       │
│ Cards/lists; tables scroll inside themselves  │
└───────────────────────────────────────────────┘
```

### Left sidebar

- Desktop expanded width: 248px; wide screens may use 264px. It does not grow indefinitely.
- Medium layouts use a 72px icon rail. Labels appear in accessible tooltips and the current section remains unambiguous.
- Small layouts replace the persistent sidebar with a modal navigation drawer triggered from the topbar. The drawer traps focus, closes on route selection/Escape, restores focus, and never leaves the document scroll-locked.
- Group navigation by operator intent: Overview; PostgreSQL; Redis ACL; Audit; System; Documentation. Nested items appear only for the active service and do not create a permanently deep tree.
- The paired service rail runs beside PostgreSQL and Redis navigation. The active page uses the relevant engine color plus text/shape; other global pages use neutral ink.
- The footer shows Redgres release and compact aggregate health. It must not expose hostnames, usernames, or infrastructure detail before the owner opens System.
- Sidebar collapse preference is non-sensitive and may persist. Navigation state must never persist credentials or credential-bearing URLs.

### Topbar

- Sticky within the application shell, 56–64px high, with an opaque background and bottom rule.
- Left/center: breadcrumb or current service context, then global search on layouts with room.
- Right: help/documentation, compact dependency health indicator, and the authenticated owner menu in the top-right corner. The owner menu contains session-safe actions such as account/security context and Log out; it never displays downstream administrator credentials.
- On small screens, show icon controls for menu, search, and owner. Search opens as a full-width sheet/dialog rather than shrinking into an unusable field.
- Primary page actions belong in the page header, not the global topbar, so Create database cannot be mistaken for Create Redis ACL user.

### Global search / command palette

- Open from the topbar, `/`, or `Ctrl/Cmd+K`; do not steal `/` while typing in a field/editor.
- Search is authenticated, bounded, cancellable, and grouped into PostgreSQL databases, Redis ACL users, navigation, documentation, and safe read-only actions.
- Results show an explicit service marker and human-readable type. Identically named PostgreSQL/Redis resources must remain distinguishable.
- Search never indexes or displays passwords, complete credential URLs, secret files, session/CSRF tokens, admin connection values, raw audit metadata, or hidden/protected resources the owner cannot manage.
- Destructive actions do not execute from search. Search may navigate to their dedicated guarded surface.
- Empty, loading, degraded-service, too-short-query, and no-result states provide a useful next action. Keyboard and screen-reader behavior are first-class.
- On narrow screens the search surface is full-screen below the safe-area inset. On desktop it is a centered palette no wider than 720px.

## 4. Responsive behavior

Breakpoints describe behavior, not device brands:

| Range | Shell | Content behavior |
|---|---|---|
| 320–479px | mobile topbar + navigation drawer | One column; full-width sheets; 16px gutters; primary action may become a labeled icon or full-width button |
| 480–767px | mobile topbar + navigation drawer | One column; related fields may form two columns only when labels/values remain readable |
| 768–1023px | 72px icon rail + topbar | Master list and detail usually switch views rather than squeeze side-by-side |
| 1024–1439px | 248px sidebar + topbar | List/detail split allowed; page gutters 24–32px |
| 1440px and above | 264px sidebar + topbar | Content uses available width up to a sensible reading/data maximum; avoid giant empty margins around tables |

Required responsive invariants:

- Core workflows function at 320 CSS px width and at 200% browser zoom.
- The viewport itself has no horizontal scrolling. Wide data grids own their bounded horizontal scroll, preserve row/header context, and offer a stacked detail view on small screens.
- Dialogs become bottom/full-screen sheets where necessary; destructive confirmation and credential tickets remain explicit and are not compressed into tiny cards.
- Safe-area insets are respected on mobile. Virtual keyboard appearance must not hide the focused control or primary action.
- Hover enhancements are optional; all information and actions work with touch, keyboard, and pointer.
- Loading skeletons preserve layout and do not animate under reduced motion. Content does not jump when the sidebar changes mode.

## 5. Login and session surfaces

The unauthenticated login route does not render the authenticated sidebar/topbar or reveal service health.

- At 1024px and above, use a two-part composition: a quiet Redgres identity/service-rail field on the left and a compact login panel in the upper-right region of the content area. Keep enough margin that it reads as intentional, not pinned to the browser edge.
- Below 1024px, center a single-column login panel with the Redgres identity above it. At 320px it uses the full available width with 16px gutters.
- Show product name, one plain-language sentence, username, password, password visibility control, primary “Log in” action, generic authentication errors, lockout/retry state, and a short “Protected by your organization’s access policy” note where configured.
- Do not show decorative infrastructure metrics, database names, raw dependency failures, default credentials, password hints, or a browser account-creation flow.
- After login, preserve only a validated safe return route. Never return to a credential ticket or replay a mutation.
- Session-expiry UI clears sensitive local state before presenting the login route.

## 6. Content patterns

### Page header

Every page header has one title, optional concise description, service/status context, and at most one primary action. PostgreSQL and Redis headers use the service rail, not a full-page color wash.

### Lists and inspectors

- Desktop favors a ledger-like table/list with sticky headers, bounded density, sorting labels, and a separate inspector/detail region.
- Small screens use summary cards or disclosure rows that retain field labels; never transform a table into unlabeled value stacks.
- Search/filter/sort state is shareable only when it contains non-sensitive query parameters. Credentials never enter URLs.
- Empty states state why the list is empty and present the safe next action.

### Forms and checklists

- Dialog fields use `.field-stack` (column flex, `--space-3` gaps, 44px control height).
- Allow-list command catalogs (Edit permissions Custom) use `.command-checklist`: one labeled checkbox per row, `min-height: 44px`, `min-width: 0`, `--space-2` label gap, and `--line` / `--radius-surface` fieldset chrome. Do not wrap commands as inline labels.

### Status and metrics

- Prefer compact labeled facts and short trends only when historical data exists. Do not invent charts from one instantaneous value.
- Overview Redis metrics are a compact labeled list on the Redis card only. Labels stay visible at 360px; numeric values use tabular numbers.
- Dependency failures are independent and actionable. PostgreSQL down must not blank Redis management, and vice versa.
- Reserve red danger styling for errors/destructive actions; Redis identity red cannot make a healthy Redis panel look failed.

### Stored and attacker-influenced text

Audit history (and any later surface that renders stored actor, target, action, outcome, request identifiers, or similar fields) replaces bidirectional and other format/control characters with U+FFFD at render time, then wraps the visible value in `unicode-bidi: isolate` with base `direction: ltr`. CSS isolation is defense in depth only: source bidi controls remain honored until replaced. See `web/src/text/displayText.ts` and the PLAT-003 audit-history UI entry in [TRACEABILITY.md](TRACEABILITY.md). Homoglyph detection is out of scope.

### Credentials and danger

- Credential tickets and destructive confirmation surfaces intentionally drop decorative service color and motion, emphasizing labels, scope, and consequences.
- Never toast a password or complete connection URL. Success toasts contain only non-secret confirmation.
- Dangerous actions live in a dedicated section, never beside ordinary edits with equal visual weight.

## 7. Motion

- Motion communicates spatial change: drawer/sheet entry, inspector transition, search opening, and progress state.
- Use approximately 120–180ms for small feedback and 180–240ms for drawers/sheets, with non-bouncy easing. No ambient animation.
- Respect `prefers-reduced-motion`; use immediate state changes or opacity-only transitions where needed.
- Credential reveal, copy, reauthentication, and destructive confirmation do not use celebratory or attention-seeking motion.

## 8. Implementation and review gate

Before a UI slice is accepted:

1. Map it to PRD requirements and use real Redgres terminology/content.
2. Use shared tokens and shell primitives; do not create page-local visual systems.
3. Capture and inspect at least 360×800, 768×1024, 1280×800, and 1600×1000, plus 200% zoom for a core workflow.
4. Verify keyboard order, visible focus, drawer/dialog focus behavior, screen-reader labels/announcements, touch targets, contrast, and reduced motion.
5. Test loading, empty, error, degraded dependency, long identifier, dense data, session expiry, and credential clearing states.
6. Run frontend unit/component tests, production build, and browser-level responsive checks from [TESTING.md](TESTING.md).

No screenshot is approval by itself. The implementation must satisfy functional, security, responsive, and accessibility tests.
