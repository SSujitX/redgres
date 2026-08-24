import { FormEvent, useId, useRef, useState } from "react";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type CreateAclUserFormProps = {
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (username: string, keyPattern: string) => void;
};

function suggestedKeyPattern(username: string): string {
  if (username === "") {
    return "";
  }
  return username.endsWith(":*") ? username : `${username}:*`;
}

export default function CreateAclUserForm({ error, submitting, onCancel, onSubmit }: CreateAclUserFormProps) {
  const [username, setUsername] = useState("");
  const [keyPattern, setKeyPattern] = useState("");
  const prefixEdited = useRef(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  useFocusTrap(dialogRef, true);

  function handleUsername(value: string) {
    setUsername(value);
    if (!prefixEdited.current) {
      setKeyPattern(suggestedKeyPattern(value));
    }
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    onSubmit(username, keyPattern);
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
        <h2 id={titleId}>Create ACL user</h2>
        <form className="field-stack" onSubmit={handleSubmit} autoComplete="off">
          <label htmlFor="acl-create-username">Username</label>
          <input
            id="acl-create-username"
            name="username"
            autoComplete="off"
            value={username}
            onChange={(event) => handleUsername(event.target.value)}
            required
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
          />
          <label htmlFor="acl-create-key-prefix">Key prefix</label>
          <input
            id="acl-create-key-prefix"
            name="key_pattern"
            autoComplete="off"
            value={keyPattern}
            onChange={(event) => {
              prefixEdited.current = true;
              setKeyPattern(event.target.value);
            }}
            required
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
          />
          <p className="muted-copy">Permission preset</p>
          <p className="readonly-field">Cache read/write</p>
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={submitting}>
              Create
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
