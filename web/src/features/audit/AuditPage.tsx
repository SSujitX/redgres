import { useEffect, useRef, useState } from "react";
import { errorMessage, fetchAuditEvents, type AuditEvent } from "../../api/audit";
import { displayText } from "../../text/displayText";

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

const sessionExpired = "Your session has expired. Sign in again to continue.";
const badCursorRecovery = "This audit page could not be loaded. Return to the newest events.";
const storageUnavailable = "Control-plane storage is unavailable";
const auditUnavailable = "Audit history is unavailable. Try again.";

export default function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[] | null>(null);
  const [error, setError] = useState("");
  const [badCursor, setBadCursor] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextCursor, setNextCursor] = useState("");
  const [cursors, setCursors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    abortRef.current = controller;
    void loadPage(undefined, controller);
    return () => {
      controller.abort();
    };
  }, []);

  async function loadPage(cursor: string | undefined, controller: AbortController) {
    setLoading(true);
    setError("");
    setEvents(null);
    try {
      const result = await fetchAuditEvents(cursor, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.events)) {
        const more = result.body.has_more === true;
        const next = typeof result.body.next_cursor === "string" ? result.body.next_cursor : "";
        if (more && next === "") {
          setHasMore(false);
          setNextCursor("");
          setBadCursor(false);
          setError(auditUnavailable);
          return;
        }
        setEvents(result.body.events);
        setHasMore(more);
        setNextCursor(more ? next : "");
        setBadCursor(false);
        setError("");
        return;
      }
      setHasMore(false);
      setNextCursor("");
      if (result.status === 401) {
        setCursors([]);
        setBadCursor(false);
        setError(sessionExpired);
        return;
      }
      if (result.status === 400) {
        setBadCursor(true);
        setError(badCursorRecovery);
        return;
      }
      if (result.status === 503) {
        setBadCursor(false);
        setError(errorMessage(result.body, storageUnavailable));
        return;
      }
      setBadCursor(false);
      setError(errorMessage(result.body, auditUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setHasMore(false);
      setNextCursor("");
      setBadCursor(false);
      setError(auditUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false);
      }
    }
  }

  function startLoad(cursor: string | undefined) {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    void loadPage(cursor, controller);
  }

  function goNewest() {
    setCursors([]);
    setBadCursor(false);
    startLoad(undefined);
  }

  function goOlder() {
    if (!hasMore || nextCursor === "") {
      return;
    }
    const next = nextCursor;
    setCursors((current) => [...current, next]);
    startLoad(next);
  }

  function goNewer() {
    const stack = cursors.slice(0, -1);
    setCursors(stack);
    startLoad(stack.length > 0 ? stack[stack.length - 1] : undefined);
  }

  const newestEnabled = cursors.length > 0 || badCursor;
  const newerEnabled = !loading && !error && cursors.length > 0;
  const olderEnabled = !loading && !error && hasMore && nextCursor !== "";

  return (
    <article className="audit-page">
      <header className="page-header">
        <h1>Audit</h1>
        <p>Security-relevant events, newest first. One page at a time.</p>
        <p>
          Source address is the address Redgres observed on the connection. Behind Cloudflare Tunnel
          this is the tunnel connector, not the browser&apos;s public address.
        </p>
      </header>
      <div className="row-pager">
        <button type="button" className="text-button" onClick={goNewest}>
          Refresh
        </button>
        <button type="button" className="text-button" disabled={!newestEnabled} onClick={goNewest}>
          Newest
        </button>
        <button type="button" className="text-button" disabled={!newerEnabled} onClick={goNewer}>
          Newer
        </button>
        <button type="button" className="text-button" disabled={!olderEnabled} onClick={goOlder}>
          Older
        </button>
      </div>
      {error ? (
        <p className="form-warning" role="alert">
          {error}
        </p>
      ) : loading && events === null ? (
        <p className="muted-copy" role="status">
          Loading audit events.
        </p>
      ) : events !== null && events.length === 0 ? (
        <p className="muted-copy" role="status">
          No audit events.
        </p>
      ) : events !== null && events.length > 0 ? (
        <AuditResults events={events} />
      ) : null}
    </article>
  );
}

function AuditResults({ events }: { events: AuditEvent[] }) {
  return (
    <div className="audit-results">
      <div className="audit-grid-wrap">
        <table className="audit-grid">
          <caption className="visually-hidden">Audit events</caption>
          <thead>
            <tr>
              <th scope="col">When</th>
              <th scope="col">Actor</th>
              <th scope="col">Action</th>
              <th scope="col">Target</th>
              <th scope="col">Outcome</th>
              <th scope="col">Request ID</th>
              <th scope="col">Source address</th>
            </tr>
          </thead>
          <tbody>
            {events.map((event) => (
              <tr key={event.id}>
                <td>
                  <WhenStamp value={event.created_at ?? ""} />
                </td>
                <td>
                  <RecordedText value={event.actor ?? ""} identifier />
                </td>
                <td>
                  <IsolatedText value={event.action ?? ""} />
                </td>
                <td>
                  <RecordedText value={event.target ?? ""} identifier />
                </td>
                <td>
                  <IsolatedText value={event.outcome ?? ""} />
                </td>
                <td>
                  <IsolatedText value={event.request_id ?? ""} identifier />
                </td>
                <td>
                  <RecordedText value={event.client_ip ?? ""} identifier />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ol className="audit-stack" aria-label="Audit events">
        {events.map((event) => (
          <li key={event.id} className="audit-stack-item">
            <dl>
              <div>
                <dt>When</dt>
                <dd>
                  <WhenStamp value={event.created_at ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Actor</dt>
                <dd>
                  <RecordedText value={event.actor ?? ""} identifier />
                </dd>
              </div>
              <div>
                <dt>Action</dt>
                <dd>
                  <IsolatedText value={event.action ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Target</dt>
                <dd>
                  <RecordedText value={event.target ?? ""} identifier />
                </dd>
              </div>
              <div>
                <dt>Outcome</dt>
                <dd>
                  <IsolatedText value={event.outcome ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Request ID</dt>
                <dd>
                  <IsolatedText value={event.request_id ?? ""} identifier />
                </dd>
              </div>
              <div>
                <dt>Source address</dt>
                <dd>
                  <RecordedText value={event.client_ip ?? ""} identifier />
                </dd>
              </div>
            </dl>
          </li>
        ))}
      </ol>
    </div>
  );
}

function IsolatedText({ value, identifier }: { value: string; identifier?: boolean }) {
  return <span className={identifier ? "bidi-isolate identifier" : "bidi-isolate"}>{displayText(value)}</span>;
}

function RecordedText({ value, identifier }: { value: string; identifier?: boolean }) {
  if (value === "") {
    return (
      <span className={identifier ? "bidi-isolate identifier" : "bidi-isolate"}>
        —<span className="visually-hidden"> Not recorded</span>
      </span>
    );
  }
  return <IsolatedText value={value} identifier={identifier} />;
}

function WhenStamp({ value }: { value: string }) {
  const shown = displayText(value);
  return (
    <time className="bidi-isolate identifier" dateTime={shown}>
      {shown} UTC
    </time>
  );
}
