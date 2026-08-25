import { FormEvent, useId, useRef, useState } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type RotatePasswordDialogProps = {
  database: string;
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onConfirm: (confirmation: string) => void;
};

export default function RotatePasswordDialog({
  database,
  error,
  submitting,
  onCancel,
  onConfirm,
}: RotatePasswordDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const confirmId = useId();
  useFocusTrap(dialogRef, true);

  const canSubmit = confirmation === database && !submitting;

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!canSubmit) {
      return;
    }
    onConfirm(confirmation);
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
        <h2 id={titleId}>Rotate password?</h2>
        <p>
          The previous credentials stop working immediately. The new password is saved in the encrypted vault. Update
          every application using this project user.
        </p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor={confirmId}>Confirm database name</label>
            <input
              id={confirmId}
              name="confirmation"
              autoComplete="off"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              disabled={submitting}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
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
            <button type="submit" className="primary-button" disabled={!canSubmit}>
              Rotate now
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
