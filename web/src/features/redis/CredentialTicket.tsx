import { useId } from "react";
import { displayText } from "../../text/displayText";

export type ShownCredential = {
  username: string;
  password: string;
  url?: string;
};

type CredentialTicketProps = {
  credential: ShownCredential;
  onDismiss: () => void;
};

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
  }
}

export default function CredentialTicket({ credential, onDismiss }: CredentialTicketProps) {
  const titleId = useId();

  return (
    <section className="credential-ticket" role="alertdialog" aria-labelledby={titleId} aria-modal="true">
      <h2 id={titleId}>This Redis password is shown now.</h2>
      <p className="muted-copy">
        This is a one-time Redis credential. Redgres cannot show the password again after you dismiss this ticket.
      </p>
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
        {credential.url ? (
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
