import { useEffect, useRef, useState } from "react";
import {
  createRedisUser,
  disableRedisUser,
  enableRedisUser,
  errorMessage,
  fetchRedisUser,
  fetchRedisUsers,
  rotateRedisUser,
  type RedisAclPreset,
  type RedisAclQueueKind,
  type RedisAclUserListItem,
} from "../../api/redis";
import { displayText } from "../../text/displayText";
import CreateAclUserForm from "./CreateAclUserForm";
import CredentialTicket, { type ShownCredential } from "./CredentialTicket";
import RotatePasswordDialog from "./RotatePasswordDialog";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const notConfigured = "Redis is not configured.";
const emptyUsers = "No ACL users.";
const truncatedCopy = "ACL user list truncated.";
const notFound = "ACL user not found.";
const redisUnavailable = "Redis is unavailable.";
const protectedCopy = "This Redis user is protected";
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

function parseCredential(raw: unknown): ShownCredential | null {
  const record = asRecord(raw);
  if (!record) {
    return null;
  }
  const username = stringField(record, "username");
  const password = stringField(record, "password");
  if (username === "" || password === "") {
    return null;
  }
  const urls = asRecord(record.urls);
  const url = urls ? stringField(urls, "primary") : "";
  if (url === "") {
    return { username, password };
  }
  return { username, password, url };
}

type AclUsersPageProps = {
  csrf: string;
  focusUsername?: string | null;
  focusNonce?: number;
};

