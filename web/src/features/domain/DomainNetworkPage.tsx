import { FormEvent, useEffect, useId, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  applyDomain,
  confirmDomainReachable,
  disconnectDomain,
  errorMessage,
  fetchDomain,
  setDomainAccessPolicy,
  setDomainToken,
  type DomainApplyPayload,
  type DomainStatusPayload,
} from "../../api/domain";
import { displayText } from "../../text/displayText";
import { useFocusTrap } from "../../hooks/useFocusTrap";

type DomainNetworkPageProps = {
  csrf: string;
};

const sessionExpired = "Your session has expired. Sign in again to continue.";
const domainUnavailable = "Domain status is unavailable. Try again.";
const maxTokenLength = 512;

/** Dashboard permission labels for the per-zone token (OPS-009 token-first apply). */
export const CLOUDFLARE_TOKEN_PERMISSIONS = [
  "Account · Cloudflare Tunnel · Edit",
  "Account · Access: Apps and Policies · Edit",
  "Zone · Zone · Read",
  "Zone · DNS · Edit",
] as const;

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function suggestedHostname(zone: string): string {
  const z = zone.trim().toLowerCase();
  if (z === "" || z.includes(" ") || !z.includes(".")) {
    return "";
  }
  return `redgres.${z}`;
}

function isDomainStatus(body: DomainStatusPayload): body is DomainStatusPayload & { configured: boolean } {
  return typeof body.configured === "boolean";
}

