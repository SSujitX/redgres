import { FormEvent, useId, useRef, useState } from "react";
import type { RedisAclPreset, RedisAclQueueKind } from "../../api/redis";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type EditPermissionsDialogProps = {
  username: string;
  keyPattern: string;
  preset: RedisAclPreset;
  queueKind: RedisAclQueueKind;
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (keyPattern: string, preset: RedisAclPreset, queueKind?: RedisAclQueueKind) => void;
};

const PRESETS: { value: RedisAclPreset; label: string }[] = [
  { value: "cache-read-write", label: "Cache read/write" },
  { value: "read-only", label: "Read only" },
  { value: "queue-worker", label: "Queue/worker" },
];

const QUEUE_KINDS: { value: RedisAclQueueKind; label: string }[] = [
  { value: "lists", label: "Lists" },
  { value: "streams", label: "Streams" },
  { value: "sorted-sets", label: "Sorted sets" },
];

export default function EditPermissionsDialog({
  username,
  keyPattern: initialKeyPattern,
  preset: initialPreset,
  queueKind: initialQueueKind,
  error,
  submitting,
  onCancel,
  onSubmit,
}: EditPermissionsDialogProps) {
  const [keyPattern, setKeyPattern] = useState(initialKeyPattern);
  const [preset, setPreset] = useState<RedisAclPreset>(initialPreset);
  const [queueKind, setQueueKind] = useState<RedisAclQueueKind>(initialQueueKind);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  useFocusTrap(dialogRef, true);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (preset === "queue-worker") {
      onSubmit(keyPattern, preset, queueKind);
      return;
    }
    onSubmit(keyPattern, preset);
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
        <h2 id={titleId}>Edit permissions</h2>
        <p>
          <span className="muted-copy">Username </span>
          <span className="bidi-isolate identifier">{displayText(username)}</span>
        </p>
        <form className="field-stack" onSubmit={handleSubmit} autoComplete="off">
          <label htmlFor="acl-edit-key-prefix">Key prefix</label>
          <input
            id="acl-edit-key-prefix"
            name="key_pattern"
            autoComplete="off"
            value={keyPattern}
            onChange={(event) => setKeyPattern(event.target.value)}
            required
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
          />
          <label htmlFor="acl-edit-preset">Permission preset</label>
          <select
            id="acl-edit-preset"
            name="preset"
            value={preset}
            onChange={(event) => setPreset(event.target.value as RedisAclPreset)}
          >
            {PRESETS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          {preset === "queue-worker" ? (
            <>
              <label htmlFor="acl-edit-queue-type">Queue type</label>
              <select
                id="acl-edit-queue-type"
                name="queue_kind"
                value={queueKind}
                onChange={(event) => setQueueKind(event.target.value as RedisAclQueueKind)}
              >
                {QUEUE_KINDS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </>
          ) : null}
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={submitting}>
              Save
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
