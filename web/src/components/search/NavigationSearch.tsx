import { useEffect, useId, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { errorMessage, fetchSearch, type SearchGroup } from "../../api/search";
import { filterNav, type SectionId } from "../../nav";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type NavigationSearchProps = {
  open: boolean;
  onClose: () => void;
  onSelect: (id: SectionId) => void;
  onSelectDatabase: (name: string) => void;
  restoreFocusRef: RefObject<HTMLButtonElement | null>;
};

const debounceMs = 200;
const maxQueryRunes = 128;
const sessionExpired = "Your session has expired. Sign in again to continue.";
const searchUnavailable = "Search is unavailable. Navigation results are still shown.";
const redisCopy = "Redis ACL user search is not available yet.";

function queryRuneCount(value: string): number {
  return Array.from(value).length;
}

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function groupById(groups: SearchGroup[] | null, id: string): SearchGroup | undefined {
  return groups?.find((group) => group.id === id);
}

function resultCountText(pages: number, databases: number, emptyQuery: boolean, tooLong: boolean): string {
  if (emptyQuery) {
    return "Type at least one character to filter navigation.";
  }
  if (tooLong) {
    return "Query is too long.";
  }
  const total = pages + databases;
  if (total === 0) {
    return "No matching pages. Try Overview or PostgreSQL.";
  }
  if (databases === 0) {
    return pages === 1 ? "1 matching page." : `${pages} matching pages.`;
  }
  if (pages === 0) {
    return databases === 1 ? "1 matching database." : `${databases} matching databases.`;
  }
  return total === 1 ? "1 matching result." : `${total} matching results.`;
}

export default function NavigationSearch({
  open,
  onClose,
  onSelect,
  onSelectDatabase,
  restoreFocusRef,
}: NavigationSearchProps) {
  const [query, setQuery] = useState("");
  const [groups, setGroups] = useState<SearchGroup[] | null>(null);
  const [resourceError, setResourceError] = useState("");
  const [searching, setSearching] = useState(false);
  const inputId = useId();
  const countId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useFocusTrap(dialogRef, open, restoreFocusRef);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setGroups(null);
      setResourceError("");
      setSearching(false);
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const trimmed = query.trim();
    const runes = queryRuneCount(trimmed);
    setGroups(null);
    setResourceError("");
    if (runes < 1 || runes > maxQueryRunes) {
      setSearching(false);
      return;
    }
    setSearching(true);
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      fetchSearch(trimmed, { signal: controller.signal })
        .then((result) => {
          if (controller.signal.aborted) {
            return;
          }
          if (result.status === 200 && Array.isArray(result.body.groups)) {
            setGroups(result.body.groups);
            setResourceError("");
            return;
          }
          setGroups(null);
          if (result.status === 401) {
            setResourceError(sessionExpired);
            return;
          }
          setResourceError(errorMessage(result.body, searchUnavailable));
        })
        .catch((err) => {
          if (controller.signal.aborted || isAbortError(err)) {
            return;
          }
          setGroups(null);
          setResourceError(searchUnavailable);
        })
        .finally(() => {
          if (!controller.signal.aborted) {
            setSearching(false);
          }
        });
    }, debounceMs);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [open, query]);

  if (!open) {
    return null;
  }

  const trimmed = query.trim();
  const runes = queryRuneCount(trimmed);
  const tooLong = runes > maxQueryRunes;
  const navResults = tooLong ? [] : filterNav(query).filter((entry) => entry.group !== "Documentation");
  const docResults = tooLong ? [] : filterNav(query).filter((entry) => entry.group === "Documentation");
  const postgres = groupById(groups, "postgres_databases");
  const postgresHits = (postgres?.hits ?? []).filter(
    (hit) => hit.type === "postgres_database" && typeof hit.label === "string" && hit.label !== "",
  );
  const pageCount = navResults.length + docResults.length;
  const countText = resultCountText(pageCount, postgresHits.length, trimmed === "", tooLong);
  const statusText =
    searching && pageCount + postgresHits.length === 0
      ? "Searching."
      : searching
        ? `Searching. ${countText}`
        : countText;

  function activateDatabase(name: string) {
    onSelectDatabase(name);
    onClose();
  }

  function activateSection(id: SectionId) {
    onSelect(id);
    onClose();
  }

  function onDialogKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
      return;
    }
    const root = dialogRef.current;
    if (!root) {
      return;
    }
    const results = Array.from(root.querySelectorAll<HTMLButtonElement>("[data-search-result]"));
    if (results.length === 0) {
      return;
    }
    const current = document.activeElement;
    const index = results.indexOf(current as HTMLButtonElement);
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (index === -1) {
        results[0].focus();
        return;
      }
      results[Math.min(index + 1, results.length - 1)].focus();
      return;
    }
    event.preventDefault();
    if (index <= 0) {
      inputRef.current?.focus();
      return;
    }
    results[index - 1].focus();
  }

  return (
    <div className="search-backdrop" role="presentation" onClick={onClose}>
      <div
        ref={dialogRef}
        className="search-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${inputId}-label`}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={onDialogKeyDown}
      >
        <p id={`${inputId}-label`} className="visually-hidden">
          Search
        </p>
        <input
          ref={inputRef}
          id={inputId}
          className="search-input"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search pages and databases"
          aria-label="Search pages and databases"
          aria-describedby={countId}
          aria-invalid={tooLong || undefined}
        />
        <p id={countId} className="search-hint" role="status" aria-live="polite">
          {statusText}
        </p>
        {resourceError ? (
          <p className="form-warning" role="alert">
            {resourceError}
          </p>
        ) : null}
        {trimmed !== "" && !tooLong ? (
          <div className="search-groups">
            <section className="search-group" aria-label="PostgreSQL databases">
              <p className="nav-group-label">PostgreSQL databases</p>
              {postgres?.status === "unavailable" ? (
                <p className="form-warning">Unavailable</p>
              ) : null}
              {postgres?.status === "not_configured" ? (
                <p className="not-connected">
                  <span className="warning-mark" aria-hidden="true">
                    !
                  </span>
                  Not configured
                </p>
              ) : null}
              {postgres?.truncated ? <p className="search-hint">Results truncated.</p> : null}
              {postgresHits.length > 0 ? (
                <ul className="search-results">
                  {postgresHits.map((hit) => {
                    const name = hit.label ?? "";
                    return (
                      <li key={hit.id ?? name}>
                        <button
                          type="button"
                          className="nav-result nav-result-postgres"
                          data-search-result=""
                          onClick={() => activateDatabase(name)}
                        >
                          <span className="bidi-isolate identifier">{displayText(name)}</span>
                          <span className="nav-result-group">PostgreSQL databases</span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              ) : null}
            </section>
            <section className="search-group" aria-label="Redis ACL users">
              <p className="nav-group-label">Redis ACL users</p>
              <p className="not-connected">
                <span className="warning-mark" aria-hidden="true">
                  !
                </span>
                Not connected. {redisCopy}
              </p>
            </section>
            <section className="search-group" aria-label="Navigation">
              <p className="nav-group-label">Navigation</p>
              {navResults.length > 0 ? (
                <ul className="search-results">
                  {navResults.map((entry) => (
                    <li key={entry.id}>
                      <button
                        type="button"
                        className={entry.service ? `nav-result nav-result-${entry.service}` : "nav-result"}
                        data-search-result=""
                        onClick={() => activateSection(entry.id)}
                      >
                        <span>{entry.label}</span>
                        <span className="nav-result-group">{entry.group}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </section>
            <section className="search-group" aria-label="Documentation">
              <p className="nav-group-label">Documentation</p>
              {docResults.length > 0 ? (
                <ul className="search-results">
                  {docResults.map((entry) => (
                    <li key={entry.id}>
                      <button
                        type="button"
                        className="nav-result"
                        data-search-result=""
                        onClick={() => activateSection(entry.id)}
                      >
                        <span>{entry.label}</span>
                        <span className="nav-result-group">{entry.group}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </section>
          </div>
        ) : null}
        <button type="button" className="text-button" onClick={onClose}>
          Close search
        </button>
      </div>
    </div>
  );
}
