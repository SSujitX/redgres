---
name: redgres-ui-design
description: Design, implement, or review Redgres React screens, navigation, responsive layouts, visual tokens, search, login, tables, dialogs, and accessibility. Use for any Redgres web UI change; not for backend-only or deployment-only work.
---

# Redgres UI design

Create a precise operator interface that remains unmistakably Redgres and usable from 320px through wide desktop.

## Required context

Read these before planning or editing UI:

- `docs/UI_DESIGN_SYSTEM.md` for the visual and responsive contract;
- `docs/UX.md` and the relevant `docs/PRD.md` requirements;
- `docs/API.md`, `docs/SECURITY.md`, and `docs/DATA_AND_SECRETS.md` for behavior and secret boundaries;
- `AGENTS.md` Language and `docs/GLOSSARY.md` for exact PostgreSQL/Redis vocabulary;
- `docs/UI_WORKLOG.md` (compact record of the current terminal theme, changed files, and the screenshot index in `web/screenshots/`) — read it before restyling so you reuse the established language and evidence.

Treat target documentation as requirements, not implementation evidence.

## Working method

Before coding, state the screen’s user job, data states, responsive modes, keyboard path, and affected PRD IDs. Reuse the shared shell and tokens. If a required token, component contract, or API is missing, update/propose the governing spec rather than inventing a page-local convention.

Use the dual-engine operations-ledger direction: quiet neutral surfaces, PostgreSQL/Redis service rail for wayfinding, restrained typography, borders over decorative shadow, and explicit labels. Do not introduce generic dashboard gradients, glass panels, oversized metric cards, ungrounded charts, excessive pills, or decoration that competes with operational data.

Keep the persistent/sidebar, icon-rail, and mobile-drawer modes coherent. Global search is safe navigation/read-only discovery; it never exposes credentials or directly executes destructive actions. The authenticated owner menu stays top-right. The unauthenticated login route never reveals service health or renders the authenticated shell.

Security UX is part of correctness. Keep credentials local/ephemeral and clear them on every required boundary. Credential and destructive surfaces use minimal motion and unambiguous scope. UI confirmation never replaces server authorization.

## Verification

Use real Redgres content, including long identifiers and degraded states. Inspect 360×800, 768×1024, 1280×800, and 1600×1000 plus 200% zoom. Verify keyboard/focus, screen-reader naming, contrast, touch targets, reduced motion, mobile safe areas, bounded table overflow, loading/empty/error states, session expiry, and credential clearing.

Run the focused frontend tests and production build from `docs/TESTING.md`. For implementation handoff, report screenshots/viewports inspected, commands/results, unmet states, and any spec/API gap. Do not claim quality from a single desktop screenshot.
