import { useId, useRef } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type RotatePasswordDialogProps = {
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
};

export default function RotatePasswordDialog({ error, submitting, onCancel, onConfirm }: RotatePasswordDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  useFocusTrap(dialogRef, true);

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
          This issues a new password. The previous credential stops working immediately and cannot be recovered.
        </p>
        {error ? (
          <p id={errorId} className="form-error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="form-actions">
          <button type="button" className="text-button" onClick={onCancel} disabled={submitting}>
            Cancel
          </button>
          <button type="button" className="primary-button" onClick={onConfirm} disabled={submitting}>
            Rotate now
          </button>
        </div>
      </div>
    </div>
  );
}
