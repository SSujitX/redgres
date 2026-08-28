import { useEffect, useRef, useState } from "react";
import { lookupDoc, navEntries, visibleNavEntries, type SectionId } from "../../nav";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import type { ToolLinks } from "../../api/auth";
import { errorMessage, fetchStatus, isStatusPayload, type StatusComponent } from "../../api/status";
import Icon from "../icons";
import BrandLogo from "../BrandLogo";
import NavigationSearch from "../search/NavigationSearch";
import OverviewPage from "../../features/overview/OverviewPage";
import DatabasesPage from "../../features/postgres/DatabasesPage";
import { SectionPage } from "../../features/pages/Placeholders";
import ChangePasswordDialog from "../../features/auth/ChangePasswordDialog";
import { displayText } from "../../text/displayText";
import ThemeToggle from "../ThemeToggle";

type AppShellProps = {
  username: string;
  csrf: string;
  toolLinks: ToolLinks;
  version?: string;
  onLogout: () => void;
  onPasswordChanged: () => void;
  loggingOut: boolean;
};

const NAV_GROUPS = ["Overview", "PostgreSQL", "Redis ACL", "Audit", "System", "Documentation"];
const sessionExpired = "Your session has expired. Sign in again to continue.";
const statusUnavailable = "Component status is unavailable. Try again.";
const HEALTH_COMPONENT_IDS: StatusComponent["id"][] = ["redgres_state", "postgres_direct", "pgbouncer", "redis"];

type AggregateHealthState = "loading" | "healthy" | "degraded" | "unavailable";

