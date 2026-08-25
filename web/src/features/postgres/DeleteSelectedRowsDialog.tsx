import { FormEvent, useId, useRef } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type DeleteSelectedRowsDialogProps = {
  schema: string;
  table: string;
  selectedCount: number;
  confirmation: string;
  password: string;
  error: string;
  submitting: boolean;
  onConfirmationChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export default function DeleteSelectedRowsDialog({
  schema,
  table,
  selectedCount,
  confirmation,
  password,
  error,
  submitting,
  onConfirmationChange,
  onPasswordChange,
  onCancel,
  onConfirm,
}: DeleteSelectedRowsDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const confirmId = useId();
  const passwordId = useId();
  useFocusTrap(dialogRef, true);

  const canSubmit = confirmation === table && password.length > 0 && !submitting;
  const qualifiedName = displayText(`${schema}.${table}`);
  const rowWord = selectedCount === 1 ? "row" : "rows";

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (confirmation !== table || password.length === 0 || submitting) {
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
        <h2 id={titleId}>Delete selected rows</h2>
        <p>
          Type the exact table name and owner password. This deletes {selectedCount} selected {rowWord} from{" "}
          <span className="identifier bidi-isolate">{qualifiedName}</span>. Cannot be undone.
        </p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor={confirmId}>Confirm table name</label>
            <input
              id={confirmId}
              name="table_confirmation"
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
              Delete
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
