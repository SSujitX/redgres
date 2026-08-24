import { useEffect, useRef, useState } from "react";
import { errorMessage, fetchRedisUser, fetchRedisUsers, type RedisAclUserListItem } from "../../api/redis";
import { displayText } from "../../text/displayText";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const notConfigured = "Redis is not configured.";
const emptyUsers = "No ACL users.";
const truncatedCopy = "ACL user list truncated.";
const notFound = "ACL user not found.";
const redisUnavailable = "Redis is unavailable.";
const limitedCopy = "Redgres cannot model these rules exactly. They are labeled limited rather than rewritten.";

type ListUser = {
  username: string;
  enabled: boolean;
  key_pattern: string;
  preset: string;
  protected: boolean;
  limited: boolean;
};

type DetailUser = ListUser & {
  queue_kind: string;
  commands: string[];
  categories: string[];
};

type ListView =
  | { kind: "loading" }
  | { kind: "session_expired" }
  | { kind: "not_configured" }
  | { kind: "unavailable"; copy: string }
  | { kind: "error"; copy: string }
  | { kind: "ok"; users: ListUser[]; truncated: boolean };

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function unavailableCopy(reason: string | undefined): string {
  switch (reason) {
    case "auth_failed":
      return "Redis authentication failed.";
    case "permission_denied":
      return "Redis permission denied.";
    case "unreachable":
      return "Redis is unreachable.";
    default:
      return redisUnavailable;
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value == null || typeof value !== "object") {
    return null;
  }
  return value as Record<string, unknown>;
}

