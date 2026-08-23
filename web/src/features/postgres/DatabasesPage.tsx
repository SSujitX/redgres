import { useEffect, useState } from "react";
import {
  errorMessage,
  fetchPostgresDatabase,
  fetchPostgresDatabases,
  type DatabaseDetails,
  type DatabaseListItem,
} from "../../api/postgres";

export default function DatabasesPage() {
  const [items, setItems] = useState<DatabaseListItem[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [details, setDetails] = useState<DatabaseDetails | null>(null);
  const [detailsError, setDetailsError] = useState("");
  const [loadingDetails, setLoadingDetails] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    fetchPostgresDatabases()
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 200 && Array.isArray(result.body.databases)) {
          setItems(result.body.databases);
          setTruncated(result.body.truncated === true);
          setListError("");
          return;
        }
        setItems(null);
        setListError(errorMessage(result.body, "PostgreSQL is unavailable"));
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setItems(null);
          setListError("PostgreSQL is unavailable");
        }
      });
    return () => controller.abort();
  }, []);

  async function openDetails(name: string) {
    setSelected(name);
    setDetails(null);
    setDetailsError("");
    setLoadingDetails(true);
    try {
      const result = await fetchPostgresDatabase(name);
      if (result.status === 200 && result.body.database?.name) {
        setDetails(result.body.database);
        return;
      }
      setDetailsError(errorMessage(result.body, "Database details are unavailable"));
    } catch {
      setDetailsError("PostgreSQL is unavailable");
    } finally {
      setLoadingDetails(false);
    }
  }

  return (
    <article>
      <header className="page-header">
        <h1>Databases</h1>
        <p>Manageable project databases only. Saved credentials are not loaded in this slice.</p>
      </header>
      {listError ? (
        <p className="form-warning" role="alert">
          {listError}
        </p>
      ) : items === null ? (
        <p className="muted-copy">Loading databases.</p>
      ) : items.length === 0 ? (
        <p className="muted-copy">No manageable project databases.</p>
      ) : (
        <ul className="ledger-list">
          {items.map((item) => {
            const name = item.name ?? "";
            if (!name) {
              return null;
            }
            return (
              <li key={name}>
                <button
                  type="button"
                  className={selected === name ? "ledger-item ledger-item-active" : "ledger-item"}
                  aria-current={selected === name ? "true" : undefined}
                  onClick={() => void openDetails(name)}
                >
                  <span>{name}</span>
                  <span className="muted-copy">{item.owner ?? ""}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {truncated ? <p className="form-warning">List truncated at 500 databases.</p> : null}
      {selected ? (
        <section className="detail-panel" aria-label="Database details">
          <h2>{selected}</h2>
          {loadingDetails ? <p className="muted-copy">Loading details.</p> : null}
          {detailsError ? (
            <p className="form-warning" role="alert">
              {detailsError}
            </p>
          ) : null}
          {details ? <DetailsFacts details={details} /> : null}
        </section>
      ) : null}
    </article>
  );
}

function DetailsFacts({ details }: { details: DatabaseDetails }) {
  return (
    <dl className="fact-list">
      <Fact label="Owner" value={details.owner ?? "—"} />
      <Fact label="Size" value={details.size ?? "—"} />
      <Fact label="Collation" value={details.collation ?? "—"} />
      <Fact label="Ctype" value={details.ctype ?? "—"} />
      <Fact label="Connections" value={String(details.connection_count ?? 0)} />
      <Fact label="PUBLIC CONNECT" value={yesNo(details.security?.public_can_connect)} />
      <Fact label="Owner can log in" value={yesNo(details.security?.owner_can_login)} />
      <Fact label="Saved credential" value="Not available" />
    </dl>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function yesNo(value: boolean | undefined): string {
  if (value === true) {
    return "Yes";
  }
  if (value === false) {
    return "No";
  }
  return "—";
}