export default function AppShell({ username, csrf, toolLinks, version, onLogout, onPasswordChanged, loggingOut }: AppShellProps) {
  const [section, setSection] = useState<SectionId>("overview");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [ownerOpen, setOwnerOpen] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [focusDatabase, setFocusDatabase] = useState<string | null>(null);
  const [focusUsername, setFocusUsername] = useState<string | null>(null);
  const [focusArticle, setFocusArticle] = useState<string | null>(null);
  const [focusNonce, setFocusNonce] = useState(0);
  const [postgresCreateIntent, setPostgresCreateIntent] = useState(false);
  const [statusComponents, setStatusComponents] = useState<StatusComponent[] | null>(null);
  const [statusError, setStatusError] = useState("");
  const [statusLoading, setStatusLoading] = useState(true);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const searchButtonRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const ownerMenuRef = useRef<HTMLDivElement>(null);
  const logoutItemRef = useRef<HTMLButtonElement>(null);
  const statusAbortRef = useRef<AbortController | null>(null);

  useFocusTrap(drawerRef, drawerOpen, menuButtonRef);

  useEffect(() => {
    document.title = `${sectionTitleSafe(section)} — Redgres`;
  }, [section]);

  useEffect(() => {
    statusAbortRef.current?.abort();
    const controller = new AbortController();
    statusAbortRef.current = controller;
    void loadStatus(controller);
    return () => controller.abort();
  }, [section]);

  async function loadStatus(controller: AbortController) {
    setStatusLoading(true);
    setStatusError("");
    setStatusComponents(null);
    try {
      const result = await fetchStatus({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && isStatusPayload(result.body)) {
        setStatusComponents(result.body.components);
        return;
      }
      if (result.status === 401) {
        setStatusError(sessionExpired);
        return;
      }
      setStatusError(errorMessage(result.body, statusUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setStatusError(statusUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setStatusLoading(false);
      }
    }
  }

  function refreshStatus() {
    statusAbortRef.current?.abort();
    const controller = new AbortController();
    statusAbortRef.current = controller;
    void loadStatus(controller);
  }

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      const typing =
        target &&
        (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable);
      if (event.key === "Escape") {
        setDrawerOpen(false);
        setOwnerOpen(false);
        setSearchOpen(false);
        return;
      }
      if (typing) {
        return;
      }
      if (event.key === "/" || ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k")) {
        event.preventDefault();
        setDrawerOpen(false);
        setOwnerOpen(false);
        setSearchOpen(true);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (!ownerOpen) {
      return;
    }
    function onPointer(event: MouseEvent) {
      if (ownerMenuRef.current && !ownerMenuRef.current.contains(event.target as Node)) {
        setOwnerOpen(false);
      }
    }
    document.addEventListener("mousedown", onPointer);
    return () => document.removeEventListener("mousedown", onPointer);
  }, [ownerOpen]);

  function go(id: SectionId) {
    const nextSection = id === "postgres-create" ? "postgres" : id;
    setPostgresCreateIntent(id === "postgres-create");
    setFocusArticle(null);
    if (nextSection !== "postgres") {
      setFocusDatabase(null);
    }
    if (nextSection !== "redis") {
      setFocusUsername(null);
    }
    setSection(nextSection);
    setDrawerOpen(false);
    setSearchOpen(false);
    setOwnerOpen(false);
  }

  function selectDatabase(name: string) {
    setFocusDatabase(name);
    setFocusNonce((n) => n + 1);
    go("postgres");
  }

  function selectAclUser(name: string) {
    setFocusUsername(name);
    setFocusNonce((n) => n + 1);
    go("redis");
  }

  function selectArticle(id: string) {
    const article = lookupDoc(id);
    if (!article) {
      go("docs");
      return;
    }
    setFocusDatabase(null);
    setFocusUsername(null);
    setFocusArticle(article.id);
    setFocusNonce((n) => n + 1);
    setSection("docs");
    setDrawerOpen(false);
    setSearchOpen(false);
    setOwnerOpen(false);
  }

  function openSearch() {
    setDrawerOpen(false);
    setOwnerOpen(false);
    setSearchOpen(true);
  }

  const aggregateHealth = aggregateHealthState(statusComponents, statusError, statusLoading);

  const nav = <PrimaryNav section={section} onSelect={go} />;

  return (
    <div className="app-shell">
      <div className="shell-content" inert={drawerOpen || searchOpen}>
        <aside className="app-sidebar" aria-label="Redgres">
          <div className="brand">
            <button
              type="button"
              className="brand-home"
              aria-label="Redgres home"
              onClick={() => go("overview")}
            >
              <BrandLogo />
            </button>
          </div>
          {nav}
          <div className="sidebar-footer">
            <span className="sidebar-footer-brand">
              <span>Redgres</span>
              {version ? <span className="sidebar-footer-version">{displayText(version)}</span> : null}
            </span>
            <AggregateHealth state={aggregateHealth} />
          </div>
        </aside>

        <div className="app-main">
          <header className="topbar">
            <button
              ref={menuButtonRef}
              type="button"
              className="icon-button drawer-toggle"
              aria-label="Open menu"
              onClick={() => {
                setOwnerOpen(false);
                setSearchOpen(false);
                setDrawerOpen(true);
              }}
            >
              <Icon name="menu" />
              <span className="button-label">Menu</span>
            </button>
            <p className="topbar-context">{sectionTitleSafe(section)}</p>
            <div className="topbar-health">
              <AggregateHealth state={aggregateHealth} />
            </div>
            <button
              ref={searchButtonRef}
              type="button"
              className="icon-button"
              aria-label="Search"
              onClick={openSearch}
            >
              <Icon name="search" />
              <span className="button-label">Search</span>
            </button>
            <button
              type="button"
              className="icon-button help-button"
              aria-label="Help"
              onClick={() => go("docs")}
            >
              <Icon name="help" />
              <span className="button-label">Help</span>
            </button>
            <ThemeToggle />
            <div className="owner-menu" ref={ownerMenuRef}>
              <button
                type="button"
                className="owner-button"
                aria-label={username}
                aria-expanded={ownerOpen}
                aria-haspopup="menu"
                onClick={() => setOwnerOpen((value) => !value)}
                onKeyDown={(event) => {
                  if (event.key === "ArrowDown") {
                    event.preventDefault();
                    setOwnerOpen(true);
                    queueMicrotask(() => logoutItemRef.current?.focus());
                  }
                }}
              >
                <Icon name="owner" />
                <span className="owner-name">{username}</span>
              </button>
              {ownerOpen ? (
                <div className="owner-dropdown" role="menu">
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setOwnerOpen(false);
                      setPasswordOpen(true);
                    }}
                  >
                    Change password
                  </button>
                  <button
                    ref={logoutItemRef}
                    type="button"
                    role="menuitem"
                    disabled={loggingOut}
                    onClick={onLogout}
                  >
                    Log out
                  </button>
                </div>
              ) : null}
            </div>
          </header>
          <main className="workspace">
            {section === "overview" ? (
              <OverviewPage
                toolLinks={toolLinks}
                onNavigate={go}
                statusComponents={statusComponents}
                statusError={statusError}
                statusLoading={statusLoading}
                onRefreshStatus={refreshStatus}
              />
            ) : section === "postgres" ? (
              <DatabasesPage
                csrf={csrf}
                focusDatabase={focusDatabase}
                focusNonce={focusNonce}
                openCreate={postgresCreateIntent}
                onCreateIntentConsumed={() => setPostgresCreateIntent(false)}
              />
            ) : (
              <SectionPage
                section={section}
                csrf={csrf}
                focusDatabase={focusDatabase}
                focusUsername={focusUsername}
                focusArticle={focusArticle}
                focusNonce={focusNonce}
                onSelectArticle={selectArticle}
                onBackToDocs={() => go("docs")}
              />
            )}
          </main>
        </div>
      </div>

      {drawerOpen ? (
        <div className="drawer-backdrop" onClick={() => setDrawerOpen(false)}>
          <div
            ref={drawerRef}
            className="nav-drawer"
            role="dialog"
            aria-modal="true"
            aria-label="Navigation"
            onClick={(event) => event.stopPropagation()}
          >
            {nav}
            <button type="button" className="text-button" onClick={() => setDrawerOpen(false)}>
              Close menu
            </button>
          </div>
        </div>
      ) : null}

      <NavigationSearch
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        onSelect={go}
        onSelectDatabase={selectDatabase}
        onSelectAclUser={selectAclUser}
        onSelectArticle={selectArticle}
        restoreFocusRef={searchButtonRef}
      />

      {passwordOpen ? (
        <ChangePasswordDialog
          csrf={csrf}
          onClose={() => setPasswordOpen(false)}
          onSuccess={onPasswordChanged}
        />
      ) : null}
    </div>
  );
}

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function aggregateHealthState(
  components: StatusComponent[] | null,
  error: string,
  loading: boolean,
): AggregateHealthState {
  if (loading && components === null && error === "") {
    return "loading";
  }
  if (error !== "" || components === null || components.length === 0) {
    return "unavailable";
  }
  const byId = new Map(components.map((component) => [component.id, component]));
  return HEALTH_COMPONENT_IDS.every((id) => byId.get(id)?.state === "ok") ? "healthy" : "degraded";
}

