import { FormEvent, useEffect, useId, useRef, useState } from "react";
import { changePassword, errorMessage } from "../../api/auth";
import { formatRetryAfter } from "./LoginPage";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type ChangePasswordDialogProps = {
  csrf: string;
  onClose: () => void;
  onSuccess: () => void;
};

const wrongCurrent = "Current password is incorrect.";
const unavailable = "Password change is unavailable. Try again.";

export default function ChangePasswordDialog({ csrf, onClose, onSuccess }: ChangePasswordDialogProps) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [fieldError, setFieldError] = useState("");
  const [retryAfter, setRetryAfter] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  const fieldErrorId = useId();

  useFocusTrap(dialogRef, true);

  // Clear secrets on unmount so they never linger in component state.
  useEffect(() => {
    return () => {
      setCurrent("");
      setNext("");
      setConfirm("");
    };
  }, []);

  function close() {
    setCurrent("");
    setNext("");
    setConfirm("");
    setError("");
    setFieldError("");
    setRetryAfter(null);
    onClose();
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setFieldError("");
    setRetryAfter(null);
    if (next !== confirm) {
      setFieldError("New passwords do not match.");
      return;
    }
    setSubmitting(true);
    try {
      const result = await changePassword(csrf, current, next);
      if (result.status === 200) {
        setCurrent("");
        setNext("");
        setConfirm("");
        onSuccess();
        return;
      }
      if (result.status === 403 && result.body.error?.code === "reauth_required") {
        setError(wrongCurrent);
      } else if (result.status === 422 && result.body.error?.fields?.new_password === "too_long") {
        setFieldError("New password is too long.");
      } else if (result.status === 422) {
        setFieldError("New password is too weak (use at least 15 characters, not equal to the username).");
      } else if (result.status === 429) {
        setRetryAfter(result.retryAfter);
        setError(errorMessage(result.body, "Too many attempts. Try again later."));
      } else {
        setError(errorMessage(result.body, unavailable));
      }
    } catch {
      setError(unavailable);
    } finally {
      setSubmitting(false);
    }
  }

  const retry = formatRetryAfter(retryAfter);

  return (
    <div className="search-backdrop" onClick={close}>
      <div
        ref={dialogRef}
        className="search-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-busy={submitting}
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id={titleId}>Change password</h2>
        <p>You will sign in again with the new password.</p>
        <form onSubmit={handleSubmit}>
          <div className="field-stack">
            <label htmlFor="change-current-password">Current password</label>
            <input
              id="change-current-password"
              name="current_password"
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={(event) => setCurrent(event.target.value)}
              required
              disabled={submitting}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
            />
          </div>
          <div className="field-stack">
            <label htmlFor="change-new-password">New password</label>
            <input
              id="change-new-password"
              name="new_password"
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={(event) => setNext(event.target.value)}
              required
              minLength={15}
              disabled={submitting}
              aria-invalid={fieldError ? true : undefined}
              aria-describedby={fieldError ? fieldErrorId : undefined}
            />
          </div>
          <div className="field-stack">
            <label htmlFor="change-confirm-password">Confirm new password</label>
            <input
              id="change-confirm-password"
              name="confirm_password"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              required
              disabled={submitting}
              aria-invalid={fieldError ? true : undefined}
              aria-describedby={fieldError ? fieldErrorId : undefined}
            />
          </div>
          {fieldError ? (
            <p id={fieldErrorId} className="form-warning" role="alert">
              {fieldError}
            </p>
          ) : null}
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          {retry ? (
            <p className="form-warning" role="status">
              {retry.text}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="button" className="text-button" onClick={close} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" className="primary-button" disabled={submitting || Boolean(retry?.disable)}>
              Change password
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
