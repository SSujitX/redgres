import { useEffect, useRef, useState } from "react";
import { navEntries, visibleNavEntries, type SectionId } from "../../nav";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import Icon from "../icons";
import NavigationSearch from "../search/NavigationSearch";
import { SectionPage } from "../../features/pages/Placeholders";

type AppShellProps = {
  username: string;
  onLogout: () => void;
  loggingOut: boolean;
};

const NAV_GROUPS = ["Overview", "PostgreSQL", "Redis ACL", "Audit", "System", "Documentation"];

export default function AppShell({ username, onLogout, loggingOut }: AppShellProps) {
  const [section, setSection] = useState<SectionId>("overview");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [ownerOpen, setOwnerOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [focusDatabase, setFocusDatabase] = useState<string | null>(null);
  const [focusUsername, setFocusUsername] = useState<string | null>(null);
  const [focusNonce, setFocusNonce] = useState(0);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const searchButtonRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const ownerMenuRef = useRef<HTMLDivElement>(null);
  const logoutItemRef = useRef<HTMLButtonElement>(null);

  useFocusTrap(drawerRef, drawerOpen, menuButtonRef);

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
    if (id !== "postgres") {
      setFocusDatabase(null);
    }
    if (id !== "redis") {
      setFocusUsername(null);
    }
    setSection(id);
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

  function openSearch() {
    setDrawerOpen(false);
    setOwnerOpen(false);
    setSearchOpen(true);
  }

  const nav = <PrimaryNav section={section} onSelect={go} />;

  return (
    <div className="app-shell">
      <div className="shell-content" inert={drawerOpen || searchOpen}>
        <aside className="app-sidebar" aria-label="Redgres">
          <div className="brand">
            <div className="service-rail" aria-hidden="true">
              <span className="service-rail-postgres" />
              <span className="service-rail-redis" />
            </div>
            <p className="brand-name">Redgres</p>
          </div>
          {nav}
          <p className="sidebar-footer">
            Redgres
            <span className="not-connected">
              <span className="warning-mark" aria-hidden="true">
                !
              </span>
              Not connected
            </span>
          </p>
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
            <SectionPage
              section={section}
              focusDatabase={focusDatabase}
              focusUsername={focusUsername}
              focusNonce={focusNonce}
            />
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
        restoreFocusRef={searchButtonRef}
      />
    </div>
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
