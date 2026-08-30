# Redgres UI worklog

Compact record of the 2026 terminal-theme UI work. Future UI agents/skills: read this before changing the shell/login so you reuse the established visual language and can find screenshot evidence.

## Screenshots

All capture screenshots live in `web/screenshots/` (Playwright, mocked `/api/v1/*` routes, deviceScaleFactor 2). Naming: `<surface>-<state>-<theme>.png`. Regenerate with a throwaway script, then delete it.

| Surface | Files |
|---|---|
| Login | `login-desktop-light/dark.png`, `login-mobile-light/dark.png` |
| Shell | `shell-desktop-overview-light/dark.png`, `shell-desktop-databases.png`, `shell-mobile-overview.png`, `shell-sidebar-light.png` |
| Domain & Network | `domain-wizard-light/mobile.png`, `domain-configured-light/dark.png` |
| Databases | `databases-detail-light.png`, `databases-rows-light/dark.png` |

## Visual language (2026 terminal theme)

- **Terminal-green accent** for primary actions and focus: light `#1a7f37`, dark `#3fb950` (`--accent*` in `web/src/styles/tokens.css`).
- **Dual-engine identity**: PostgreSQL blue (`--postgres`), Redis red (`--redis`) — service context via labels/titles only. **No colored left rails anywhere** (sidebar, search results, ledgers, status/endpoint cards, page headers, detail panels): active rows use a neutral ring (`inset 0 0 0 1px var(--line-strong)` + `--surface-hover`). Do not re-add `inset 3px 0 0` lines.
- **Monospace** (`--font-mono`) for identifiers, data, labels, inputs, and action buttons.
- **Panels**: `.panel` (surface card + mono uppercase header), `.panel-sub` (inset sub-card).
- **Login**: terminal-window card (title-bar dots, mono form, green button) + block-grid canvas that bumps near the cursor (rAF, disabled under reduced motion). Logo links to `/`.
- **Light/dark**: `data-theme` on `<html>`, `useTheme` hook, FOUC-prevention script in `web/index.html`.
- **Version in footer**: GET `/api/v1/session` returns `version` (default `dev`, overridable via `-ldflags "-X github.com/SSujitX/redgres/internal/version.Version=…"`); the sidebar footer renders it as a pill next to "Redgres".
- No gradients/glassmorphism/glow cards; quiet ledger surfaces; danger styling only for destructive.

## Changed in this pass

- Domain wizard: API token vs OAuth chosen once; API-token apply never shows OAuth paste. Status copy says the installer enables the Ubuntu connector and apply starts it. TLS retry copy says the same API token is reused.

- `web/src/styles/tokens.css` — accent palette, dark theme, typography, radii, elevation, motion.
- `web/src/styles/globals.css` — typography, buttons, `.panel`, databases page.
- `web/src/styles/shell.css` — sidebar/topbar/cards, nav section gaps (no left service lines on nav), domain page.
- `web/src/styles/login.css` — terminal login.
- `web/src/hooks/useTheme.ts`, `web/src/components/ThemeToggle.tsx`, `web/src/components/icons.tsx` — theme system + sun/moon icons.
- `web/index.html` — theme boot script + theme-color meta.
- `web/src/features/auth/LoginPage.tsx` — terminal login (logo→home, block-grid bump).
- `web/src/features/domain/DomainNetworkPage.tsx` — panelized wizard, button classes, rounded terminal inputs.
- `web/src/features/postgres/DatabasesPage.tsx` — detail panel card, facts grid, rows sub-panel.
- `web/src/components/shell/AppShell.tsx` — theme toggle in topbar, logo→home button, footer version pill.
- `internal/version/version.go` (+ test) — build-time release version, exposed on `/api/v1/session` (`auth_routes.go` + test).

## Pre-existing failures (not caused by UI work)

- vitest: 2 in `web/src/App.test.tsx` (shell status re-fetch on nav vs mount-only expectation; from commit `6416f9c`).
- e2e: 2 in `e2e/domain-wizard.spec.ts` (route interception/backend-dependent at :8790).
