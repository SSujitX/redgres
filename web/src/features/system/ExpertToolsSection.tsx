import { useEffect, useRef, useState } from "react";
import { fetchAuditEvents, type AuditEvent } from "../../api/audit";
import { errorMessage } from "../../api/client";
import type { ToolLinks } from "../../api/auth";
import {
  isLaunchURL,
  launchExpertTool,
  revealPgAdminCredentials,
  type ExpertTool,
} from "../../api/tools";
import { useFocusTrap } from "../../hooks/useFocusTrap";
import { displayText } from "../../text/displayText";
import CredentialTicket, { type ShownCredential } from "../redis/CredentialTicket";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const openFailed = "Could not open the expert tool.";
const revealFailed = "pgAdmin login is not configured.";
const popupBlocked = "Allow pop-ups for this site, then try Open again.";
const activityUnavailable = "Tool activity is unavailable.";
const activityPollMs = 8000;
const toolActions = new Set(["tools.launch", "tools.pgadmin.reveal"]);

type ExpertToolsSectionProps = {
  csrf: string;
  toolLinks: ToolLinks;
  variant: "full" | "compact";
  refreshNonce?: number;
};

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
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
  if (value === "") {
    return <RecordedText value="" identifier />;
  }
  const shown = displayText(value);
  return (
    <time className="bidi-isolate identifier" dateTime={shown}>
      {shown} UTC
    </time>
  );
}

function toolEvents(events: AuditEvent[]): AuditEvent[] {
  return events.filter((event) => typeof event.action === "string" && toolActions.has(event.action));
}

