import { Fragment, useEffect, useState } from "react";
import { fetchRedisPresets, type RedisAclPreset, type RedisAclQueueKind } from "../../api/redis";
import { displayText } from "../../text/displayText";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const presetsUnavailable = "Permission presets are unavailable.";

const PRESET_LABELS: { value: Exclude<RedisAclPreset, "custom">; label: string }[] = [
  { value: "cache-read-write", label: "Cache read/write" },
  { value: "read-only", label: "Read only" },
  { value: "queue-worker", label: "Queue/worker" },
];

const QUEUE_LABELS: { value: RedisAclQueueKind; label: string }[] = [
  { value: "lists", label: "Lists" },
  { value: "streams", label: "Streams" },
  { value: "sorted-sets", label: "Sorted sets" },
];

type CatalogRow = {
  preset: Exclude<RedisAclPreset, "custom">;
  label: string;
  queueKind?: RedisAclQueueKind;
  queueLabel?: string;
  commands: string[];
};

type View =
  | { kind: "loading" }
  | { kind: "session_expired" }
  | { kind: "unavailable" }
  | { kind: "ok"; rows: CatalogRow[] };

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value == null || typeof value !== "object") {
    return null;
  }
  return value as Record<string, unknown>;
}

function namedPreset(value: unknown): Exclude<RedisAclPreset, "custom"> | null {
  if (value === "cache-read-write" || value === "read-only" || value === "queue-worker") {
    return value;
  }
  return null;
}

function namedQueueKind(value: unknown): RedisAclQueueKind | undefined {
  if (value === "lists" || value === "streams" || value === "sorted-sets") {
    return value;
  }
  return undefined;
}

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

function parseCatalog(raw: unknown): CatalogRow[] | null {
  if (!Array.isArray(raw)) {
    return null;
  }
  const rows: CatalogRow[] = [];
  for (const item of raw) {
    const record = asRecord(item);
    if (!record) {
      continue;
    }
    const preset = namedPreset(record.preset);
    if (!preset) {
      continue;
    }
    const label = PRESET_LABELS.find((option) => option.value === preset)?.label ?? preset;
    const queueKind = preset === "queue-worker" ? namedQueueKind(record.queue_kind) : undefined;
    const queueLabel = queueKind
      ? QUEUE_LABELS.find((option) => option.value === queueKind)?.label
      : undefined;
    rows.push({
      preset,
      label,
      queueKind,
      queueLabel,
      commands: stringList(record.commands),
    });
  }
  return rows;
}

function IsolatedId({ value }: { value: string }) {
  return <span className="bidi-isolate identifier">{displayText(value)}</span>;
}

function CommandList({ values }: { values: string[] }) {
  if (values.length === 0) {
    return <p className="muted-copy">No commands.</p>;
  }
  return (
    <ul className="rule-list">
      {values.map((value, index) => (
        <li key={`${value}-${index}`}>
          <IsolatedId value={value} />
        </li>
      ))}
    </ul>
  );
}

function CatalogSections({ rows }: { rows: CatalogRow[] }) {
  const named = rows.filter((row) => row.preset !== "queue-worker");
  const queue = rows.filter((row) => row.preset === "queue-worker");
  const queueHeading = queue[0];
  return (
    <>
      {named.map((row, index) => (
        <section key={`${row.preset}-${index}`}>
          <h2>{row.label}</h2>
          <CommandList values={row.commands} />
        </section>
      ))}
      {queueHeading ? (
        <section className="preset-queue">
          <h2>{queueHeading.label}</h2>
          {queue.map((row, index) => (
            <Fragment key={`${row.preset}-${row.queueKind ?? ""}-${index}`}>
              {row.queueLabel ? <h3>{row.queueLabel}</h3> : null}
              <CommandList values={row.commands} />
            </Fragment>
          ))}
        </section>
      ) : null}
    </>
  );
}

export default function PresetsPage() {
  const [view, setView] = useState<View>({ kind: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    fetchRedisPresets({ signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 401) {
          setView({ kind: "session_expired" });
          return;
        }
        if (result.status === 200) {
          const rows = parseCatalog(result.body.presets);
          if (rows && rows.length > 0) {
            setView({ kind: "ok", rows });
            return;
          }
        }
        setView({ kind: "unavailable" });
      })
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setView({ kind: "unavailable" });
      });
    return () => {
      controller.abort();
    };
  }, []);

  const alertCopy =
    view.kind === "session_expired" ? sessionExpired : view.kind === "unavailable" ? presetsUnavailable : "";

  return (
    <article>
      <header className="page-header page-header-redis">
        <h1>Permission presets</h1>
        <p>Named Redis command sets used when creating or editing ACL users. Custom is not listed here.</p>
      </header>
      {alertCopy ? (
        <p className="form-warning" role="alert">
          {alertCopy}
        </p>
      ) : null}
      {view.kind === "loading" ? (
        <p className="muted-copy" role="status">
          Loading presets.
        </p>
      ) : null}
      {view.kind === "ok" ? <CatalogSections rows={view.rows} /> : null}
    </article>
  );
}
