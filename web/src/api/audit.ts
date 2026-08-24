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

export async function fetchAuditEvents(cursor?: string, init: RequestInit = {}) {
  const path = cursor ? `/api/v1/audit?cursor=${encodeURIComponent(cursor)}` : "/api/v1/audit";
  return apiRequest<AuditPage & ApiErrorBody>(path, init);
}

export { errorMessage };
