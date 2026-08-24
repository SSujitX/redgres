import { useEffect, useRef, useState } from "react";
import { fetchRedisStatus, type RedisStatusMetrics, type RedisStatusPayload } from "../../api/redis";
import { errorMessage, fetchStatus, type StatusComponent } from "../../api/status";
import { displayText } from "../../text/displayText";

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

type RedisDetail =
  | { kind: "none" }
  | { kind: "not_configured" }
  | { kind: "ok"; metrics: RedisStatusMetrics }
  | { kind: "degraded"; reasonCopy: string | null };

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

function redisReasonCopy(reason: string | undefined): string | null {
  switch (reason) {
    case "auth_failed":
      return "Authentication failed";
    case "permission_denied":
      return "Permission denied";
    case "unreachable":
      return "Unreachable";
    default:
      return null;
  }
}

function finiteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function parseRedisMetrics(raw: unknown): RedisStatusMetrics | null {
  if (raw == null || typeof raw !== "object") {
    return null;
  }
  const candidate = raw as Record<string, unknown>;
  if (typeof candidate.version !== "string") {
    return null;
  }
  if (
    !finiteNumber(candidate.uptime_seconds) ||
    !finiteNumber(candidate.connected_clients) ||
    !finiteNumber(candidate.used_memory_bytes) ||
    !finiteNumber(candidate.max_memory_bytes) ||
    !finiteNumber(candidate.ops_per_sec) ||
    !finiteNumber(candidate.db_size) ||
    !finiteNumber(candidate.latency_ms)
  ) {
    return null;
  }
  return {
    version: candidate.version,
    uptime_seconds: candidate.uptime_seconds,
    connected_clients: candidate.connected_clients,
    used_memory_bytes: candidate.used_memory_bytes,
    max_memory_bytes: candidate.max_memory_bytes,
    ops_per_sec: candidate.ops_per_sec,
    db_size: candidate.db_size,
    latency_ms: candidate.latency_ms,
  };
}

function parseRedisDetail(payload: RedisStatusPayload): RedisDetail {
  if (payload.state === "not_configured") {
    return { kind: "not_configured" };
  }
  if (payload.state === "unavailable") {
    return { kind: "degraded", reasonCopy: redisReasonCopy(payload.reason) };
  }
  if (payload.state === "ok") {
    const metrics = parseRedisMetrics(payload.metrics);
    if (metrics) {
      return { kind: "ok", metrics };
    }
  }
  return { kind: "degraded", reasonCopy: null };
}

function formatUptime(seconds: number): string {
  const total = Math.floor(seconds);
  if (total < 60) {
    return `${total}s`;
  }
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m ${secs}s`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1048576) {
    return `${(bytes / 1024).toFixed(1)} KiB`;
  }
  if (bytes < 1073741824) {
    return `${(bytes / 1048576).toFixed(1)} MiB`;
  }
  return `${(bytes / 1073741824).toFixed(1)} GiB`;
}

function formatMemory(used: number, max: number): string {
  const usedText = formatBytes(used);
  const maxText = max === 0 ? "Unlimited" : formatBytes(max);
  return `${usedText} / ${maxText}`;
}

function indexById(components: StatusComponent[]): Map<string, StatusComponent> {
  const out = new Map<string, StatusComponent>();
  for (const item of components) {
    if (typeof item.id === "string" && item.id !== "") {
      out.set(item.id, item);
    }
  }
  return out;
}

function RedisMetrics({
  headlineTone,
  detail,
}: {
  headlineTone: Tone;
  detail: RedisDetail;
}) {
  if (detail.kind === "none" || detail.kind === "not_configured") {
    return null;
  }
  if (detail.kind === "ok") {
    const { metrics } = detail;
    return (
      <dl className="redis-metrics">
        <div>
          <dt>Version</dt>
          <dd className="bidi-isolate identifier">{displayText(metrics.version)}</dd>
        </div>
        <div>
          <dt>Uptime</dt>
          <dd className="metric">{formatUptime(metrics.uptime_seconds)}</dd>
        </div>
        <div>
          <dt>Clients</dt>
          <dd className="metric">{String(metrics.connected_clients)}</dd>
        </div>
        <div>
          <dt>Used / max memory</dt>
          <dd className="metric">{formatMemory(metrics.used_memory_bytes, metrics.max_memory_bytes)}</dd>
        </div>
        <div>
          <dt>Ops/s</dt>
          <dd className="metric">{String(metrics.ops_per_sec)}</dd>
        </div>
        <div>
          <dt>DB size</dt>
          <dd className="metric">{String(metrics.db_size)}</dd>
        </div>
        <div>
          <dt>Latency</dt>
          <dd className="metric">{`${metrics.latency_ms} ms`}</dd>
        </div>
      </dl>
    );
  }
  const showUnavailableLead = headlineTone === "ok" || detail.reasonCopy == null;
  return (
    <div className="redis-metrics-note">
      {showUnavailableLead ? <p className="not-connected">Metrics unavailable</p> : null}
      {detail.reasonCopy ? <p className="not-connected">{detail.reasonCopy}</p> : null}
    </div>
  );
}

export default function OverviewPage() {
  const [components, setComponents] = useState<StatusComponent[] | null>(null);
  const [redisDetail, setRedisDetail] = useState<RedisDetail>({ kind: "none" });
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
    setRedisDetail({ kind: "none" });
    const statusOutcome = fetchStatus({ signal: controller.signal }).then(
      (result) => ({ kind: "ok" as const, result }),
      (err: unknown) => ({ kind: "throw" as const, err }),
    );
    const redisOutcome = fetchRedisStatus({ signal: controller.signal }).then(
      (result) => ({ kind: "ok" as const, result }),
      (err: unknown) => ({ kind: "throw" as const, err }),
    );
    try {
      const [status, redis] = await Promise.all([statusOutcome, redisOutcome]);
      if (controller.signal.aborted) {
        return;
      }
      if (status.kind === "throw") {
        if (isAbortError(status.err)) {
          return;
        }
        setError(statusUnavailable);
        return;
      }
      if (status.result.status === 200 && Array.isArray(status.result.body.components)) {
        setComponents(status.result.body.components);
        setError("");
        if (redis.kind === "throw") {
          if (isAbortError(redis.err)) {
            return;
          }
          setRedisDetail({ kind: "degraded", reasonCopy: null });
          return;
        }
        if (redis.result.status === 200 && typeof redis.result.body.state === "string") {
          setRedisDetail(parseRedisDetail(redis.result.body));
          return;
        }
        setRedisDetail({ kind: "degraded", reasonCopy: null });
        return;
      }
      if (status.result.status === 401) {
        setError(sessionExpired);
        return;
      }
      setError(errorMessage(status.result.body, statusUnavailable));
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
        <h1>Overview</h1>
        <p>Independent component status.</p>
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
                {card.id === "redis" ? <RedisMetrics headlineTone={tone} detail={redisDetail} /> : null}
              </li>
            );
          })}
        </ul>
      ) : null}
    </article>
  );
}
