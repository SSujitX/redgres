import { useEffect, useState } from "react";
import {
  errorMessage,
  fetchPostgresSecurity,
  type PostgresSecurityConnection,
  type PostgresSecurityDatabase,
  type PostgresSecuritySummary,
} from "../../api/postgres";
import { displayText } from "../../text/displayText";

const sessionExpired = "Your session has expired. Sign in again to continue.";
const postgresUnavailable = "PostgreSQL is unavailable";
const truncatedCopy = "Security overview truncated at 500 databases or connection groups.";

type OverviewData = {
  summary: PostgresSecuritySummary;
  databases: PostgresSecurityDatabase[];
  connections: PostgresSecurityConnection[];
  truncated: boolean;
};

type View =
  | { kind: "loading" }
  | { kind: "session_expired" }
  | { kind: "unavailable"; copy: string }
  | { kind: "ok"; data: OverviewData };

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value == null || typeof value !== "object") {
    return null;
  }
  return value as Record<string, unknown>;
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function optionalBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function stringField(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function parseDatabase(raw: unknown): PostgresSecurityDatabase | null {
  const record = asRecord(raw);
  if (!record) {
    return null;
  }
  const name = stringField(record, "name");
  if (name === "") {
    return null;
  }
  return {
    name,
    owner: stringField(record, "owner"),
    protected: record.protected === true,
    public_can_connect: optionalBoolean(record.public_can_connect),
    owner_is_superuser: optionalBoolean(record.owner_is_superuser),
    owner_can_login: optionalBoolean(record.owner_can_login),
    owner_createdb: optionalBoolean(record.owner_createdb),
    owner_createrole: optionalBoolean(record.owner_createrole),
    owner_replication: optionalBoolean(record.owner_replication),
    active_connections: finiteNumber(record.active_connections),
  };
}

function parseConnection(raw: unknown): PostgresSecurityConnection | null {
  const record = asRecord(raw);
  if (!record) {
    return null;
  }
  return {
    database: stringField(record, "database"),
    user: stringField(record, "user"),
    client: stringField(record, "client"),
    application: stringField(record, "application"),
    state: stringField(record, "state"),
    count: finiteNumber(record.count),
  };
}

function parseSummary(raw: unknown): PostgresSecuritySummary {
  const record = asRecord(raw);
  if (!record) {
    return {};
  }
  return {
    database_count: finiteNumber(record.database_count),
    public_connect_count: finiteNumber(record.public_connect_count),
    active_connection_count: finiteNumber(record.active_connection_count),
    connection_group_count: finiteNumber(record.connection_group_count),
  };
}

function IsolatedId({ value }: { value: string }) {
  return <span className="bidi-isolate identifier">{displayText(value)}</span>;
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

function metricText(value: number | undefined): string {
  return value == null ? "—" : String(value);
}

export default function SecurityOverview() {
  const [view, setView] = useState<View>({ kind: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    fetchPostgresSecurity({ signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 401) {
          setView({ kind: "session_expired" });
          return;
        }
        if (result.status === 200 && Array.isArray(result.body.databases) && Array.isArray(result.body.connections)) {
          setView({
            kind: "ok",
            data: {
              summary: parseSummary(result.body.summary),
              databases: result.body.databases.flatMap((row) => {
                const parsed = parseDatabase(row);
                return parsed ? [parsed] : [];
              }),
              connections: result.body.connections.flatMap((row) => {
                const parsed = parseConnection(row);
                return parsed ? [parsed] : [];
              }),
              truncated: result.body.truncated === true,
            },
          });
          return;
        }
        setView({ kind: "unavailable", copy: errorMessage(result.body, postgresUnavailable) });
      })
      .catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          return;
        }
        setView({ kind: "unavailable", copy: postgresUnavailable });
      });
    return () => {
      controller.abort();
    };
  }, []);

  const alertCopy =
    view.kind === "session_expired" ? sessionExpired : view.kind === "unavailable" ? view.copy : "";

  return (
    <article>
      <header className="page-header">
        <h1>Security overview</h1>
        <p>
          All non-template databases, including protected names. Saved credentials are not loaded in this
          slice. Rotation is not available.
        </p>
      </header>
      {alertCopy ? (
        <p className="form-warning" role="alert">
          {alertCopy}
        </p>
      ) : null}
      {view.kind === "loading" ? (
        <p className="muted-copy" role="status">
          Loading security overview.
        </p>
      ) : null}
      {view.kind === "ok" ? <OverviewBody data={view.data} /> : null}
    </article>
  );
}

function OverviewBody({ data }: { data: OverviewData }) {
  return (
    <>
      <dl className="fact-list">
        <Fact label="Databases" value={metricText(data.summary.database_count)} metric />
        <Fact label="PUBLIC CONNECT" value={metricText(data.summary.public_connect_count)} metric />
        <Fact label="Active connections" value={metricText(data.summary.active_connection_count)} metric />
        <Fact label="Connection groups" value={metricText(data.summary.connection_group_count)} metric />
        <Fact label="Saved credential" value="Not available" />
      </dl>
      {data.truncated ? (
        <p className="form-warning" role="alert">
          {truncatedCopy}
        </p>
      ) : null}
      <h2>Databases</h2>
      {data.databases.length === 0 ? (
        <p className="muted-copy">No databases.</p>
      ) : (
        <DatabaseLedger rows={data.databases} />
      )}
      <h2>Connection groups</h2>
      {data.connections.length === 0 ? (
        <p className="muted-copy">No connection groups.</p>
      ) : (
        <ConnectionLedger rows={data.connections} />
      )}
    </>
  );
}

