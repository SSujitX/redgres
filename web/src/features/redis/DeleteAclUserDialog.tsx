import { FormEvent, useId, useRef } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type DeleteAclUserDialogProps = {
  username: string;
  confirmation: string;
  password: string;
  error: string;
  submitting: boolean;
  onConfirmationChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export default function DeleteAclUserDialog({
  username,
  confirmation,
  password,
  error,
  submitting,
  onConfirmationChange,
  onPasswordChange,
  onCancel,
  onConfirm,
}: DeleteAclUserDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const confirmId = useId();
  const passwordId = useId();
  useFocusTrap(dialogRef, true);

  const canSubmit = confirmation === username && password.length > 0 && !submitting;

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (confirmation !== username || password.length === 0 || submitting) {
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
        <h2 id={titleId}>Delete Redis user</h2>
        <p>
          Type the exact username and owner password. This removes the ACL user. Existing Redis connections for that
          user are terminated. Keys are not deleted. This cannot be undone.
        </p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor={confirmId}>Confirm username</label>
            <input
              id={confirmId}
              name="username_confirmation"
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
