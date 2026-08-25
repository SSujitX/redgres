import { FormEvent, useId, useRef, useState } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type DuplicateDatabaseFormProps = {
  source: string;
  connectionCount: number;
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (database: string, owner: string) => void;
};

const identifierPattern = /^[A-Za-z_][A-Za-z0-9_]*$/;
const maxIdentifierLength = 63;

function isValidIdentifier(name: string): boolean {
  return name.length > 0 && name.length <= maxIdentifierLength && identifierPattern.test(name);
}

function suggestedOwner(database: string): string {
  return database !== "" && identifierPattern.test(database) ? `app_${database}` : "";
}

export default function DuplicateDatabaseForm({
  source,
  connectionCount,
  error,
  submitting,
  onCancel,
  onSubmit,
}: DuplicateDatabaseFormProps) {
  const [database, setDatabase] = useState("");
  const [owner, setOwner] = useState("");
  const ownerEdited = useRef(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const helperId = useId();
  const warningId = useId();
  useFocusTrap(dialogRef, true);

  const canSubmit =
    isValidIdentifier(database) && isValidIdentifier(owner) && database !== source && !submitting;

  function handleDatabase(value: string) {
    setDatabase(value);
    if (!ownerEdited.current) {
      setOwner(suggestedOwner(value));
    }
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!canSubmit) {
      return;
    }
    onSubmit(database, owner);
  }

  return (
    <div className="search-backdrop">
      <div
        ref={dialogRef}
        className="search-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-busy={submitting}
      >
        <h2 id={titleId}>Duplicate database</h2>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor="postgres-duplicate-database">New database name</label>
            <input
              id="postgres-duplicate-database"
              name="database"
              autoComplete="off"
              value={database}
              onChange={(event) => handleDatabase(event.target.value)}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? `${helperId} ${warningId} ${errorId}` : `${helperId} ${warningId}`}
            />
            <label htmlFor="postgres-duplicate-owner">Project user</label>
            <input
              id="postgres-duplicate-owner"
              name="owner"
              autoComplete="off"
              value={owner}
              onChange={(event) => {
                ownerEdited.current = true;
                setOwner(event.target.value);
              }}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? `${helperId} ${warningId} ${errorId}` : `${helperId} ${warningId}`}
            />
          </div>
          <p id={helperId} className="muted-copy">
            Redgres generates the password and saves it in the encrypted vault.
          </p>
          <p id={warningId} className="form-warning">
            A unique project user is required. {connectionCount} active connections to {displayText(source)} will be
            terminated. Object owners inside the copy change. Source ownership is verified unchanged.
          </p>
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={!canSubmit}>
              Duplicate
            </button>
            <button type="button" className="text-button" onClick={onCancel} disabled={submitting}>
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
