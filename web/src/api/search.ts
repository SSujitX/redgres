import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type SearchHit = {
  id?: string;
  type?: string;
  label?: string;
};

export type SearchGroup = {
  id?: string;
  label?: string;
  service?: string;
  status?: string;
  truncated?: boolean;
  hits?: SearchHit[];
};

export type SearchPayload = {
  groups?: SearchGroup[];
  limit?: number;
  request_id?: string;
};

export async function fetchSearch(q: string, init: RequestInit = {}) {
  return apiRequest<SearchPayload & ApiErrorBody>(`/api/v1/search?q=${encodeURIComponent(q)}`, init);
}

export { errorMessage };
