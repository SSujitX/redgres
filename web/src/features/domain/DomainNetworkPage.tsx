import { FormEvent, useEffect, useId, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  applyDomain,
  confirmDomainManualAccess,
  confirmDomainReachable,
  disconnectDomain,
  errorMessage,
  fetchDomain,
  issueDomainTLS,
  setDomainAccessPolicy,
  setDomainOAuthClient,
  setDomainToken,
  startDomainOAuth,
  verifyDomainManual,
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

function suggestedConsoleHostname(zone: string): string {
  const z = zone.trim().toLowerCase();
  if (z === "" || z.includes(" ") || !z.includes(".")) {
    return "";
  }
  return `console.${z}`;
}

function suggestedSubHostname(prefix: string, zone: string): string {
  const z = zone.trim().toLowerCase();
  if (z === "" || z.includes(" ") || !z.includes(".")) {
    return "";
  }
  return `${prefix}.${z}`;
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
  const [dbHostname, setDbHostname] = useState("");
  const [redisHostname, setRedisHostname] = useState("");
  const [originIP, setOriginIP] = useState("");
  const [dnsProvider, setDnsProvider] = useState<"cloudflare" | "manual">("cloudflare");
  const [manualInstructions, setManualInstructions] = useState<string[]>([]);
  const [manualVerifyBusy, setManualVerifyBusy] = useState(false);
  const [manualVerifyError, setManualVerifyError] = useState("");
  const [manualVerifyResults, setManualVerifyResults] = useState<Record<string, string> | null>(null);
  const [manualAccessBusy, setManualAccessBusy] = useState(false);
  const [manualAccessError, setManualAccessError] = useState("");
  const hostnameEdited = useRef(false);
  const dbEdited = useRef(false);
  const redisEdited = useRef(false);
  const [applyError, setApplyError] = useState("");
  const [applyBusy, setApplyBusy] = useState(false);
  const [applyResult, setApplyResult] = useState<DomainApplyPayload | null>(null);

  const [disconnectOpen, setDisconnectOpen] = useState(false);
  const [disconnectConfirm, setDisconnectConfirm] = useState("");
  const [disconnectError, setDisconnectError] = useState("");
  const [disconnectBusy, setDisconnectBusy] = useState(false);

  const [allowEmails, setAllowEmails] = useState<string[]>([""]);
  const [allowError, setAllowError] = useState("");
  const [allowBusy, setAllowBusy] = useState(false);

  const [oauthClientID, setOauthClientID] = useState("");
  const [oauthClientSecret, setOauthClientSecret] = useState("");
  const [oauthError, setOauthError] = useState("");
  const [oauthBusy, setOauthBusy] = useState(false);

  const [tlsError, setTlsError] = useState("");
  const [tlsBusy, setTlsBusy] = useState(false);
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
        if (result.body.configured && Array.isArray(result.body.instructions)) {
          setManualInstructions(result.body.instructions);
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
      setHostname(suggestedConsoleHostname(value));
    }
    if (!dbEdited.current) {
      setDbHostname(suggestedSubHostname("db", value));
    }
    if (!redisEdited.current) {
      setRedisHostname(suggestedSubHostname("redis", value));
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
    const consoleHost = hostname.trim().toLowerCase();
    const dbHost = dbHostname.trim().toLowerCase();
    const redisHost = redisHostname.trim().toLowerCase();
    const origin = originIP.trim();
    if (z === "" || consoleHost === "" || dbHost === "" || redisHost === "" || origin === "" || applyBusy) {
      setApplyError("Enter zone, all hostnames, and origin IP.");
      return;
    }
    setApplyBusy(true);
    setApplyError("");
    setApplyResult(null);
    try {
      const result = await applyDomain(
        {
          zone: z,
          originIP: origin,
          hostnames: { console: consoleHost, db: dbHost, redis: redisHost },
          dnsProvider,
        },
        csrf,
      );
      if (result.status === 200 && Array.isArray(result.body.instructions)) {
        setManualInstructions(result.body.instructions);
        setApplyResult(result.body);
        refresh();
        return;
      }
      if (result.status === 200 && typeof result.body.hostname === "string") {
        setApplyResult(result.body);
        refresh();
        return;
      }
      if (result.status === 200 && result.body.hostnames?.console) {
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
        setAllowEmails([""]);
        hostnameEdited.current = false;
        dbEdited.current = false;
        redisEdited.current = false;
        setZone("");
        setHostname("");
        setDbHostname("");
        setRedisHostname("");
        setOriginIP("");
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
    const emails = allowEmails.map((e) => e.trim()).filter((e) => e !== "");
    if (emails.length === 0 || allowBusy) {
      setAllowError("Enter at least one email allowed by Cloudflare Access.");
      return;
    }
    if (emails.length > 8) {
      setAllowError("At most 8 emails are allowed.");
      return;
    }
    setAllowBusy(true);
    setAllowError("");
    try {
      const result = await setDomainAccessPolicy(emails, csrf);
      setAllowEmails([""]);
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
      setAllowEmails([""]);
      setAllowError("Access allow policy could not be created.");
    } finally {
      setAllowBusy(false);
    }
  }

  async function handleOAuthConnect(event: FormEvent) {
    event.preventDefault();
    const clientID = oauthClientID.trim();
    const clientSecret = oauthClientSecret.trim();
    if (clientID === "" || clientSecret === "" || oauthBusy) {
      setOauthError("Enter OAuth client ID and secret.");
      return;
    }
    setOauthBusy(true);
    setOauthError("");
    try {
      const storeResult = await setDomainOAuthClient(clientID, clientSecret, csrf);
      setOauthClientSecret("");
      if (storeResult.status !== 200) {
        setOauthError(errorMessage(storeResult.body, "OAuth client could not be stored."));
        return;
      }
      const startResult = await startDomainOAuth(csrf);
      if (startResult.status === 200 && typeof startResult.body.authorize_url === "string") {
        window.location.assign(startResult.body.authorize_url);
        return;
      }
      setOauthError(errorMessage(startResult.body, "OAuth connect could not start."));
    } catch {
      setOauthClientSecret("");
      setOauthError("OAuth connect could not start.");
    } finally {
      setOauthBusy(false);
    }
  }

  async function handleTLSIssue() {
    if (tlsBusy) {
      return;
    }
    setTlsBusy(true);
    setTlsError("");
    try {
      const result = await issueDomainTLS(csrf);
      if (result.status === 200 && result.body.ok === true) {
        refresh();
        return;
      }
      setTlsError(errorMessage(result.body, "TLS issuance failed."));
    } catch {
      setTlsError("TLS issuance failed.");
    } finally {
      setTlsBusy(false);
    }
  }

  async function handleManualVerify() {
    if (manualVerifyBusy) {
      return;
    }
    setManualVerifyBusy(true);
    setManualVerifyError("");
    try {
      const result = await verifyDomainManual(csrf);
      if (result.status === 200 && result.body.results) {
        setManualVerifyResults(result.body.results);
        return;
      }
      setManualVerifyError(errorMessage(result.body, "Manual DNS verification failed."));
    } catch {
      setManualVerifyError("Manual DNS verification failed.");
    } finally {
      setManualVerifyBusy(false);
    }
  }

  async function handleManualConfirmAccess() {
    if (manualAccessBusy) {
      return;
    }
    setManualAccessBusy(true);
    setManualAccessError("");
    try {
      const result = await confirmDomainManualAccess(csrf);
      if (result.status === 200 && result.body.ok === true) {
        refresh();
        return;
      }
      setManualAccessError(errorMessage(result.body, "Could not confirm manual Access."));
    } catch {
      setManualAccessError("Could not confirm manual Access.");
    } finally {
      setManualAccessBusy(false);
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
        if (result.body.bootstrap_ufw_attempted && result.body.bootstrap_ufw_removed === false) {
          setConfirmError("Bootstrap closed, but the UFW removal helper did not succeed. Check firewall rules manually.");
        }
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
  const isManual = status?.dns_provider === "manual" || dnsProvider === "manual";
  const showWizard = status?.configured === false && !statusError;
  const showCloudflareTokenWizard = showWizard && dnsProvider === "cloudflare";
  const canApply =
    showWizard &&
    zone.trim() !== "" &&
    hostname.trim() !== "" &&
    dbHostname.trim() !== "" &&
    redisHostname.trim() !== "" &&
    originIP.trim() !== "" &&
    !applyBusy;
  const accessAllow = status?.access === "allow";
  const bootstrapOpen = status?.bootstrap_still_open === true;
  const credential = status?.credential ?? "none";
  const tlsDb = status?.tls?.db ?? "not_issued";
  const tlsRedis = status?.tls?.redis ?? "not_issued";

  return (
    <article>
      <header className="page-header">
        <h1>Domain & Network</h1>
        <p>
          Connect Cloudflare (automated) or save a manual DNS plan. Credentials stay on the server and are never
          returned.
        </p>
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
            {configured && (status.hostname || status.hostnames?.console) ? (
              <div>
                <dt>Console hostname</dt>
                <dd className="bidi-isolate identifier">
                  {displayText(status.hostname ?? status.hostnames?.console ?? "")}
                </dd>
              </div>
            ) : null}
            {configured && status.hostnames?.db ? (
              <div>
                <dt>DB hostname</dt>
                <dd className="bidi-isolate identifier">{displayText(status.hostnames.db)}</dd>
              </div>
            ) : null}
            {configured && status.hostnames?.redis ? (
              <div>
                <dt>Redis hostname</dt>
                <dd className="bidi-isolate identifier">{displayText(status.hostnames.redis)}</dd>
              </div>
            ) : null}
            {configured && status.origin_ip ? (
              <div>
                <dt>Origin IP</dt>
                <dd className="bidi-isolate identifier">{displayText(status.origin_ip)}</dd>
              </div>
            ) : null}
            {configured ? (
              <div>
                <dt>Credential</dt>
                <dd>{credential === "oauth" ? "Cloudflare OAuth" : credential === "api_token" ? "API token" : "None"}</dd>
              </div>
            ) : null}
            {configured ? (
              <div>
                <dt>TLS (db / redis)</dt>
                <dd>
                  {displayText(tlsDb)} / {displayText(tlsRedis)}
                </dd>
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
          {configured && !accessAllow && isManual ? (
            <div>
              <h3>3. Manual DNS and Access</h3>
              <p className="muted-copy">
                Complete the DNS records and Cloudflare Access steps below outside Redgres, then confirm when done.
              </p>
              {manualInstructions.length > 0 ? (
                <ol className="muted-copy">
                  {manualInstructions.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ol>
              ) : null}
              {manualVerifyError ? (
                <p className="form-error" role="alert">
                  {manualVerifyError}
                </p>
              ) : null}
              {manualVerifyResults ? (
                <ul className="muted-copy">
                  {Object.entries(manualVerifyResults).map(([host, result]) => (
                    <li key={host}>
                      <span className="bidi-isolate identifier">{displayText(host)}</span>: {displayText(result)}
                    </li>
                  ))}
                </ul>
              ) : null}
              <div className="form-actions">
                <button type="button" disabled={manualVerifyBusy} onClick={() => void handleManualVerify()}>
                  Verify public DNS
                </button>
                <button type="button" disabled={manualAccessBusy} onClick={() => void handleManualConfirmAccess()}>
                  Access configured manually
                </button>
              </div>
              {manualAccessError ? (
                <p className="form-error" role="alert">
                  {manualAccessError}
                </p>
              ) : null}
            </div>
          ) : null}
          {configured && !accessAllow && !isManual ? (
            <form onSubmit={handleAccessAllow} autoComplete="off">
              <h3>3. Access allow policy</h3>
              <p className="muted-copy">
                Add up to 8 emails allowed by Cloudflare Access for{" "}
                {status.hostname ? (
                  <span className="bidi-isolate identifier">{displayText(status.hostname)}</span>
                ) : (
                  "the console hostname"
                )}
                .
              </p>
              <div className="field-stack">
                {allowEmails.map((email, index) => (
                  <label key={`access-email-${index}`} htmlFor={`domain-access-email-${index}`}>
                    Allowed email {index + 1}
                    <input
                      id={`domain-access-email-${index}`}
                      name={`access_email_${index}`}
                      type="email"
                      autoComplete="off"
                      value={email}
                      disabled={allowBusy}
                      onChange={(event) => {
                        const next = [...allowEmails];
                        next[index] = event.target.value;
                        setAllowEmails(next);
                        setAllowError("");
                      }}
                    />
                  </label>
                ))}
                {allowEmails.length < 8 ? (
                  <button
                    type="button"
                    className="text-button"
                    disabled={allowBusy}
                    onClick={() => setAllowEmails([...allowEmails, ""])}
                  >
                    Add another email
                  </button>
                ) : null}
              </div>
              {allowError ? (
                <p className="form-error" role="alert">
                  {allowError}
                </p>
              ) : null}
              <button type="submit" disabled={allowBusy || allowEmails.every((e) => e.trim() === "")}>
                Add Access allow policy
              </button>
            </form>
          ) : null}
          {configured && accessAllow && credential !== "oauth" && !isManual ? (
            <form onSubmit={handleOAuthConnect} autoComplete="off">
              <h3>4. Connect Cloudflare OAuth</h3>
              <p className="muted-copy">
                Create a self-hosted OAuth app in Cloudflare, paste client ID and secret, then connect. Redirect URI:{" "}
                <span className="bidi-isolate identifier">
                  https://{displayText(status.hostname ?? "console.example.com")}/api/v1/domain/oauth/callback
                </span>
              </p>
              <div className="field-stack">
                <label htmlFor="domain-oauth-client-id">OAuth client ID</label>
                <input
                  id="domain-oauth-client-id"
                  name="oauth_client_id"
                  autoComplete="off"
                  value={oauthClientID}
                  disabled={oauthBusy}
                  onChange={(event) => {
                    setOauthClientID(event.target.value);
                    setOauthError("");
                  }}
                />
                <label htmlFor="domain-oauth-client-secret">OAuth client secret</label>
                <input
                  id="domain-oauth-client-secret"
                  name="oauth_client_secret"
                  type="password"
                  autoComplete="off"
                  value={oauthClientSecret}
                  disabled={oauthBusy}
                  onChange={(event) => {
                    setOauthClientSecret(event.target.value);
                    setOauthError("");
                  }}
                />
              </div>
              {oauthError ? (
                <p className="form-error" role="alert">
                  {oauthError}
                </p>
              ) : null}
              <button type="submit" disabled={oauthBusy || oauthClientID.trim() === "" || oauthClientSecret.trim() === ""}>
                Connect Cloudflare
              </button>
            </form>
          ) : null}
          {configured && accessAllow && tlsDb !== "issued" && status.dns_provider !== "manual" ? (
            <div>
              <h3>5. Issue TLS certificates (db + redis)</h3>
              <p className="muted-copy">
                Issues Let&apos;s Encrypt DNS-01 certificates for grey-cloud db and redis hostnames via certbot.
              </p>
              {tlsError ? (
                <p className="form-error" role="alert">
                  {tlsError}
                </p>
              ) : null}
              <button type="button" disabled={tlsBusy} onClick={() => void handleTLSIssue()}>
                Issue TLS certificates
              </button>
            </div>
          ) : null}
          {configured && accessAllow && bootstrapOpen ? (
            <div>
              <h3>6. Close bootstrap</h3>
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
          <section aria-labelledby="domain-provider-heading">
            <h2 id="domain-provider-heading">1. DNS provider</h2>
            <fieldset className="field-stack">
              <legend className="sr-only">DNS provider</legend>
              <label>
                <input
                  type="radio"
                  name="dns_provider"
                  value="cloudflare"
                  checked={dnsProvider === "cloudflare"}
                  onChange={() => setDnsProvider("cloudflare")}
                />
                Cloudflare API (automated tunnel + DNS)
              </label>
              <label>
                <input
                  type="radio"
                  name="dns_provider"
                  value="manual"
                  checked={dnsProvider === "manual"}
                  onChange={() => setDnsProvider("manual")}
                />
                Manual DNS (instructions only)
              </label>
            </fieldset>
          </section>

          {showCloudflareTokenWizard ? (
          <section aria-labelledby="domain-token-heading">
            <h2 id="domain-token-heading">2. Cloudflare API token</h2>
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
          ) : null}

          <section aria-labelledby="domain-apply-heading">
            <h2 id="domain-apply-heading">{dnsProvider === "manual" ? "2. Save hostname plan" : "3. Apply tunnel and DNS"}</h2>
            <p className="muted-copy">
              {dnsProvider === "manual"
                ? "Records and Access steps below as operator instructions; Redgres does not mutate Cloudflare in manual mode."
                : "Creates a remotely managed tunnel, ingress to loopback Redgres, a proxied CNAME, grey-cloud db/redis A or AAAA records, and a deny-by-default Access app."}
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
                  placeholder="console.example.com"
                />
                <label htmlFor="domain-db-hostname">DB hostname</label>
                <input
                  id="domain-db-hostname"
                  name="db_hostname"
                  autoComplete="off"
                  value={dbHostname}
                  disabled={applyBusy}
                  onChange={(event) => {
                    dbEdited.current = true;
                    setDbHostname(event.target.value);
                  }}
                  placeholder="db.example.com"
                />
                <label htmlFor="domain-redis-hostname">Redis hostname</label>
                <input
                  id="domain-redis-hostname"
                  name="redis_hostname"
                  autoComplete="off"
                  value={redisHostname}
                  disabled={applyBusy}
                  onChange={(event) => {
                    redisEdited.current = true;
                    setRedisHostname(event.target.value);
                  }}
                  placeholder="redis.example.com"
                />
                <label htmlFor="domain-origin-ip">Origin IP (grey-cloud A or AAAA)</label>
                <input
                  id="domain-origin-ip"
                  name="origin_ip"
                  autoComplete="off"
                  value={originIP}
                  disabled={applyBusy}
                  onChange={(event) => setOriginIP(event.target.value)}
                  placeholder="203.0.113.10"
                />
              </div>
              {applyError ? (
                <p className="form-error" role="alert">
                  {applyError}
                </p>
              ) : null}
              {!tokenSaved && dnsProvider === "cloudflare" ? (
                <p className="muted-copy">
                  If this host has no Cloudflare API token yet, store one above first. Apply uses the server-side file
                  (it is not re-checked in this browser session).
                </p>
              ) : null}
              <button type="submit" disabled={!canApply}>
                {dnsProvider === "manual" ? "Save manual plan" : "Apply domain"}
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