export default function AclUsersPage({ csrf, focusUsername = null, focusNonce = 0 }: AclUsersPageProps) {
  const [list, setList] = useState<ListView>({ kind: "loading" });
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<DetailUser | null>(null);
  const [detailError, setDetailError] = useState("");
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [ticket, setTicket] = useState<ShownCredential | null>(null);
  const [pendingInspect, setPendingInspect] = useState<string | null>(null);
  const [toggling, setToggling] = useState(false);
  const [actionError, setActionError] = useState("");
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [rotateError, setRotateError] = useState("");
  const selectionAbort = useRef<AbortController | null>(null);
  const listAbort = useRef<AbortController | null>(null);
  const toggleAbort = useRef<AbortController | null>(null);
  const rotateAbort = useRef<AbortController | null>(null);
  const inspectorRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (selected) {
      inspectorRef.current?.focus();
    }
  }, [selected]);

  function applyListResult(
    result: Awaited<ReturnType<typeof fetchRedisUsers>>,
    controller: AbortController,
  ) {
    if (controller.signal.aborted) {
      return;
    }
    if (result.status === 401) {
      clearTicket();
      setCreateOpen(false);
      setRotateOpen(false);
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
  }

  function requestList() {
    listAbort.current?.abort();
    const controller = new AbortController();
    listAbort.current = controller;
    fetchRedisUsers({ signal: controller.signal })
      .then((result) => applyListResult(result, controller))
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setList({ kind: "error", copy: redisUnavailable });
      });
  }

  useEffect(() => {
    requestList();
    return () => {
      listAbort.current?.abort();
      selectionAbort.current?.abort();
      toggleAbort.current?.abort();
      rotateAbort.current?.abort();
    };
  }, []);

  function clearTicket() {
    setTicket(null);
    setPendingInspect(null);
  }

  function openDetails(username: string) {
    clearTicket();
    selectionAbort.current?.abort();
    toggleAbort.current?.abort();
    rotateAbort.current?.abort();
    const controller = new AbortController();
    selectionAbort.current = controller;
    setSelected(username);
    setDetail(null);
    setDetailError("");
    setActionError("");
    setToggling(false);
    setRotateOpen(false);
    setRotating(false);
    setRotateError("");
    setLoadingDetail(true);
    void loadDetail(username, controller);
  }

  useEffect(() => {
    if (!focusUsername) {
      return;
    }
    openDetails(focusUsername);
  }, [focusUsername, focusNonce]);

  function clearSelection() {
    selectionAbort.current?.abort();
    toggleAbort.current?.abort();
    rotateAbort.current?.abort();
    setSelected(null);
    setDetail(null);
    setDetailError("");
    setActionError("");
    setToggling(false);
    setRotateOpen(false);
    setRotating(false);
    setRotateError("");
    setLoadingDetail(false);
  }

  async function loadDetail(username: string, controller: AbortController) {
    try {
      const result = await fetchRedisUser(username, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        clearTicket();
        setCreateOpen(false);
        setRotateOpen(false);
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
      if (result.status === 200 && result.body.state === "unavailable") {
        setDetail(null);
        setDetailError(unavailableCopy(result.body.reason));
        return;
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

  function dismissTicket() {
    const name = pendingInspect;
    setTicket(null);
    setPendingInspect(null);
    if (name) {
      openDetails(name);
      return;
    }
    if (selected) {
      const username = selected;
      requestList();
      selectionAbort.current?.abort();
      const controller = new AbortController();
      selectionAbort.current = controller;
      setLoadingDetail(true);
      setActionError("");
      void loadDetail(username, controller);
    }
  }

  async function handleCreate(
    username: string,
    keyPattern: string,
    preset: RedisAclPreset,
    queueKind?: RedisAclQueueKind,
  ) {
    setCreating(true);
    setCreateError("");
    try {
      const result = await createRedisUser(username, keyPattern, csrf, { preset, queueKind });
      if (result.status === 401) {
        clearTicket();
        setCreateOpen(false);
        setRotateOpen(false);
        setList({ kind: "session_expired" });
        return;
      }
      if (result.status === 201) {
        const shown = parseCredential(result.body.credential);
        if (!shown) {
          setCreateError(errorMessage(result.body, redisUnavailable));
          return;
        }
        setCreateOpen(false);
        setCreateError("");
        setTicket(shown);
        setPendingInspect(shown.username);
        requestList();
        return;
      }
      setCreateError(errorMessage(result.body, redisUnavailable));
    } catch {
      setCreateError(redisUnavailable);
    } finally {
      setCreating(false);
    }
  }

  async function handleToggleEnabled() {
    if (!detail || detail.protected || list.kind !== "ok" || toggling) {
      return;
    }
    const username = detail.username;
    const enable = !detail.enabled;
    setToggling(true);
    setActionError("");
    toggleAbort.current?.abort();
    const controller = new AbortController();
    toggleAbort.current = controller;
    try {
      const result = enable
        ? await enableRedisUser(username, csrf, { signal: controller.signal })
        : await disableRedisUser(username, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        clearTicket();
        setCreateOpen(false);
        setRotateOpen(false);
        setList({ kind: "session_expired" });
        return;
      }
      if (result.status === 404) {
        setDetail(null);
        setDetailError(errorMessage(result.body, notFound));
        return;
      }
      if (result.status === 200) {
        const parsed = parseDetailUser(result.body.user);
        if (parsed && parsed.username === username) {
          setDetail(parsed);
          setList((current) => {
            if (current.kind !== "ok") {
              return current;
            }
            return {
              ...current,
              users: current.users.map((item) =>
                item.username === parsed.username ? { ...item, enabled: parsed.enabled } : item,
              ),
            };
          });
          return;
        }
      }
      setActionError(errorMessage(result.body, redisUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setActionError(redisUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setToggling(false);
      }
    }
  }

  async function handleRotate() {
    if (!detail || detail.protected || list.kind !== "ok" || rotating || ticket) {
      return;
    }
    const username = detail.username;
    setRotating(true);
    setRotateError("");
    setActionError("");
    rotateAbort.current?.abort();
    const controller = new AbortController();
    rotateAbort.current = controller;
    try {
      const result = await rotateRedisUser(username, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        clearTicket();
        setCreateOpen(false);
        setRotateOpen(false);
        setList({ kind: "session_expired" });
        return;
      }
      if (result.status === 404) {
        setRotateOpen(false);
        setDetail(null);
        setDetailError(errorMessage(result.body, notFound));
        return;
      }
      if (result.status === 403) {
        setRotateError(errorMessage(result.body, protectedCopy));
        return;
      }
      if (result.status === 200) {
        const shown = parseCredential(result.body.credential);
        if (!shown) {
          setRotateError(errorMessage(result.body, redisUnavailable));
          return;
        }
        setRotateOpen(false);
        setRotateError("");
        setTicket(shown);
        return;
      }
      setRotateError(errorMessage(result.body, redisUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setRotateError(redisUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setRotating(false);
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

  const canCreate = list.kind === "ok" && ticket === null;

  return (
    <article>
      <header className="page-header page-header-redis">
        <div className="page-header-row">
          <h1>ACL users</h1>
          {canCreate ? (
            <button type="button" className="primary-button" onClick={() => setCreateOpen(true)}>
              Create ACL user
            </button>
          ) : null}
        </div>
        <p>Create an ACL user with a named permission preset and a project key prefix, inspect modeled rules, or rotate a non-protected password. Delete is not available in this slice.</p>
      </header>
      {ticket ? <CredentialTicket credential={ticket} onDismiss={dismissTicket} /> : null}
      {createOpen ? (
        <CreateAclUserForm
          error={createError}
          submitting={creating}
          onCancel={() => {
            setCreateOpen(false);
            setCreateError("");
          }}
          onSubmit={(username, keyPattern, preset, queueKind) =>
            void handleCreate(username, keyPattern, preset, queueKind)
          }
        />
      ) : null}
      {rotateOpen ? (
        <RotatePasswordDialog
          error={rotateError}
          submitting={rotating}
          onCancel={() => {
            if (rotating) {
              return;
            }
            setRotateOpen(false);
            setRotateError("");
          }}
          onConfirm={() => void handleRotate()}
        />
      ) : null}
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
      {list.kind === "ok" && list.truncated ? (
        <p className="form-warning" role="alert">
          {truncatedCopy}
        </p>
      ) : null}
      {list.kind === "ok" && list.users.length > 0 ? (
        <>
          {selected ? (
            <p>
              <button type="button" className="text-button" onClick={clearSelection}>
                Back to users
              </button>
            </p>
          ) : null}
          <ul className={selected ? "ledger-list ledger-list-inspecting" : "ledger-list"}>
          {list.users.map((item) => (
            <li key={item.username} className={selected === item.username ? "is-selected" : undefined}>
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
                    {item.protected ? <span className="ledger-badge">Protected</span> : null}
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
        </>
      ) : null}
      {selected ? (
        <section
          ref={inspectorRef}
          className="detail-panel detail-panel-redis"
          aria-label="ACL user details"
          aria-busy={loadingDetail}
          tabIndex={-1}
        >
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
          {actionError ? (
            <p className="form-warning" role="alert">
              {actionError}
            </p>
          ) : null}
          {detail ? <InspectorFacts user={detail} /> : null}
          {detail && !loadingDetail && list.kind === "ok" && !detail.protected ? (
            <div className="form-actions">
              <button
                type="button"
                className="text-button"
                disabled={toggling}
                onClick={() => void handleToggleEnabled()}
              >
                {detail.enabled ? "Disable" : "Enable"}
              </button>
              <button
                type="button"
                className="text-button"
                disabled={rotating || ticket !== null}
                onClick={() => {
                  if (rotating || ticket) {
                    return;
                  }
                  setRotateError("");
                  setRotateOpen(true);
                }}
              >
                Rotate
              </button>
            </div>
          ) : null}
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
