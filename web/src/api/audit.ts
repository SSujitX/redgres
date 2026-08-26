import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type AuditEvent = {
  id: number;
  actor?: string;
  action?: string;
  target?: string;
  outcome?: string;
  request_id?: string;
  client_ip?: string;
  created_at?: string;
};

export type AuditPage = {
  events?: AuditEvent[];
  has_more?: boolean;
  next_cursor?: string;
  limit?: number;
  request_id?: string;
};

export type FetchAuditEventsQuery = {
  cursor?: string;
  limit?: number;
};

export async function fetchAuditEvents(query?: string | FetchAuditEventsQuery, init: RequestInit = {}) {
  const cursor = typeof query === "string" ? query : query?.cursor;
  const limit = typeof query === "object" && query != null ? query.limit : undefined;
  const params = new URLSearchParams();
  if (cursor) {
    params.set("cursor", cursor);
  }
  if (limit != null) {
    params.set("limit", String(limit));
  }
  const qs = params.toString();
  const path = qs === "" ? "/api/v1/audit" : `/api/v1/audit?${qs}`;
  return apiRequest<AuditPage & ApiErrorBody>(path, init);
}

export { errorMessage };
