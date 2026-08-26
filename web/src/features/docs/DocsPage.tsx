import { useEffect, useRef } from "react";
import { docArticles, lookupDoc, type DocArticleId } from "./catalog";

type DocsPageProps = {
  focusArticle?: string | null;
  focusNonce?: number;
  onSelectArticle: (id: string) => void;
  onBack: () => void;
};

export default function DocsPage({
  focusArticle = null,
  focusNonce = 0,
  onSelectArticle,
  onBack,
}: DocsPageProps) {
  const article = focusArticle ? lookupDoc(focusArticle) : undefined;
  const articleRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (!article) {
      return;
    }
    articleRef.current?.focus();
  }, [article, focusNonce]);

  return (
    <article>
      <header className="page-header">
        <h1>Documentation</h1>
        <p>Operator notes for this console. They describe what Redgres does today.</p>
      </header>
      <ul className={article ? "ledger-list ledger-list-inspecting" : "ledger-list"} aria-label="Articles">
        {docArticles.map((item) => {
          const selected = article?.id === item.id;
          return (
            <li key={item.id} className={selected ? "is-selected" : undefined}>
              <button
                type="button"
                className={selected ? "ledger-item ledger-item-active" : "ledger-item"}
                aria-current={selected ? "true" : undefined}
                onClick={() => onSelectArticle(item.id)}
              >
                <span>{item.title}</span>
              </button>
            </li>
          );
        })}
      </ul>
      {article ? (
        <section ref={articleRef} className="detail-panel doc-article" aria-label="Article" tabIndex={-1}>
          <p>
            <button type="button" className="text-button" onClick={onBack}>
              Back to documentation
            </button>
          </p>
          <h1>{article.title}</h1>
          <ArticleBody id={article.id} />
        </section>
      ) : null}
    </article>
  );
}

function ArticleBody({ id }: { id: DocArticleId }) {
  switch (id) {
    case "using-search":
      return <UsingSearchArticle />;
    case "postgres-databases":
      return <PostgresDatabasesArticle />;
    case "redis-acl-users":
      return <RedisAclUsersArticle />;
    case "credentials":
      return <CredentialsArticle />;
  }
}

function UsingSearchArticle() {
  return (
    <>
      <p>
        Search finds pages, project databases, Redis ACL users, and these notes. It is read-only discovery. It does
        not run drop, truncate, delete, reveal, or rotate, and it never shows passwords.
      </p>
      <h2>Open the palette</h2>
      <ul>
        <li>Use Search in the topbar.</li>
        <li>Press / or Ctrl/Cmd+K when you are not typing in a field.</li>
      </ul>
      <h2>How results are grouped</h2>
      <ul>
        <li>PostgreSQL databases — manageable names from the server.</li>
        <li>Redis ACL users — non-protected usernames from the server.</li>
        <li>Navigation — pages in this console, filtered in the browser.</li>
        <li>Documentation — this catalog’s landing page plus matching articles, filtered in the browser.</li>
      </ul>
      <p>Use the arrow keys to move through hits, then Enter to open one. Close search returns focus to Search.</p>
    </>
  );
}

function PostgresDatabasesArticle() {
  return (
    <>
      <p>
        The Databases page lists manageable project databases. Templates and other protected names are omitted. Select
        a row to inspect owner, size, connections, tables, and rows.
      </p>
      <h2>Inspect tables and rows</h2>
      <p>
        Open a table to page through rows. Search within a table is optional and bounded. Changing the selected
        database clears the table and row view.
      </p>
      <h2>Create and credentials</h2>
      <p>
        Create database is a page-header action. Redgres generates the project password and saves it in the encrypted
        vault. After create, a ticket shows the password in this session. Reveal can show a saved password again later
        from the inspector.
      </p>
      <h2>Inspector actions</h2>
      <ul>
        <li>Reveal — when a vault entry exists.</li>
        <li>Rotate — issues a new password and saves it. Update every application.</li>
        <li>
          Duplicate — copies the database. Active connections to the source are terminated. When the operation
          finishes, use Reveal if you need the new password.
        </li>
        <li>
          Truncate, Drop, and Delete selected rows — danger actions. They stay off until enabled on the server, and
          they require typed confirmation plus the owner password.
        </li>
      </ul>
      <p>
        Search never runs those mutations. Security overview is a separate diagnostic page; it does not reveal or
        rotate.
      </p>
    </>
  );
}

function RedisAclUsersArticle() {
  return (
    <>
      <p>
        The ACL users page lists Redis ACL users. Protected names are listed and inspectable but cannot be changed
        here.
      </p>
      <h2>Create</h2>
      <p>
        Create ACL user from the page header. Choose a username, a key prefix, and a permission preset: Cache
        read/write, Read only, Queue/worker, or Custom from the tested command allow-list. Redgres always creates the
        user enabled.
      </p>
      <p>
        A successful create opens a one-time ticket. Redgres cannot show that Redis password again; rotate to issue a
        new one.
      </p>
      <h2>Inspector</h2>
      <ul>
        <li>Enable or disable the user.</li>
        <li>Edit permissions to change prefix and grants. The password is unchanged.</li>
        <li>Rotate to issue a new password in a one-time ticket.</li>
        <li>
          Delete requires typing the username and the owner password. Existing connections for that user end; keys are
          not deleted.
        </li>
      </ul>
      <p>Permission presets is a nested page that shows the named command sets. It does not change users.</p>
    </>
  );
}

function CredentialsArticle() {
  return (
    <>
      <p>
        PostgreSQL project passwords are stored in the encrypted vault. Reveal shows a saved password again. Rotate
        replaces it; the previous password stops working, so update every application.
      </p>
      <p>
        Redis create and rotate show the password only in that ticket. There is no Redis reveal. If the ticket is
        dismissed, rotate again.
      </p>
      <ul>
        <li>Tickets stay in this browser tab’s memory.</li>
        <li>Dismiss the ticket, leave the page, or log out to clear it.</li>
        <li>Redgres does not write passwords to browser storage.</li>
      </ul>
      <p>
        Copied values remain in the operating system clipboard outside Redgres. Do not paste passwords into Search.
      </p>
    </>
  );
}
