import { FormEvent, useEffect, useId, useRef, useState } from "react";
import { errorMessage, fetchRedisCommands, type RedisAclPreset, type RedisAclQueueKind } from "../../api/redis";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";

type EditPermissionsDialogProps = {
  username: string;
  keyPattern: string;
  preset: RedisAclPreset;
  queueKind: RedisAclQueueKind;
  inspectCommands: string[];
  error: string;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (
    keyPattern: string,
    preset: RedisAclPreset,
    queueKind?: RedisAclQueueKind,
    commands?: string[],
  ) => void;
};

const PRESETS: { value: RedisAclPreset; label: string }[] = [
  { value: "cache-read-write", label: "Cache read/write" },
  { value: "read-only", label: "Read only" },
  { value: "queue-worker", label: "Queue/worker" },
  { value: "custom", label: "Custom" },
];

const QUEUE_KINDS: { value: RedisAclQueueKind; label: string }[] = [
  { value: "lists", label: "Lists" },
  { value: "streams", label: "Streams" },
  { value: "sorted-sets", label: "Sorted sets" },
];

const sessionExpired = "Your session has expired. Sign in again to continue.";
const redisUnavailable = "Redis is unavailable.";

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function parseCommandCatalog(value: unknown): string[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  return value.filter((item): item is string => typeof item === "string" && item !== "");
}

function prefillSelection(inspectCommands: string[], catalog: string[]): Set<string> {
  const allowed = new Set(catalog);
  return new Set(inspectCommands.filter((command) => allowed.has(command)));
}

export default function EditPermissionsDialog({
  username,
  keyPattern: initialKeyPattern,
  preset: initialPreset,
  queueKind: initialQueueKind,
  inspectCommands,
  error,
  submitting,
  onCancel,
  onSubmit,
}: EditPermissionsDialogProps) {
  const [keyPattern, setKeyPattern] = useState(initialKeyPattern);
  const [preset, setPreset] = useState<RedisAclPreset>(initialPreset);
  const [queueKind, setQueueKind] = useState<RedisAclQueueKind>(initialQueueKind);
  const [catalog, setCatalog] = useState<string[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [catalogError, setCatalogError] = useState("");
  const [catalogLoading, setCatalogLoading] = useState(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const errorId = useId();
  useFocusTrap(dialogRef, true);

  useEffect(() => {
    if (preset !== "custom") {
      return;
    }
    const controller = new AbortController();
    setCatalog(null);
    setSelected(new Set());
    setCatalogError("");
    setCatalogLoading(true);
    fetchRedisCommands({ signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 401) {
          setCatalogError(sessionExpired);
          setCatalog(null);
          return;
        }
        if (result.status === 200) {
          const parsed = parseCommandCatalog(result.body.commands);
          if (parsed) {
            setCatalog(parsed);
            setSelected(prefillSelection(inspectCommands, parsed));
            setCatalogError("");
            return;
          }
        }
        setCatalogError(errorMessage(result.body, redisUnavailable));
        setCatalog(null);
      })
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setCatalogError(redisUnavailable);
        setCatalog(null);
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setCatalogLoading(false);
        }
      });
    return () => {
      controller.abort();
    };
  }, [preset, inspectCommands]);

  const shownError = error || catalogError;
  const selectedCommands = catalog ? catalog.filter((command) => selected.has(command)) : [];
  const saveDisabled =
    submitting ||
    (preset === "custom" && (catalogLoading || catalog === null || catalogError !== "" || selectedCommands.length === 0));

  function toggleCommand(command: string, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(command);
      } else {
        next.delete(command);
      }
      return next;
    });
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (preset === "custom") {
      if (saveDisabled) {
        return;
      }
      onSubmit(keyPattern, preset, undefined, selectedCommands);
      return;
    }
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
        aria-busy={submitting || catalogLoading}
      >
        <h2 id={titleId}>Edit permissions</h2>
        <p>
          <span className="muted-copy">Username </span>
          <span className="bidi-isolate identifier">{displayText(username)}</span>
        </p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
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
          </div>
          {preset === "custom" && catalogLoading ? (
            <p className="muted-copy" role="status">
              Loading commands.
            </p>
          ) : null}
          {preset === "custom" && catalog ? (
            <fieldset className="command-checklist">
              <legend>Commands</legend>
              {catalog.map((command) => {
                const id = `acl-edit-cmd-${command}`;
                return (
                  <label key={command} htmlFor={id}>
                    <input
                      id={id}
                      type="checkbox"
                      name="commands"
                      value={command}
                      checked={selected.has(command)}
                      onChange={(event) => toggleCommand(command, event.target.checked)}
                      disabled={submitting}
                    />
                    <span className="bidi-isolate identifier">{displayText(command)}</span>
                  </label>
                );
              })}
            </fieldset>
          ) : null}
          {shownError ? (
            <p id={errorId} className="form-error" role="alert">
              {shownError}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={saveDisabled}>
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