function Fact({ label, value, metric }: { label: string; value: string; metric?: boolean }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={metric ? "metric" : undefined}>{value}</dd>
    </div>
  );
}

function DatabaseLedger({ rows }: { rows: PostgresSecurityDatabase[] }) {
  return (
    <div className="audit-results">
      <div className="audit-grid-wrap">
        <table className="audit-grid">
          <caption className="visually-hidden">Database security</caption>
          <thead>
            <tr>
              <th scope="col">Database</th>
              <th scope="col">Owner</th>
              <th scope="col">Protected</th>
              <th scope="col">PUBLIC CONNECT</th>
              <th scope="col">Superuser</th>
              <th scope="col">Can log in</th>
              <th scope="col">Create databases</th>
              <th scope="col">Create roles</th>
              <th scope="col">Replication</th>
              <th scope="col">Connections</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={row.name || index}>
                <td>
                  <IsolatedId value={row.name ?? ""} />
                </td>
                <td>
                  <IsolatedId value={row.owner ?? ""} />
                </td>
                <td>{row.protected ? <span className="ledger-badge">Protected</span> : "—"}</td>
                <td>{yesNo(row.public_can_connect)}</td>
                <td>{yesNo(row.owner_is_superuser)}</td>
                <td>{yesNo(row.owner_can_login)}</td>
                <td>{yesNo(row.owner_createdb)}</td>
                <td>{yesNo(row.owner_createrole)}</td>
                <td>{yesNo(row.owner_replication)}</td>
                <td className="metric">{metricText(row.active_connections)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ol className="audit-stack" aria-label="Database security">
        {rows.map((row, index) => (
          <li key={row.name || index} className="audit-stack-item">
            <dl>
              <div>
                <dt>Database</dt>
                <dd>
                  <IsolatedId value={row.name ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Owner</dt>
                <dd>
                  <IsolatedId value={row.owner ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Protected</dt>
                <dd>{row.protected ? <span className="ledger-badge">Protected</span> : "—"}</dd>
              </div>
              <div>
                <dt>PUBLIC CONNECT</dt>
                <dd>{yesNo(row.public_can_connect)}</dd>
              </div>
              <div>
                <dt>Owner is superuser</dt>
                <dd>{yesNo(row.owner_is_superuser)}</dd>
              </div>
              <div>
                <dt>Owner can log in</dt>
                <dd>{yesNo(row.owner_can_login)}</dd>
              </div>
              <div>
                <dt>Owner can create databases</dt>
                <dd>{yesNo(row.owner_createdb)}</dd>
              </div>
              <div>
                <dt>Owner can create roles</dt>
                <dd>{yesNo(row.owner_createrole)}</dd>
              </div>
              <div>
                <dt>Owner replication</dt>
                <dd>{yesNo(row.owner_replication)}</dd>
              </div>
              <div>
                <dt>Connections</dt>
                <dd className="metric">{metricText(row.active_connections)}</dd>
              </div>
            </dl>
          </li>
        ))}
      </ol>
    </div>
  );
}

function ConnectionLedger({ rows }: { rows: PostgresSecurityConnection[] }) {
  return (
    <div className="audit-results">
      <div className="audit-grid-wrap">
        <table className="audit-grid">
          <caption className="visually-hidden">Connection groups</caption>
          <thead>
            <tr>
              <th scope="col">Database</th>
              <th scope="col">Role</th>
              <th scope="col">Client</th>
              <th scope="col">Application</th>
              <th scope="col">State</th>
              <th scope="col">Count</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={`${row.database}-${row.user}-${row.client}-${row.application}-${row.state}-${index}`}>
                <td>
                  <IsolatedId value={row.database ?? ""} />
                </td>
                <td>
                  <IsolatedId value={row.user ?? ""} />
                </td>
                <td>
                  <IsolatedId value={row.client ?? ""} />
                </td>
                <td>
                  <IsolatedId value={row.application ?? ""} />
                </td>
                <td>
                  <IsolatedId value={row.state ?? ""} />
                </td>
                <td className="metric">{metricText(row.count)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ol className="audit-stack" aria-label="Connection groups">
        {rows.map((row, index) => (
          <li
            key={`${row.database}-${row.user}-${row.client}-${row.application}-${row.state}-${index}`}
            className="audit-stack-item"
          >
            <dl>
              <div>
                <dt>Database</dt>
                <dd>
                  <IsolatedId value={row.database ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Role</dt>
                <dd>
                  <IsolatedId value={row.user ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Client</dt>
                <dd>
                  <IsolatedId value={row.client ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Application</dt>
                <dd>
                  <IsolatedId value={row.application ?? ""} />
                </dd>
              </div>
              <div>
                <dt>State</dt>
                <dd>
                  <IsolatedId value={row.state ?? ""} />
                </dd>
              </div>
              <div>
                <dt>Count</dt>
                <dd className="metric">{metricText(row.count)}</dd>
              </div>
            </dl>
          </li>
        ))}
      </ol>
    </div>
  );
}
