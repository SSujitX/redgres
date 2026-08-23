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

export async function fetchPostgresDatabases() {
  return apiRequest<DatabaseListPayload & ApiErrorBody>("/api/v1/postgres/databases");
}

export async function fetchPostgresDatabase(name: string) {
  return apiRequest<{ database?: DatabaseDetails } & ApiErrorBody>(
    `/api/v1/postgres/databases/${encodeURIComponent(name)}`,
  );
}

export { errorMessage };
