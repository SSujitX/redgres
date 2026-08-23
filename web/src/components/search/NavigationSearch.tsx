import { useEffect, useId, useRef, useState, type RefObject } from "react";
import { filterNav, type SectionId } from "../../nav";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type NavigationSearchProps = {
  open: boolean;
  onClose: () => void;
  onSelect: (id: SectionId) => void;
  restoreFocusRef: RefObject<HTMLButtonElement | null>;
};

export default function NavigationSearch({
  open,
  onClose,
  onSelect,
  restoreFocusRef,
}: NavigationSearchProps) {
  const [query, setQuery] = useState("");
  const inputId = useId();
  const countId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);

  useFocusTrap(dialogRef, open, restoreFocusRef);

  useEffect(() => {
    if (!open) {
      setQuery("");
    }
  }, [open]);

  if (!open) {
    return null;
  }

  const results = filterNav(query);
  const trimmed = query.trim();
  const countText =
    trimmed === ""
      ? "Type at least one character to filter navigation."
      : results.length === 0
        ? "No matching pages."
        : results.length === 1
          ? "1 matching page."
          : `${results.length} matching pages.`;

  return (
    <div className="search-backdrop" role="presentation" onClick={onClose}>
      <div
        ref={dialogRef}
        className="search-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${inputId}-label`}
        onClick={(event) => event.stopPropagation()}
      >
        <p id={`${inputId}-label`} className="visually-hidden">
          Search navigation
        </p>
        <input
          id={inputId}
          className="search-input"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search pages"
          aria-label="Search pages"
          aria-describedby={countId}
        />
        <p id={countId} className="search-hint" role="status" aria-live="polite">
          {trimmed === ""
            ? countText
            : results.length === 0
              ? "No matching pages. Try Overview or PostgreSQL."
              : countText}
        </p>
        {trimmed !== "" && results.length > 0 ? (
          <ul className="search-results">
            {results.map((entry) => (
              <li key={entry.id}>
                <button
                  type="button"
                  className={entry.service ? `nav-result nav-result-${entry.service}` : "nav-result"}
                  onClick={() => {
                    onSelect(entry.id);
                    onClose();
                  }}
                >
                  <span>{entry.label}</span>
                  <span className="nav-result-group">{entry.group}</span>
                </button>
              </li>
            ))}
          </ul>
        ) : null}
        <button type="button" className="text-button" onClick={onClose}>
          Close search
        </button>
      </div>
    </div>
  );
}