function stringField(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

function parseListUser(raw: unknown): ListUser | null {
  const record = asRecord(raw);
  if (!record) {
    return null;
  }
  const username = stringField(record, "username");
  if (username === "") {
    return null;
  }
  return {
    username,
    enabled: record.enabled === true,
    key_pattern: stringField(record, "key_pattern"),
    preset: stringField(record, "preset"),
    protected: record.protected === true,
    limited: record.rule_fidelity === "limited",
  };
}

function parseDetailUser(raw: unknown): DetailUser | null {
  const record = asRecord(raw);
  const base = parseListUser(raw);
  if (!record || !base) {
    return null;
  }
  return {
    ...base,
    queue_kind: stringField(record, "queue_kind"),
    commands: stringList(record.commands),
    categories: stringList(record.categories),
  };
}

function IsolatedId({ value }: { value: string }) {
  return <span className="bidi-isolate identifier">{displayText(value)}</span>;
}

export default function AclUsersPage() {
  const [list, setList] = useState<ListView>({ kind: "loading" });
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<DetailUser | null>(null);
  const [detailError, setDetailError] = useState("");
  const [loadingDetail, setLoadingDetail] = useState(false);
  const selectionAbort = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    fetchRedisUsers({ signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 401) {
          setList({ kind: "session_expired" });
          return;
        }
        if (result.status === 200 && result.body.state === "not_configured") {
          setList({ kind: "not_configured" });
          return;
        }
        if (result.status === 200 && result.body.state === "unavailable") {
          setList({ kind: "unavailable", copy: unavailableCopy(result.body.reason) });
          return;
        }
        if (result.status === 200 && result.body.state === "ok" && Array.isArray(result.body.users)) {
          const users = result.body.users.flatMap((item: RedisAclUserListItem) => {
            const parsed = parseListUser(item);
            return parsed ? [parsed] : [];
          });
          setList({ kind: "ok", users, truncated: result.body.truncated === true });
          return;
        }
        setList({ kind: "error", copy: errorMessage(result.body, redisUnavailable) });
      })
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setList({ kind: "error", copy: redisUnavailable });
      });
    return () => {
      controller.abort();
      selectionAbort.current?.abort();
    };
  }, []);

  function openDetails(username: string) {
    selectionAbort.current?.abort();
    const controller = new AbortController();
    selectionAbort.current = controller;
    setSelected(username);
    setDetail(null);
    setDetailError("");
    setLoadingDetail(true);
    void loadDetail(username, controller);
  }

  async function loadDetail(username: string, controller: AbortController) {
    try {
      const result = await fetchRedisUser(username, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setDetail(null);
        setDetailError(sessionExpired);
        return;
      }
      if (result.status === 404) {
        setDetail(null);
        setDetailError(errorMessage(result.body, notFound));
        return;
      }
      if (result.status === 200 && result.body.state === "ok") {
        const parsed = parseDetailUser(result.body.user);
        if (parsed && parsed.username === username) {
          setDetail(parsed);
          setDetailError("");
          return;
        }
      }
      setDetail(null);
      setDetailError(errorMessage(result.body, redisUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDetail(null);
      setDetailError(redisUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setLoadingDetail(false);
      }
    }
  }

  const listAlert =
    list.kind === "session_expired"
      ? sessionExpired
      : list.kind === "not_configured"
        ? notConfigured
        : list.kind === "unavailable"
          ? list.copy
          : list.kind === "error"
            ? list.copy
            : "";

  return (
    <article>
      <header className="page-header page-header-redis">
        <h1>ACL users</h1>
        <p>Inspect Redis ACL users and modeled rules. Create, rotate, and delete are not available in this slice.</p>
      </header>
      {listAlert ? (
        <p className="form-warning" role="alert">
          {listAlert}
        </p>
      ) : null}
      {list.kind === "loading" ? (
        <p className="muted-copy" role="status">
          Loading ACL users.
        </p>
      ) : null}
      {list.kind === "ok" && list.users.length === 0 ? <p className="muted-copy">{emptyUsers}</p> : null}
      {list.kind === "ok" && list.users.length > 0 ? (
        <ul className="ledger-list">
          {list.users.map((item) => (
            <li key={item.username}>
              <button
                type="button"
                className={
                  selected === item.username
                    ? "ledger-item ledger-item-redis ledger-item-active"
                    : "ledger-item ledger-item-redis"
                }
                aria-current={selected === item.username ? "true" : undefined}
                onClick={() => void openDetails(item.username)}
              >
                <span className="ledger-facts">
                  <span>
                    <span className="muted-copy">Username </span>
                    <IsolatedId value={item.username} />
                    {item.protected ? <span> Protected</span> : null}
                    {item.limited ? <span className="ledger-limited"> Limited</span> : null}
                  </span>
                  <span>
                    <span className="muted-copy">Status </span>
                    <span>{item.enabled ? "Enabled" : "Disabled"}</span>
                  </span>
                  <span>
                    <span className="muted-copy">Preset </span>
                    <IsolatedId value={item.preset} />
                  </span>
                  <span>
                    <span className="muted-copy">Key prefix </span>
                    <IsolatedId value={item.key_pattern} />
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      {list.kind === "ok" && list.truncated ? <p className="form-warning">{truncatedCopy}</p> : null}
      {selected ? (
        <section className="detail-panel detail-panel-redis" aria-label="ACL user details" aria-busy={loadingDetail}>
          <h2>
            <IsolatedId value={selected} />
          </h2>
          {loadingDetail ? (
            <p className="muted-copy" role="status">
              Loading details.
            </p>
          ) : null}
          {detailError ? (
            <p className="form-warning" role="alert">
              {detailError}
            </p>
          ) : null}
          {detail ? <InspectorFacts user={detail} /> : null}
        </section>
      ) : null}
    </article>
  );
}

function InspectorFacts({ user }: { user: DetailUser }) {
  return (
    <>
      <dl className="fact-list">
        <Fact label="Username" value={user.username} identifier />
        <div>
          <dt>Status</dt>
          <dd>{user.enabled ? "Enabled" : "Disabled"}</dd>
        </div>
        <Fact label="Preset" value={user.preset} identifier />
        <Fact label="Key prefix" value={user.key_pattern} identifier />
        {user.queue_kind ? <Fact label="Queue kind" value={user.queue_kind} identifier /> : null}
        <div>
          <dt>Protected</dt>
          <dd>{user.protected ? "Yes" : "No"}</dd>
        </div>
      </dl>
      {user.limited ? <p className="ledger-limited">{limitedCopy}</p> : null}
      <h3>Commands</h3>
      <RuleList values={user.commands} empty="No commands." />
      <h3>Categories</h3>
      <RuleList values={user.categories} empty="No categories." />
    </>
  );
}

function Fact({ label, value, identifier }: { label: string; value: string; identifier?: boolean }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={identifier ? "bidi-isolate identifier" : undefined}>{identifier ? displayText(value) : value}</dd>
    </div>
  );
}

function RuleList({ values, empty }: { values: string[]; empty: string }) {
  if (values.length === 0) {
    return <p className="muted-copy">{empty}</p>;
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
