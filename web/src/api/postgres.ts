import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type DatabaseListItem = {
  name?: string;
  owner?: string;
};

export type DatabaseListPayload = {
  databases?: DatabaseListItem[];
  truncated?: boolean;
};

export type DatabaseDetails = {
  name?: string;
  owner?: string;
  size?: string;
  size_bytes?: number;
  collation?: string;
  ctype?: string;
  locale_provider?: string;
  locale?: string | null;
  connection_count?: number;
  security?: {
    public_can_connect?: boolean;
    owner_is_superuser?: boolean;
    owner_can_login?: boolean;
    owner_createdb?: boolean;
    owner_createrole?: boolean;
    owner_replication?: boolean;
  };
  saved_credential?: {
    status?: string;
    reason?: string;
  };
};

export async function fetchPostgresDatabases(init: RequestInit = {}) {
  return apiRequest<DatabaseListPayload & ApiErrorBody>("/api/v1/postgres/databases", init);
}

export async function fetchPostgresDatabase(name: string, init: RequestInit = {}) {
  return apiRequest<{ database?: DatabaseDetails } & ApiErrorBody>(
    `/api/v1/postgres/databases/${encodeURIComponent(name)}`,
    init,
  );
}

export type DatabaseConnection = {
  database?: string;
  owner?: string;
  saved_credential?: {
    status?: string;
    reason?: string;
  };
  masked_direct_url?: string;
  masked_pooled_url?: string;
  request_id?: string;
};

export async function fetchPostgresConnection(name: string, init: RequestInit = {}) {
  return apiRequest<DatabaseConnection & ApiErrorBody>(
    `/api/v1/postgres/databases/${encodeURIComponent(name)}/connection`,
    init,
  );
}

export type PostgresRevealCredential = {
  username?: string;
  password?: string;
  one_time?: boolean;
  urls?: {
    direct?: string;
    pooled?: string;
  };
};

export type PostgresRevealPayload = {
  resource?: { type?: string; name?: string };
  credential?: PostgresRevealCredential;
  request_id?: string;
};

export async function revealPostgresConnection(name: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<PostgresRevealPayload & ApiErrorBody>(
    `/api/v1/postgres/databases/${encodeURIComponent(name)}/connection/reveal`,
    {
      ...init,
      method: "POST",
      csrf,
    },
  );
}

export type PostgresCreatePayload = PostgresRevealPayload;

export async function createPostgresDatabase(
  database: string,
  owner: string,
  csrf: string,
  init: RequestInit = {},
) {
  return apiRequest<PostgresCreatePayload & ApiErrorBody>("/api/v1/postgres/databases", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify({ database, owner }),
  });
}

export type TableItem = {
  schema?: string;
  name?: string;
};

export type TableListPayload = {
  tables?: TableItem[];
  truncated?: boolean;
};

export async function fetchPostgresTables(name: string, init: RequestInit = {}) {
  return apiRequest<TableListPayload & ApiErrorBody>(
    `/api/v1/postgres/databases/${encodeURIComponent(name)}/tables`,
    init,
  );
}

export type RowPage = {
  columns?: string[];
  rows?: Array<Record<string, unknown>>;
  total?: number;
  offset?: number;
  limit?: number;
  request_id?: string;
};

export async function fetchPostgresRows(
  db: string,
  schema: string,
  table: string,
  params: { q?: string; offset?: number } = {},
  init: RequestInit = {},
) {
  const search = new URLSearchParams();
  if (params.q !== undefined && params.q !== "") {
    search.set("q", params.q);
  }
  if (params.offset !== undefined && params.offset > 0) {
    search.set("offset", String(params.offset));
  }
  const query = search.toString();
  const path = `/api/v1/postgres/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/rows`;
  return apiRequest<RowPage & ApiErrorBody>(query ? `${path}?${query}` : path, init);
}

export type PostgresSecuritySummary = {
  database_count?: number;
  public_connect_count?: number;
  active_connection_count?: number;
  connection_group_count?: number;
  missing_password_count?: number;
};

export type PostgresSecurityDatabase = {
  name?: string;
  owner?: string;
  protected?: boolean;
  public_can_connect?: boolean;
  owner_is_superuser?: boolean;
  owner_can_login?: boolean;
  owner_createdb?: boolean;
  owner_createrole?: boolean;
  owner_replication?: boolean;
  active_connections?: number;
  rotation_eligible?: boolean;
};

export type PostgresSecurityConnection = {
  database?: string;
  user?: string;
  client?: string;
  application?: string;
  state?: string;
  count?: number;
};

export type PostgresSecurityPayload = {
  summary?: PostgresSecuritySummary;
  saved_credential?: {
    status?: string;
    reason?: string;
  };
  databases?: PostgresSecurityDatabase[];
  connections?: PostgresSecurityConnection[];
  truncated?: boolean;
  request_id?: string;
};

export async function fetchPostgresSecurity(init: RequestInit = {}) {
  return apiRequest<PostgresSecurityPayload & ApiErrorBody>("/api/v1/postgres/security", init);
}

export { errorMessage };
