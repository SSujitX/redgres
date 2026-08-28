import { useEffect, useRef, useState } from "react";
import { errorMessage, fetchStatus, isStatusPayload, type StatusComponent } from "../../api/status";

type CardSpec = {
  id: string;
  title: string;
  rail?: "postgres" | "redis";
};

const CARDS: CardSpec[] = [
  { id: "redgres_state", title: "Redgres state" },
  { id: "postgres_direct", title: "PostgreSQL direct", rail: "postgres" },
  { id: "pgbouncer", title: "PgBouncer" },
  { id: "redis", title: "Redis", rail: "redis" },
  { id: "tool_links", title: "Tool links" },
];

type Tone = "ok" | "unavailable" | "warning";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const statusUnavailable = "Component status is unavailable. Try again.";

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function presentation(state: string | undefined): { text: string; tone: Tone } {
  switch (state) {
    case "ok":
      return { text: "Reachable", tone: "ok" };
    case "unavailable":
      return { text: "Unavailable", tone: "unavailable" };
    case "not_configured":
      return { text: "Not configured", tone: "warning" };
    case "not_implemented":
      return { text: "Not connected", tone: "warning" };
    default:
      return { text: "Unavailable", tone: "unavailable" };
  }
}

function indexById(components: StatusComponent[]): Map<string, StatusComponent> {
  const out = new Map<string, StatusComponent>();
  for (const item of components) {
    out.set(item.id, item);
  }
  return out;
}

export default function SystemPage() {
  const [components, setComponents] = useState<StatusComponent[] | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    abortRef.current = controller;
    void load(controller);
    return () => {
      controller.abort();
    };
  }, []);

  async function load(controller: AbortController) {
    setLoading(true);
    setError("");
    setComponents(null);
    try {
      const result = await fetchStatus({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && isStatusPayload(result.body)) {
        setComponents(result.body.components);
        setError("");
        return;
      }
      if (result.status === 401) {
        setError(sessionExpired);
        return;
      }
      setError(errorMessage(result.body, statusUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setError(statusUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false);
      }
    }
  }

  function refresh() {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    void load(controller);
  }

  const byId = components ? indexById(components) : new Map<string, StatusComponent>();
  const showCards = !error && components !== null;

  return (
    <article>
      <header className="page-header">
        <h1>System</h1>
        <p>Component status for Redgres, PostgreSQL, PgBouncer, Redis, and tool links.</p>
        <button type="button" className="text-button" onClick={refresh}>
          Refresh
        </button>
      </header>
      {error ? (
        <p className="form-warning" role="alert">
          {error}
        </p>
      ) : null}
      {loading && components === null && !error ? (
        <p className="muted-copy" role="status">
          Loading component status.
        </p>
      ) : null}
      {showCards ? (
        <ul className="status-cards" aria-busy={loading ? "true" : "false"}>
          {CARDS.map((card) => {
            const found = byId.get(card.id);
            const { text, tone } = presentation(found?.state);
            const className = ["status-card"];
            if (card.rail === "postgres") {
              className.push("status-card-postgres");
            }
            if (card.rail === "redis") {
              className.push("status-card-redis");
            }
            const statusClass =
              tone === "ok" ? "status-ok" : tone === "unavailable" ? "status-unavailable" : "not-connected";
            return (
              <li key={card.id} className={className.join(" ")} aria-label={`${card.title}: ${text}`}>
                <h2>{card.title}</h2>
                <p className={statusClass}>
                  {tone !== "ok" ? (
                    <span className="warning-mark" aria-hidden="true">
                      !
                    </span>
                  ) : null}
                  {text}
                </p>
              </li>
            );
          })}
        </ul>
      ) : null}
    </article>
  );
}