export default function DomainNetworkPage({ csrf }: DomainNetworkPageProps) {
  const [status, setStatus] = useState<DomainStatusPayload | null>(null);
  const [statusError, setStatusError] = useState("");
  const [loading, setLoading] = useState(true);

  const [token, setToken] = useState("");
  const [tokenError, setTokenError] = useState("");
  const [tokenBusy, setTokenBusy] = useState(false);
  const [tokenSaved, setTokenSaved] = useState(false);

  const [zone, setZone] = useState("");
  const [hostname, setHostname] = useState("");
  const hostnameEdited = useRef(false);
  const [applyError, setApplyError] = useState("");
  const [applyBusy, setApplyBusy] = useState(false);
  const [applyResult, setApplyResult] = useState<DomainApplyPayload | null>(null);

  const [disconnectOpen, setDisconnectOpen] = useState(false);
  const [disconnectConfirm, setDisconnectConfirm] = useState("");
  const [disconnectError, setDisconnectError] = useState("");
  const [disconnectBusy, setDisconnectBusy] = useState(false);

  const [allowEmail, setAllowEmail] = useState("");
  const [allowError, setAllowError] = useState("");
  const [allowBusy, setAllowBusy] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmTyped, setConfirmTyped] = useState("");
  const [confirmError, setConfirmError] = useState("");
  const [confirmBusy, setConfirmBusy] = useState(false);

  const abortRef = useRef<AbortController | null>(null);
  const tokenInputRef = useRef<HTMLInputElement | null>(null);
  const disconnectButtonRef = useRef<HTMLButtonElement | null>(null);
  const confirmButtonRef = useRef<HTMLButtonElement | null>(null);

  function clearTokenField() {
    setToken("");
    if (tokenInputRef.current) {
      tokenInputRef.current.value = "";
    }
  }

  useEffect(() => {
    const controller = new AbortController();
    abortRef.current = controller;
    void loadStatus(controller);
    return () => {
      controller.abort();
    };
  }, []);

  async function loadStatus(controller: AbortController) {
    setLoading(true);
    setStatusError("");
    setStatus(null);
    try {
      const result = await fetchDomain({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && isDomainStatus(result.body)) {
        setStatus(result.body);
        if (result.body.configured && typeof result.body.zone === "string") {
          setZone(result.body.zone);
        }
        if (result.body.configured && typeof result.body.hostname === "string") {
          setHostname(result.body.hostname);
          hostnameEdited.current = true;
        }
        return;
      }
      if (result.status === 401) {
        setStatusError(sessionExpired);
        return;
      }
      if (result.status === 403) {
        setStatusError(errorMessage(result.body, "You do not have permission to manage domain settings."));
        return;
      }
      setStatusError(errorMessage(result.body, domainUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setStatusError(domainUnavailable);
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
    void loadStatus(controller);
  }

  function handleZoneChange(value: string) {
    setZone(value);
    if (!hostnameEdited.current) {
      setHostname(suggestedHostname(value));
    }
  }

  async function handleTokenSubmit(event: FormEvent) {
    event.preventDefault();
    const trimmed = token.trim();
    if (trimmed === "" || trimmed.length > maxTokenLength || /\s/.test(trimmed) || tokenBusy) {
      setTokenError("Enter a single Cloudflare API token (no spaces, max 512 characters).");
      return;
    }
    setTokenBusy(true);
    setTokenError("");
    setTokenSaved(false);
    try {
      const result = await setDomainToken(trimmed, csrf);
      clearTokenField();
      if (result.status === 200 && result.body.ok === true) {
        setTokenSaved(true);
        return;
      }
      if (result.status === 401) {
        setTokenError(sessionExpired);
        return;
      }
      setTokenError(errorMessage(result.body, "Cloudflare token could not be stored."));
    } catch {
      clearTokenField();
      setTokenError("Cloudflare token could not be stored.");
    } finally {
      setTokenBusy(false);
    }
  }

  async function handleApply(event: FormEvent) {
    event.preventDefault();
    const z = zone.trim().toLowerCase();
    const h = hostname.trim().toLowerCase();
    if (z === "" || h === "" || applyBusy) {
      setApplyError("Enter both zone and hostname.");
      return;
    }
    setApplyBusy(true);
    setApplyError("");
    setApplyResult(null);
    try {
      const result = await applyDomain(z, h, csrf);
      if (result.status === 200 && typeof result.body.hostname === "string") {
        setApplyResult(result.body);
        refresh();
        return;
      }
      if (result.status === 401) {
        setApplyError(sessionExpired);
        return;
      }
      setApplyError(errorMessage(result.body, "Domain apply failed."));
    } catch {
      setApplyError("Domain apply failed.");
    } finally {
      setApplyBusy(false);
    }
  }

  async function handleDisconnect() {
    if (disconnectBusy) {
      return;
    }
    const expected = typeof status?.hostname === "string" ? status.hostname : "";
    if (disconnectConfirm !== expected) {
      setDisconnectError("Type the exact hostname to confirm disconnect.");
      return;
    }
    setDisconnectBusy(true);
    setDisconnectError("");
    try {
      const result = await disconnectDomain(csrf);
      if (result.status === 200 && result.body.ok === true) {
        setDisconnectOpen(false);
        setDisconnectConfirm("");
        setApplyResult(null);
        setAllowEmail("");
        hostnameEdited.current = false;
        setZone("");
        setHostname("");
        refresh();
        return;
      }
      if (result.status === 401) {
        setDisconnectError(sessionExpired);
        return;
      }
      setDisconnectError(errorMessage(result.body, "Disconnect failed."));
    } catch {
      setDisconnectError("Disconnect failed.");
    } finally {
      setDisconnectBusy(false);
    }
  }

  async function handleAccessAllow(event: FormEvent) {
    event.preventDefault();
    const email = allowEmail.trim();
    if (email === "" || allowBusy) {
      setAllowError("Enter an email allowed by Cloudflare Access.");
      return;
    }
    setAllowBusy(true);
    setAllowError("");
    try {
      const result = await setDomainAccessPolicy([email], csrf);
      setAllowEmail("");
      if (result.status === 200 && result.body.ok === true) {
        refresh();
        return;
      }
      if (result.status === 401) {
        setAllowError(sessionExpired);
        return;
      }
      setAllowError(errorMessage(result.body, "Access allow policy could not be created."));
    } catch {
      setAllowEmail("");
      setAllowError("Access allow policy could not be created.");
    } finally {
      setAllowBusy(false);
    }
  }

  async function handleConfirmReachable() {
    if (confirmBusy) {
      return;
    }
    const expected = status?.hostname ?? "";
    if (confirmTyped !== expected) {
      setConfirmError("Type the exact hostname to close bootstrap.");
      return;
    }
    setConfirmBusy(true);
    setConfirmError("");
    try {
      const result = await confirmDomainReachable(csrf);
      if (result.status === 200 && result.body.ok === true) {
        setConfirmOpen(false);
        setConfirmTyped("");
        refresh();
        return;
      }
      if (result.status === 401) {
        setConfirmError(sessionExpired);
        return;
      }
      setConfirmError(errorMessage(result.body, "Could not confirm console reachability."));
    } catch {
      setConfirmError("Could not confirm console reachability.");
    } finally {
      setConfirmBusy(false);
    }
  }

  const configured = status?.configured === true;
  const showWizard = status?.configured === false && !statusError;
  const canApply = showWizard && zone.trim() !== "" && hostname.trim() !== "" && !applyBusy;
  const accessAllow = status?.access === "allow";
  const bootstrapOpen = status?.bootstrap_still_open === true;

  return (
    <article>
      <header className="page-header">
        <h1>Domain & Network</h1>
        <p>Connect a Cloudflare zone with a per-zone API token. Tokens stay on the server and are never returned.</p>
        <button type="button" className="text-button" onClick={refresh}>
          Refresh
        </button>
      </header>

      {statusError ? (
        <p className="form-warning" role="alert">
          {statusError}
        </p>
      ) : null}

      {loading && status === null && !statusError ? (
        <p className="muted-copy" aria-live="polite">
          Loading domain status.
        </p>
      ) : null}

      {status && !statusError ? (
        <section aria-labelledby="domain-status-heading">
          <h2 id="domain-status-heading">Status</h2>
          <dl className="fact-list">
            <div>
              <dt>Configured</dt>
              <dd>{configured ? "Yes" : "No"}</dd>
            </div>
            {configured && status.zone ? (
              <div>
                <dt>Zone</dt>
                <dd className="bidi-isolate identifier">{displayText(status.zone)}</dd>
              </div>
            ) : null}
            {configured && status.hostname ? (
              <div>
                <dt>Hostname</dt>
                <dd className="bidi-isolate identifier">{displayText(status.hostname)}</dd>
              </div>
            ) : null}
            {configured ? (
              <div>
                <dt>Access</dt>
                <dd>
                  {accessAllow
                    ? "Allow policy configured"
                    : "Deny by default (add an allow policy)"}
                </dd>
              </div>
            ) : null}
            <div>
              <dt>Bootstrap</dt>
              <dd>{bootstrapOpen ? "Still open" : "Closed or not configured"}</dd>
            </div>
          </dl>
          {configured && !accessAllow ? (
            <form onSubmit={handleAccessAllow} autoComplete="off">
              <h3>3. Access allow policy</h3>
              <p className="muted-copy">
                Add your email so Cloudflare Access allows you through to{" "}
                {status.hostname ? (
                  <span className="bidi-isolate identifier">{displayText(status.hostname)}</span>
                ) : (
                  "the console hostname"
                )}
                .
              </p>
              <div className="field-stack">
                <label htmlFor="domain-access-email">Allowed email</label>
                <input
                  id="domain-access-email"
                  name="access_email"
                  type="email"
                  autoComplete="off"
                  value={allowEmail}
                  disabled={allowBusy}
                  onChange={(event) => {
                    setAllowEmail(event.target.value);
                    setAllowError("");
                  }}
                />
              </div>
              {allowError ? (
                <p className="form-error" role="alert">
                  {allowError}
                </p>
              ) : null}
              <button type="submit" disabled={allowBusy || allowEmail.trim() === ""}>
                Add Access allow policy
              </button>
            </form>
          ) : null}
          {configured && accessAllow && bootstrapOpen ? (
            <div>
              <h3>4. Close bootstrap</h3>
              <p className="muted-copy">
                After you can open the console hostname through Tunnel + Access, close the temporary public bootstrap
                listener. This cannot be undone from the UI.
              </p>
              {confirmError && !confirmOpen ? (
                <p className="form-error" role="alert">
                  {confirmError}
                </p>
              ) : null}
              <button
                ref={confirmButtonRef}
                type="button"
                disabled={confirmBusy}
                onClick={() => {
                  setConfirmTyped("");
                  setConfirmError("");
                  setConfirmOpen(true);
                }}
              >
                Console is reachable — close bootstrap
              </button>
            </div>
          ) : null}
          {configured ? (
            <button
              ref={disconnectButtonRef}
              type="button"
              className="text-button"
              onClick={() => {
                setDisconnectConfirm("");
                setDisconnectError("");
                setDisconnectOpen(true);
              }}
            >
              Disconnect domain
            </button>
          ) : null}
        </section>
      ) : null}

      {showWizard ? (
        <>
          <section aria-labelledby="domain-token-heading">
            <h2 id="domain-token-heading">1. Cloudflare API token</h2>
            <p className="muted-copy">
              Create a per-zone API token in Cloudflare with at least these permissions, then paste it once. Redgres
              stores it server-side only.
            </p>
            <ul className="muted-copy">
              {CLOUDFLARE_TOKEN_PERMISSIONS.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
            <form onSubmit={handleTokenSubmit} autoComplete="off">
              <div className="field-stack">
                <label htmlFor="domain-cloudflare-token">API token</label>
                <input
                  ref={tokenInputRef}
                  id="domain-cloudflare-token"
                  name="cloudflare_api_token"
                  type="password"
                  autoComplete="off"
                  value={token}
                  maxLength={maxTokenLength}
                  disabled={tokenBusy}
                  onChange={(event) => {
                    setToken(event.target.value);
                    setTokenSaved(false);
                    setTokenError("");
                  }}
                  aria-invalid={tokenError ? true : undefined}
                />
              </div>
              {tokenError ? (
                <p className="form-error" role="alert">
                  {tokenError}
                </p>
              ) : null}
              {tokenSaved ? (
                <p className="muted-copy" role="status">
                  Token stored on the server. It is not shown again.
                </p>
              ) : null}
              <button type="submit" disabled={tokenBusy || token.trim() === ""}>
                Store token
              </button>
            </form>
          </section>

          <section aria-labelledby="domain-apply-heading">
            <h2 id="domain-apply-heading">2. Apply tunnel and DNS</h2>
            <p className="muted-copy">
              Creates a remotely managed tunnel, ingress to loopback Redgres, a proxied CNAME, and a deny-by-default
              Access app. Run <code>cloudflared</code> on the host afterward (see operator docs).
            </p>
            <form onSubmit={handleApply} autoComplete="off">
              <div className="field-stack">
                <label htmlFor="domain-zone">Zone</label>
                <input
                  id="domain-zone"
                  name="zone"
                  autoComplete="off"
                  value={zone}
                  disabled={applyBusy}
                  onChange={(event) => handleZoneChange(event.target.value)}
                  placeholder="example.com"
                />
                <label htmlFor="domain-hostname">Console hostname</label>
                <input
                  id="domain-hostname"
                  name="hostname"
                  autoComplete="off"
                  value={hostname}
                  disabled={applyBusy}
                  onChange={(event) => {
                    hostnameEdited.current = true;
                    setHostname(event.target.value);
                  }}
                  placeholder="redgres.example.com"
                />
              </div>
              {applyError ? (
                <p className="form-error" role="alert">
                  {applyError}
                </p>
              ) : null}
              {!tokenSaved ? (
                <p className="muted-copy">
                  If this host has no Cloudflare API token yet, store one above first. Apply uses the server-side file
                  (it is not re-checked in this browser session).
                </p>
              ) : null}
              <button type="submit" disabled={!canApply}>
                Apply domain
              </button>
            </form>
          </section>
        </>
      ) : null}

      {applyResult ? (
        <section aria-labelledby="domain-apply-result-heading">
          <h2 id="domain-apply-result-heading">Apply result</h2>
          <dl className="fact-list">
            <div>
              <dt>Zone</dt>
              <dd className="bidi-isolate identifier">{displayText(applyResult.zone ?? "")}</dd>
            </div>
            <div>
              <dt>Hostname</dt>
              <dd className="bidi-isolate identifier">{displayText(applyResult.hostname ?? "")}</dd>
            </div>
            <div>
              <dt>Tunnel ID</dt>
              <dd className="bidi-isolate identifier">{displayText(applyResult.tunnel_id ?? "")}</dd>
            </div>
            <div>
              <dt>Access</dt>
              <dd>{applyResult.access === "deny_by_default" ? "Deny by default (add an allow policy)" : displayText(applyResult.access ?? "")}</dd>
            </div>
            <div>
              <dt>Bootstrap</dt>
              <dd>
                {applyResult.bootstrap_still_open
                  ? "Still open — closes on hard-cap or a later console-reachable confirm"
                  : "Closed"}
              </dd>
            </div>
          </dl>
          <p className="muted-copy">
            Tunnel ID is a Cloudflare resource identifier (also visible in the public CNAME), not a secret token.
          </p>
        </section>
      ) : null}

      {disconnectOpen && configured && status?.hostname ? (
        <HostnameConfirmDialog
          title="Disconnect domain"
          description={
            <>
              Deletes only the tunnel, DNS record, and Access app Redgres created for{" "}
              <span className="bidi-isolate identifier">{displayText(status.hostname)}</span>. Unrelated zone records are
              not touched. Type the exact hostname to confirm.
            </>
          }
          hostname={status.hostname}
          confirmation={disconnectConfirm}
          error={disconnectError}
          submitting={disconnectBusy}
          submitLabel="Disconnect"
          restoreFocusRef={disconnectButtonRef}
          onConfirmationChange={setDisconnectConfirm}
          onCancel={() => {
            setDisconnectOpen(false);
            setDisconnectConfirm("");
            setDisconnectError("");
          }}
          onConfirm={() => void handleDisconnect()}
        />
      ) : null}

      {confirmOpen && configured && status?.hostname ? (
        <HostnameConfirmDialog
          title="Close bootstrap listener"
          description={
            <>
              Closes the temporary public bootstrap port. Use this only after{" "}
              <span className="bidi-isolate identifier">{displayText(status.hostname)}</span> opens through Tunnel +
              Access. Type the exact hostname to confirm — this cannot be undone from the UI.
            </>
          }
          hostname={status.hostname}
          confirmation={confirmTyped}
          error={confirmError}
          submitting={confirmBusy}
          submitLabel="Close bootstrap"
          restoreFocusRef={confirmButtonRef}
          onConfirmationChange={setConfirmTyped}
          onCancel={() => {
            setConfirmOpen(false);
            setConfirmTyped("");
            setConfirmError("");
          }}
          onConfirm={() => void handleConfirmReachable()}
        />
      ) : null}
    </article>
  );
}

function HostnameConfirmDialog({
  title,
  description,
  hostname,
  confirmation,
  error,
  submitting,
  submitLabel,
  restoreFocusRef,
  onConfirmationChange,
  onCancel,
  onConfirm,
}: {
  title: string;
  description: ReactNode;
  hostname: string;
  confirmation: string;
  error: string;
  submitting: boolean;
  submitLabel: string;
  restoreFocusRef: RefObject<HTMLElement | null>;
  onConfirmationChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const confirmId = useId();
  const errorId = useId();
  useFocusTrap(dialogRef, true, restoreFocusRef);

  const canSubmit = confirmation === hostname && !submitting;

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!canSubmit) {
      return;
    }
    onConfirm();
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
        onKeyDown={(event) => {
          if (event.key === "Escape" && !submitting) {
            event.stopPropagation();
            onCancel();
          }
        }}
      >
        <h2 id={titleId}>{title}</h2>
        <p>{description}</p>
        <form onSubmit={handleSubmit} autoComplete="off">
          <div className="field-stack">
            <label htmlFor={confirmId}>Confirm hostname</label>
            <input
              id={confirmId}
              name="hostname_confirmation"
              autoComplete="off"
              value={confirmation}
              disabled={submitting}
              onChange={(event) => onConfirmationChange(event.target.value)}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
            />
          </div>
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="form-actions">
            <button type="button" className="text-button" disabled={submitting} onClick={onCancel}>
              Cancel
            </button>
            <button type="submit" disabled={!canSubmit}>
              {submitLabel}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
