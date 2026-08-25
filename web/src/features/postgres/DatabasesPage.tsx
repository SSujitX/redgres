import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  errorMessage,
  fetchPostgresConnection,
  fetchPostgresDatabase,
  fetchPostgresDatabases,
  fetchPostgresRows,
  fetchPostgresTables,
  revealPostgresConnection,
  type DatabaseDetails,
  type DatabaseListItem,
  type RowPage,
  type TableItem,
} from "../../api/postgres";
import CredentialTicket, { type ShownCredential } from "../redis/CredentialTicket";
import { displayText } from "../../text/displayText";

const maxRowQueryRunes = 128;
const sessionExpired = "Your session has expired. Sign in again to continue.";
const postgresUnavailable = "PostgreSQL is unavailable";

type ConnectionUrls = {
  savedCredentialStatus: string;
  maskedDirectUrl: string | null;
  maskedPooledUrl: string | null;
};

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
  }
}

function presentUrl(value: unknown): string | null {
  return typeof value === "string" && value !== "" ? value : null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function stringField(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function parsePostgresCredential(raw: unknown): ShownCredential | null {
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
  const directUrl = urls ? stringField(urls, "direct") : "";
  const pooledUrl = urls ? stringField(urls, "pooled") : "";
  const shown: ShownCredential = { username, password };
  if (directUrl !== "") {
    shown.directUrl = directUrl;
  }
  if (pooledUrl !== "") {
    shown.pooledUrl = pooledUrl;
  }
  return shown;
}

type SelectedTable = {
  schema: string;
  name: string;
};

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function queryRuneCount(value: string): number {
  return Array.from(value).length;
}

type DatabasesPageProps = {
  csrf?: string;
  focusDatabase?: string | null;
  focusNonce?: number;
};

export default function DatabasesPage({ csrf = "", focusDatabase = null, focusNonce = 0 }: DatabasesPageProps) {
  const [items, setItems] = useState<DatabaseListItem[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [details, setDetails] = useState<DatabaseDetails | null>(null);
  const [detailsError, setDetailsError] = useState("");
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [connection, setConnection] = useState<ConnectionUrls | null>(null);
  const [connectionError, setConnectionError] = useState("");
  const [loadingConnection, setLoadingConnection] = useState(false);
  const [ticket, setTicket] = useState<ShownCredential | null>(null);
  const [revealing, setRevealing] = useState(false);
  const [revealError, setRevealError] = useState("");
  const revealAbort = useRef<AbortController | null>(null);
  const [tables, setTables] = useState<TableItem[] | null>(null);
  const [tablesError, setTablesError] = useState("");
  const [tablesTruncated, setTablesTruncated] = useState(false);
  const [loadingTables, setLoadingTables] = useState(false);
  const [selectedTable, setSelectedTable] = useState<SelectedTable | null>(null);
  const [rowPage, setRowPage] = useState<RowPage | null>(null);
  const [rowsError, setRowsError] = useState("");
  const [rowsQueryError, setRowsQueryError] = useState("");
  const [loadingRows, setLoadingRows] = useState(false);
  const [queryDraft, setQueryDraft] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const selectionAbort = useRef<AbortController | null>(null);
  const rowsAbort = useRef<AbortController | null>(null);
  const rowsRegionRef = useRef<HTMLElement | null>(null);

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
      rowsAbort.current?.abort();
      revealAbort.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (!selectedTable) {
      return;
    }
    const node = rowsRegionRef.current;
    if (node && typeof node.scrollIntoView === "function") {
      node.scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }, [selectedTable]);

  function clearRowState() {
    setSelectedTable(null);
    setRowPage(null);
    setRowsError("");
    setRowsQueryError("");
    setQueryDraft("");
    setAppliedQuery("");
    setLoadingRows(false);
  }

  function clearTicket() {
    setTicket(null);
    setRevealError("");
    setRevealing(false);
  }

  function openDetails(name: string) {
    selectionAbort.current?.abort();
    rowsAbort.current?.abort();
    revealAbort.current?.abort();
    const controller = new AbortController();
    selectionAbort.current = controller;
    setSelected(name);
    setDetails(null);
    setDetailsError("");
    setLoadingDetails(true);
    setConnection(null);
    setConnectionError("");
    setLoadingConnection(true);
    setTables(null);
    setTablesError("");
    setTablesTruncated(false);
    setLoadingTables(true);
    clearTicket();
    clearRowState();
    void loadDetails(name, controller);
    void loadConnection(name, controller);
    void loadTables(name, controller);
  }

  useEffect(() => {
    if (!focusDatabase) {
      return;
    }
    openDetails(focusDatabase);
  }, [focusDatabase, focusNonce]);

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

  async function loadConnection(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresConnection(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setConnection(null);
        setConnectionError(sessionExpired);
        return;
      }
      if (result.status === 200) {
        setConnection({
          savedCredentialStatus: result.body.saved_credential?.status ?? "",
          maskedDirectUrl: presentUrl(result.body.masked_direct_url),
          maskedPooledUrl: presentUrl(result.body.masked_pooled_url),
        });
        setConnectionError("");
        return;
      }
      setConnection(null);
      setConnectionError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setConnection(null);
      setConnectionError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setLoadingConnection(false);
      }
    }
  }

  async function handleReveal() {
    if (!selected || revealing || ticket) {
      return;
    }
    revealAbort.current?.abort();
    const controller = new AbortController();
    revealAbort.current = controller;
    setRevealing(true);
    setRevealError("");
    try {
      const result = await revealPostgresConnection(selected, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setTicket(null);
        setRevealError(sessionExpired);
        return;
      }
      if (result.status === 404) {
        setTicket(null);
        setRevealError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 200) {
        const shown = parsePostgresCredential(result.body.credential);
        if (!shown) {
          setRevealError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        setTicket(shown);
        setRevealError("");
        return;
      }
      setTicket(null);
      setRevealError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setTicket(null);
      setRevealError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setRevealing(false);
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

  function openTable(table: SelectedTable) {
    if (!selected) {
      return;
    }
    rowsAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setSelectedTable(table);
    setQueryDraft("");
    setAppliedQuery("");
    setRowsQueryError("");
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    void loadRows(selected, table.schema, table.name, "", 0, controller);
  }

  function closeTable() {
    rowsAbort.current?.abort();
    clearRowState();
  }

  function applySearch() {
    if (!selected || !selectedTable) {
      return;
    }
    if (queryRuneCount(queryDraft) > maxRowQueryRunes) {
      setRowsQueryError("Query is too long");
      return;
    }
    rowsAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setAppliedQuery(queryDraft);
    setRowsQueryError("");
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    void loadRows(selected, selectedTable.schema, selectedTable.name, queryDraft, 0, controller);
  }

  function pageRows(nextOffset: number) {
    if (!selected || !selectedTable || !rowPage) {
      return;
    }
    rowsAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    void loadRows(selected, selectedTable.schema, selectedTable.name, appliedQuery, nextOffset, controller);
  }

  async function loadRows(
    db: string,
    schema: string,
    table: string,
    q: string,
    offset: number,
    controller: AbortController,
  ) {
    try {
      const result = await fetchPostgresRows(db, schema, table, { q, offset }, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.columns) && Array.isArray(result.body.rows)) {
        setRowPage(result.body);
        setRowsError("");
        setRowsQueryError("");
        return;
      }
      setRowPage(null);
      if (result.status === 400 && result.body.error?.fields?.q) {
        setRowsQueryError(errorMessage(result.body, "Query is too long"));
        setRowsError("");
        return;
      }
      if (result.status === 404) {
        setRowsError(errorMessage(result.body, "Not found"));
        return;
      }
      setRowsError(errorMessage(result.body, "PostgreSQL is unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setRowPage(null);
      setRowsError("PostgreSQL is unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingRows(false);
      }
    }
  }

  const showReveal =
    !loadingDetails &&
    !loadingConnection &&
    connectionError === "" &&
    connection?.savedCredentialStatus === "present";

  return (
    <article>
      <header className="page-header">
        <h1>Databases</h1>
        <p>Manageable project databases only. Passwords are not revealed.</p>
      </header>
      {ticket ? <CredentialTicket kind="postgres" credential={ticket} onDismiss={clearTicket} /> : null}
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
          aria-busy={loadingDetails || loadingConnection || loadingTables || loadingRows}
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
          {loadingConnection ? (
            <p className="muted-copy" role="status">
              Loading connection.
            </p>
          ) : null}
          {connectionError ? (
            <p className="form-warning" role="alert">
              {connectionError}
            </p>
          ) : null}
          {connection ? <ConnectionFacts urls={connection} /> : null}
          {revealError ? (
            <p className="form-warning" role="alert">
              {revealError}
            </p>
          ) : null}
          {showReveal ? (
            <div className="form-actions">
              <button
                type="button"
                className="text-button"
                disabled={revealing || ticket !== null}
                onClick={() => void handleReveal()}
              >
                Reveal
              </button>
            </div>
          ) : null}
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
          {tables && tables.length > 0 ? (
            <TableNameList
              tables={tables}
              selected={selectedTable}
              onSelect={openTable}
              onBack={closeTable}
              rows={
                selectedTable ? (
                  <RowsPanel
                    regionRef={rowsRegionRef}
                    table={selectedTable}
                    page={rowPage}
                    error={rowsError}
                    queryError={rowsQueryError}
                    loading={loadingRows}
                    queryDraft={queryDraft}
                    onQueryDraftChange={setQueryDraft}
                    onSearch={applySearch}
                    onPrevious={() => pageRows(Math.max(0, (rowPage?.offset ?? 0) - (rowPage?.limit ?? 0)))}
                    onNext={() => pageRows((rowPage?.offset ?? 0) + (rowPage?.limit ?? 0))}
                  />
                ) : null
              }
            />
          ) : null}
        </section>
      ) : null}
    </article>
  );
}

function TableNameList({
  tables,
  selected,
  onSelect,
  onBack,
  rows,
}: {
  tables: TableItem[];
  selected: SelectedTable | null;
  onSelect: (table: SelectedTable) => void;
  onBack: () => void;
  rows: ReactNode;
}) {
  return (
    <>
      {selected ? (
        <p>
          <button type="button" className="text-button" onClick={onBack}>
            Back to tables
          </button>
        </p>
      ) : null}
      <ul className={selected ? "table-name-list table-name-list-inspecting" : "table-name-list"}>
        {tables.flatMap((table) => {
          const schema = table.schema ?? "";
          const name = table.name ?? "";
          if (!schema || !name) {
            return [];
          }
          const active = selected?.schema === schema && selected?.name === name;
          return [
            <li key={`${schema}.${name}`} className={active ? "is-selected" : undefined}>
              <button
                type="button"
                className={active ? "table-name-item table-name-item-active" : "table-name-item"}
                aria-label={`Schema ${schema} Table ${name}`}
                aria-current={active ? "true" : undefined}
                onClick={() => onSelect({ schema, name })}
              >
                <span>
                  <span className="muted-copy">Schema </span>
                  <span className="identifier">{schema}</span>
                </span>
                <span>
                  <span className="muted-copy">Table </span>
                  <span className="identifier">{name}</span>
                </span>
              </button>
            </li>,
            active ? (
              <li key={`${schema}.${name}.rows`} className="table-rows-slot is-selected">
                {rows}
              </li>
            ) : null,
          ];
        })}
      </ul>
    </>
  );
}

function RowsPanel({
  regionRef,
  table,
  page,
  error,
  queryError,
  loading,
  queryDraft,
  onQueryDraftChange,
  onSearch,
  onPrevious,
  onNext,
}: {
  regionRef: RefObject<HTMLElement | null>;
  table: SelectedTable;
  page: RowPage | null;
  error: string;
  queryError: string;
  loading: boolean;
  queryDraft: string;
  onQueryDraftChange: (value: string) => void;
  onSearch: () => void;
  onPrevious: () => void;
  onNext: () => void;
}) {
  const columns = page?.columns ?? [];
  const rows = page?.rows ?? [];
  const offset = page?.offset ?? 0;
  const total = page?.total ?? 0;
  const previousDisabled = page == null || offset === 0;
  const nextDisabled = page == null || offset + rows.length >= total;
  const range = rows.length > 0 ? `${offset + 1}–${offset + rows.length} of ${total}` : "";

  return (
    <section
      ref={regionRef}
      className="rows-region"
      aria-label={`Rows for ${table.schema}.${table.name}`}
      aria-busy={loading}
    >
      <h3>
        <span className="identifier">{table.schema}</span>
        <span className="muted-copy">.</span>
        <span className="identifier">{table.name}</span>
      </h3>
      <form
        className="row-search"
        onSubmit={(event) => {
          event.preventDefault();
          onSearch();
        }}
      >
        <div className="row-search-field">
          <label htmlFor="row-query">Search rows</label>
          <input
            id="row-query"
            autoComplete="off"
            value={queryDraft}
            onChange={(event) => onQueryDraftChange(event.target.value)}
            aria-invalid={queryError ? true : undefined}
            aria-describedby={queryError ? "row-query-hint row-query-error" : "row-query-hint"}
          />
          <p id="row-query-hint" className="muted-copy">
            Maximum 128 code points. Apply to search.
          </p>
        </div>
        <button type="submit" className="text-button">
          Apply
        </button>
      </form>
      {queryError ? (
        <p id="row-query-error" className="form-error" role="alert">
          {queryError}
        </p>
      ) : null}
      {loading ? (
        <p className="muted-copy" role="status">
          Loading rows.
        </p>
      ) : null}
      {error ? (
        <p className="form-warning" role="alert">
          {error}
        </p>
      ) : null}
      {page && rows.length === 0 && !error ? (
        <p className="muted-copy" role="status">
          No rows.
        </p>
      ) : null}
      {page && rows.length > 0 && !error ? (
        <RowGrid schema={table.schema} table={table.name} columns={columns} rows={rows} />
      ) : null}
      {range ? (
        <p className="muted-copy" role="status">
          {range}
        </p>
      ) : null}
      {page && !error ? (
        <div className="row-pager">
          <button type="button" className="text-button" disabled={previousDisabled} onClick={onPrevious}>
            Previous
          </button>
          <button type="button" className="text-button" disabled={nextDisabled} onClick={onNext}>
            Next
          </button>
        </div>
      ) : null}
    </section>
  );
}

function RowGrid({
  schema,
  table,
  columns,
  rows,
}: {
  schema: string;
  table: string;
  columns: string[];
  rows: Array<Record<string, unknown>>;
}) {
  return (
    <div className="row-grid-wrap">
      <table className="row-grid">
        <caption className="visually-hidden">
          Rows for {schema}.{table}
        </caption>
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column} scope="col">
                <span className="identifier">{column}</span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index}>
              {columns.map((column) => {
                const cell = formatCell(row[column]);
                return (
                  <td key={column} className={cell.nullish ? "muted-copy" : "identifier"}>
                    {cell.text}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatCell(value: unknown): { text: string; nullish: boolean } {
  if (value === null || value === undefined) {
    return { text: "Null", nullish: true };
  }
  if (typeof value === "boolean" || typeof value === "number" || typeof value === "string") {
    return { text: String(value), nullish: false };
  }
  try {
    return { text: JSON.stringify(value), nullish: false };
  } catch {
    return { text: "Null", nullish: true };
  }
}

function ConnectionFacts({ urls }: { urls: ConnectionUrls }) {
  const directUrl = urls.maskedDirectUrl;
  const pooledUrl = urls.maskedPooledUrl;
  if (!directUrl && !pooledUrl) {
    return null;
  }
  return (
    <dl className="fact-list">
      {directUrl ? (
        <div>
          <dt>Direct URL</dt>
          <dd className="bidi-isolate identifier">{displayText(directUrl)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(directUrl)}>
            Copy Direct URL
          </button>
        </div>
      ) : null}
      {pooledUrl ? (
        <div>
          <dt>Pooled URL</dt>
          <dd className="bidi-isolate identifier">{displayText(pooledUrl)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(pooledUrl)}>
            Copy Pooled URL
          </button>
        </div>
      ) : null}
    </dl>
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
      <Fact label="Saved credential" value={savedCredentialCopy(details.saved_credential?.status)} />
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

function savedCredentialCopy(status: string | undefined): string {
  if (status === "present") {
    return "Saved";
  }
  if (status === "missing") {
    return "Not saved";
  }
  return "Not available";
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
