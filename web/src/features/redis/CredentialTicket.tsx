import { useId } from "react";
import { displayText } from "../../text/displayText";
import { copyText } from "../../text/copyText";

export type ShownCredential = {
  username: string;
  password: string;
  url?: string;
  directUrl?: string;
  pooledUrl?: string;
};

type CredentialTicketProps = {
  credential: ShownCredential;
  onDismiss: () => void;
  kind?: "redis" | "postgres";
  rotateWarning?: boolean;
};

export default function CredentialTicket({
  credential,
  onDismiss,
  kind = "redis",
  rotateWarning = false,
}: CredentialTicketProps) {
  const titleId = useId();
  const postgres = kind === "postgres";

  return (
    <section className="credential-ticket" role="alertdialog" aria-labelledby={titleId} aria-modal="true">
      <h2 id={titleId}>
        {postgres ? "This PostgreSQL password is still saved." : "This Redis password is shown now."}
      </h2>
      <p className="muted-copy">
        {postgres
          ? "Redgres can show this password again from the encrypted vault. It is not a one-time Redis credential."
          : "This is a one-time Redis credential. Redgres cannot show the password again after you dismiss this ticket."}
      </p>
      {rotateWarning ? (
        <p className="form-warning">
          Update every application using this project user. The previous password stops working.
        </p>
      ) : null}
      <dl className="fact-list">
        <div>
          <dt>Username</dt>
          <dd className="bidi-isolate identifier">{displayText(credential.username)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(credential.username)}>
            Copy username
          </button>
        </div>
        <div>
          <dt>Password</dt>
          <dd className="bidi-isolate identifier">{displayText(credential.password)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(credential.password)}>
            Copy password
          </button>
        </div>
        {postgres ? (
          <>
            {credential.directUrl ? (
              <div>
                <dt>Direct URL</dt>
                <dd className="bidi-isolate identifier">{displayText(credential.directUrl)}</dd>
                <button type="button" className="text-button" onClick={() => void copyText(credential.directUrl ?? "")}>
                  Copy Direct URL
                </button>
              </div>
            ) : null}
            {credential.pooledUrl ? (
              <div>
                <dt>Pooled URL</dt>
                <dd className="bidi-isolate identifier">{displayText(credential.pooledUrl)}</dd>
                <button type="button" className="text-button" onClick={() => void copyText(credential.pooledUrl ?? "")}>
                  Copy Pooled URL
                </button>
              </div>
            ) : null}
          </>
        ) : credential.url ? (
          <div>
            <dt>URL</dt>
            <dd className="bidi-isolate identifier">{displayText(credential.url)}</dd>
            <button type="button" className="text-button" onClick={() => void copyText(credential.url ?? "")}>
              Copy URL
            </button>
          </div>
        ) : null}
      </dl>
      <p className="form-warning">
        Copied values remain in the operating system clipboard history outside Redgres control.
      </p>
      <button type="button" className="primary-button" onClick={onDismiss}>
        I have copied it — dismiss
      </button>
    </section>
  );
}
