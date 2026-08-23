import { useEffect, useRef, useState } from "react";
import {
  errorMessage,
  fetchPostgresDatabase,
  fetchPostgresDatabases,
  fetchPostgresTables,
  type DatabaseDetails,
  type DatabaseListItem,
  type TableItem,
} from "../../api/postgres";

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

export default function DatabasesPage() {
  const [items, setItems] = useState<DatabaseListItem[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [details, setDetails] = useState<DatabaseDetails | null>(null);
  const [detailsError, setDetailsError] = useState("");
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [tables, setTables] = useState<TableItem[] | null>(null);
  const [tablesError, setTablesError] = useState("");
  const [tablesTruncated, setTablesTruncated] = useState(false);
  const [loadingTables, setLoadingTables] = useState(false);
  const selectionAbort = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    fetchPostgresDatabases({ signal: controller.signal })
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
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setItems(null);
        setListError("PostgreSQL is unavailable");
      });
    return () => {
      controller.abort();
      selectionAbort.current?.abort();
    };
  }, []);

  function openDetails(name: string) {
    selectionAbort.current?.abort();
    const controller = new AbortController();
    selectionAbort.current = controller;
    setSelected(name);
    setDetails(null);
    setDetailsError("");
    setLoadingDetails(true);
    setTables(null);
    setTablesError("");
    setTablesTruncated(false);
    setLoadingTables(true);
    void loadDetails(name, controller);
    void loadTables(name, controller);
  }

  async function loadDetails(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresDatabase(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && result.body.database?.name === name) {
        setDetails(result.body.database);
        setDetailsError("");
        return;
      }
      setDetails(null);
      setDetailsError(errorMessage(result.body, "Database details are unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDetails(null);
      setDetailsError("PostgreSQL is unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingDetails(false);
      }
    }
  }

  async function loadTables(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresTables(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.tables)) {
        setTables(result.body.tables);
        setTablesTruncated(result.body.truncated === true);
        setTablesError("");
        return;
      }
      setTables(null);
      setTablesTruncated(false);
      setTablesError(errorMessage(result.body, "Tables are unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setTables(null);
      setTablesTruncated(false);
      setTablesError("Tables are unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingTables(false);
      }
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
                  <span className="identifier">{name}</span>
                  <span className="muted-copy identifier">{item.owner ?? ""}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {truncated ? <p className="form-warning">List truncated at 500 databases.</p> : null}
      {selected ? (
        <section
          className="detail-panel"
          aria-label="Database details"
          aria-busy={loadingDetails || loadingTables}
        >
          <h2 className="identifier">{selected}</h2>
          {loadingDetails ? (
            <p className="muted-copy" role="status">
              Loading details.
            </p>
          ) : null}
          {detailsError ? (
            <p className="form-warning" role="alert">
              {detailsError}
            </p>
          ) : null}
          {details ? <DetailsFacts details={details} /> : null}
          <h3>Tables</h3>
          {loadingTables ? (
            <p className="muted-copy" role="status">
              Loading tables.
            </p>
          ) : null}
          {tablesError ? (
            <p className="form-warning" role="alert">
              {tablesError}
            </p>
          ) : null}
          {tablesTruncated ? <p className="form-warning">Table list truncated at 500 tables.</p> : null}
          {tables && tables.length === 0 && !tablesError ? <p className="muted-copy">No tables.</p> : null}
          {tables && tables.length > 0 ? <TableNameList tables={tables} /> : null}
        </section>
      ) : null}
    </article>
  );
}

function TableNameList({ tables }: { tables: TableItem[] }) {
  return (
    <ul className="table-name-list">
      {tables.map((table) => {
        const schema = table.schema ?? "";
        const name = table.name ?? "";
        if (!schema || !name) {
          return null;
        }
        return (
          <li key={`${schema}.${name}`} className="table-name-item">
            <span>
              <span className="muted-copy">Schema </span>
              <span className="identifier">{schema}</span>
            </span>
            <span>
              <span className="muted-copy">Table </span>
              <span className="identifier">{name}</span>
            </span>
          </li>
        );
      })}
    </ul>
  );
}

function DetailsFacts({ details }: { details: DatabaseDetails }) {
  return (
    <dl className="fact-list">
      <Fact label="Owner" value={details.owner ?? "—"} kind="identifier" />
      <Fact label="Size" value={details.size ?? "—"} kind="metric" />
      <Fact label="Collation" value={details.collation ?? "—"} kind="identifier" />
      <Fact label="Ctype" value={details.ctype ?? "—"} kind="identifier" />
      <Fact
        label="Connections"
        value={details.connection_count == null ? "—" : String(details.connection_count)}
        kind="metric"
      />
      <Fact label="PUBLIC CONNECT" value={yesNo(details.security?.public_can_connect)} />
      <Fact label="Owner is superuser" value={yesNo(details.security?.owner_is_superuser)} />
      <Fact label="Owner can log in" value={yesNo(details.security?.owner_can_login)} />
      <Fact label="Owner can create databases" value={yesNo(details.security?.owner_createdb)} />
      <Fact label="Owner can create roles" value={yesNo(details.security?.owner_createrole)} />
      <Fact label="Owner replication" value={yesNo(details.security?.owner_replication)} />
      <Fact label="Saved credential" value="Not available" />
    </dl>
  );
}

function Fact({
  label,
  value,
  kind,
}: {
  label: string;
  value: string;
  kind?: "identifier" | "metric";
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={kind === "identifier" ? "identifier" : kind === "metric" ? "metric" : undefined}>{value}</dd>
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
