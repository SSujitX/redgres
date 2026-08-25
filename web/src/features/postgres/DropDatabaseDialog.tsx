import { FormEvent, useId, useRef } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type DropDatabaseDialogProps = {
  database: string;
  confirmation: string;
  password: string;
  error: string;
  submitting: boolean;
  onConfirmationChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export default function DropDatabaseDialog({
  database,
  confirmation,
  password,
  error,
  submitting,
  onConfirmationChange,
  onPasswordChange,
  onCancel,
  onConfirm,
}: DropDatabaseDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const confirmId = useId();
  const passwordId = useId();
  useFocusTrap(dialogRef, true);

  const canSubmit = confirmation === database && password.length > 0 && !submitting;
  const databaseName = displayText(database);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (confirmation !== database || password.length === 0 || submitting) {
      return;
    }
    onConfirm();
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
        <h2 id={titleId}>Drop database</h2>
        <p>
          Type the exact database name and owner password. This permanently deletes{" "}
          <span className="identifier bidi-isolate">{databaseName}</span>. Active connections are
          terminated. The project role is removed only if it owns no other database. Cannot be
          undone. Recovery requires a valid external backup.
        </p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor={confirmId}>Confirm database name</label>
            <input
              id={confirmId}
              name="database_confirmation"
              autoComplete="off"
              value={confirmation}
              onChange={(event) => onConfirmationChange(event.target.value)}
              disabled={submitting}
            />
            <label htmlFor={passwordId}>Owner password</label>
            <input
              id={passwordId}
              name="owner_password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => onPasswordChange(event.target.value)}
              disabled={submitting}
            />
          </div>
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="button" className="text-button" onClick={onCancel} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" className="danger-button" disabled={!canSubmit}>
              Confirm Drop
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