export default function ExpertToolsSection({
  csrf,
  toolLinks,
  variant,
  refreshNonce = 0,
}: ExpertToolsSectionProps) {
  const [busy, setBusy] = useState<"" | ExpertTool | "reveal">("");
  const [error, setError] = useState("");
  const [ticket, setTicket] = useState<ShownCredential | null>(null);
  const [activity, setActivity] = useState<AuditEvent[] | null>(null);
  const [activityError, setActivityError] = useState("");
  const abortRef = useRef<AbortController | null>(null);
  const activityAbort = useRef<AbortController | null>(null);
  const activityInFlight = useRef(false);
  const activityNeedsRefresh = useRef(false);
  const ticketRef = useRef<HTMLDivElement | null>(null);
  const revealRef = useRef<HTMLButtonElement | null>(null);

  const hasPgAdmin = Boolean(toolLinks.pgadmin);
  const hasRedisInsight = Boolean(toolLinks.redisinsight);
  const configured = hasPgAdmin || hasRedisInsight;

  async function loadActivity(controller: AbortController) {
    if (activityInFlight.current) {
      activityNeedsRefresh.current = true;
      return;
    }
    activityInFlight.current = true;
    try {
      const result = await fetchAuditEvents({ limit: 8 }, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setTicket(null);
        setActivity(null);
        setActivityError(sessionExpired);
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.events)) {
        setActivity(toolEvents(result.body.events));
        setActivityError("");
        return;
      }
      setActivityError(errorMessage(result.body, activityUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setActivityError(activityUnavailable);
    } finally {
      activityInFlight.current = false;
      if (activityNeedsRefresh.current && !controller.signal.aborted) {
        activityNeedsRefresh.current = false;
        void loadActivity(controller);
      }
    }
  }

  useFocusTrap(ticketRef, ticket !== null, revealRef);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      activityAbort.current?.abort();
      setTicket(null);
    };
  }, []);

  useEffect(() => {
    if (variant !== "full") {
      return;
    }
    activityAbort.current?.abort();
    const controller = new AbortController();
    activityAbort.current = controller;
    void loadActivity(controller);

    const tick = () => {
      if (document.visibilityState !== "visible" || controller.signal.aborted) {
        return;
      }
      void loadActivity(controller);
    };
    const interval = window.setInterval(tick, activityPollMs);
    const onVisibility = () => {
      if (document.visibilityState === "visible") {
        tick();
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisibility);
      controller.abort();
    };
  }, [variant, refreshNonce]);

  async function openTool(tool: ExpertTool) {
    if (busy) {
      return;
    }
    setError("");
    const popup = window.open("about:blank", "_blank");
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setBusy(tool);
    try {
      const result = await launchExpertTool(tool, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        popup?.close();
        return;
      }
      if (result.status === 401) {
        popup?.close();
        setTicket(null);
        setError(sessionExpired);
        return;
      }
      if (result.status === 200 && isLaunchURL(result.body.launch_url)) {
        if (popup && !popup.closed) {
          popup.location.replace(result.body.launch_url);
          if (variant === "full" && activityAbort.current && !activityAbort.current.signal.aborted) {
            void loadActivity(activityAbort.current);
          }
          return;
        }
        setError(popupBlocked);
        return;
      }
      popup?.close();
      setError(errorMessage(result.body, result.status === 404 ? "Expert tool is not configured" : openFailed));
    } catch (err) {
      popup?.close();
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setError(openFailed);
    } finally {
      if (!controller.signal.aborted) {
        setBusy("");
      }
    }
  }

  async function revealLogin() {
    if (busy || ticket) {
      return;
    }
    setError("");
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setBusy("reveal");
    try {
      const result = await revealPgAdminCredentials(csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setTicket(null);
        setError(sessionExpired);
        return;
      }
      const email = result.body.email;
      const password = result.body.password;
      if (result.status === 200 && typeof email === "string" && email !== "" && typeof password === "string" && password !== "") {
        setTicket({ username: email, password });
        if (activityAbort.current && !activityAbort.current.signal.aborted) {
          void loadActivity(activityAbort.current);
        }
        return;
      }
      setError(errorMessage(result.body, revealFailed));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setError(revealFailed);
    } finally {
      if (!controller.signal.aborted) {
        setBusy("");
      }
    }
  }

  if (variant === "compact") {
    if (!configured) {
      return null;
    }
    return (
      <div className="expert-tools-compact">
        {hasPgAdmin ? (
          <button
            type="button"
            className="text-button"
            disabled={busy !== ""}
            onClick={() => void openTool("pgadmin")}
          >
            {busy === "pgadmin" ? "Opening pgAdmin" : "Open pgAdmin"}
          </button>
        ) : null}
        {hasRedisInsight ? (
          <button
            type="button"
            className="text-button"
            disabled={busy !== ""}
            onClick={() => void openTool("redisinsight")}
          >
            {busy === "redisinsight" ? "Opening RedisInsight" : "Open RedisInsight"}
          </button>
        ) : null}
        {error ? (
          <p className="form-warning" role="alert">
            {error}
          </p>
        ) : null}
      </div>
    );
  }

  return (
    <section className="expert-tools panel" aria-labelledby="expert-tools-heading">
      <h2 id="expert-tools-heading">Expert tools</h2>
      <p className="muted-copy">
        Open pgAdmin and RedisInsight from this signed-in console. Cloudflare Access still asks for email on those
        hostnames. Direct tool URLs without this launch do not get a Redgres session.
      </p>
      {!configured ? (
        <p className="muted-copy">pgAdmin and RedisInsight are not configured on this host.</p>
      ) : (
        <div className="expert-tools-grid">
          {hasPgAdmin ? (
            <article className="expert-tool-card panel-sub">
              <h3>pgAdmin</h3>
              <p>PostgreSQL browser. Reveal the saved login, then Open. The password stays on the server until you dismiss the ticket.</p>
              <div className="expert-tool-actions">
                <button
                  type="button"
                  className="primary-button"
                  disabled={busy !== "" || ticket !== null}
                  onClick={() => void openTool("pgadmin")}
                >
                  {busy === "pgadmin" ? "Opening pgAdmin" : "Open pgAdmin"}
                </button>
                <button
                  ref={revealRef}
                  type="button"
                  className="text-button"
                  disabled={busy !== "" || ticket !== null}
                  onClick={() => void revealLogin()}
                >
                  {busy === "reveal" ? "Revealing login" : "Reveal pgAdmin login"}
                </button>
              </div>
            </article>
          ) : null}
          {hasRedisInsight ? (
            <article className="expert-tool-card panel-sub">
              <h3>RedisInsight</h3>
              <p>
                Redis data explorer. Open it from here, then add the Redis host in RedisInsight. Redgres does not store
                a RedisInsight password.
              </p>
              <div className="expert-tool-actions">
                <button
                  type="button"
                  className="primary-button"
                  disabled={busy !== "" || ticket !== null}
                  onClick={() => void openTool("redisinsight")}
                >
                  {busy === "redisinsight" ? "Opening RedisInsight" : "Open RedisInsight"}
                </button>
              </div>
            </article>
          ) : null}
        </div>
      )}
      {error ? (
        <p className="form-warning" role="alert">
          {error}
        </p>
      ) : null}
      {ticket ? (
        <div ref={ticketRef}>
          <CredentialTicket kind="pgadmin" credential={ticket} onDismiss={() => setTicket(null)} />
        </div>
      ) : null}
      <div className="expert-tool-activity">
        <h3>Recent tool activity</h3>
        <p className="muted-copy">
          Launch and reveal events from the existing audit log. Passwords, tickets, and request IDs are not shown.
        </p>
        {activityError ? (
          <p className="form-warning" role="alert">
            {activityError}
          </p>
        ) : null}
        {activity === null && !activityError ? (
          <p className="muted-copy" role="status">
            Loading tool activity.
          </p>
        ) : null}
        {activity !== null && activity.length === 0 && !activityError ? (
          <p className="muted-copy">No tool events in the latest audit page.</p>
        ) : null}
        {activity !== null && activity.length > 0 ? (
          <ol className="overview-recent-audit-list">
            {activity.map((event) => (
              <li key={event.id} className="overview-recent-audit-item">
                <dl>
                  <div>
                    <dt>When</dt>
                    <dd>
                      <WhenStamp value={event.created_at ?? ""} />
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
                </dl>
              </li>
            ))}
          </ol>
        ) : null}
      </div>
    </section>
  );
}