function AggregateHealth({ state }: { state: AggregateHealthState }) {
  const copy =
    state === "healthy"
      ? "Healthy"
      : state === "degraded"
        ? "Degraded"
        : state === "unavailable"
          ? "Health unavailable"
          : "Checking health";
  const showWarning = state === "degraded" || state === "unavailable";
  const mark = state === "healthy" ? "✓" : state === "loading" ? "…" : "!";
  return (
    <span
      className={`aggregate-health aggregate-health-${state}`}
      aria-live="polite"
      aria-atomic="true"
      aria-label={`Aggregate health: ${copy}`}
    >
      <span className={showWarning ? "warning-mark health-mark" : "health-mark"} aria-hidden="true">
        {mark}
      </span>
      <span className="aggregate-health-copy">{copy}</span>
    </span>
  );
}

function PrimaryNav({
  section,
  onSelect,
}: {
  section: SectionId;
  onSelect: (id: SectionId) => void;
}) {
  return (
    <nav className="app-nav" aria-label="Primary">
      {NAV_GROUPS.map((group) => {
        const items = visibleNavEntries(section).filter((entry) => entry.group === group);
        if (items.length === 0) {
          return null;
        }
        return (
          <div key={group} className="nav-group">
            <p className="nav-group-label">{group}</p>
            {items.map((entry) => (
              <button
                key={entry.id}
                type="button"
                className={[
                  "nav-item",
                  section === entry.id ? "nav-item-active" : "",
                  entry.service ? `nav-item-${entry.service}` : "",
                ]
                  .filter(Boolean)
                  .join(" ")}
                title={entry.label}
                onClick={() => onSelect(entry.id)}
                aria-current={section === entry.id ? "page" : undefined}
              >
                <Icon name={entry.icon} />
                <span className="nav-item-label">{entry.label}</span>
              </button>
            ))}
          </div>
        );
      })}
    </nav>
  );
}

function sectionTitleSafe(id: SectionId): string {
  const found = navEntries.find((entry) => entry.id === id) ?? {
    group: "Overview",
    label: "Overview",
  };
  if (found.group === found.label) {
    return found.label;
  }
  return `${found.group} · ${found.label}`;
}
