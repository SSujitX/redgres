import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type StatusComponent = {
  id?: string;
  state?: string;
  reason?: string;
};

export type StatusPayload = {
  components?: StatusComponent[];
  request_id?: string;
};

export async function fetchStatus(init: RequestInit = {}) {
  return apiRequest<StatusPayload & ApiErrorBody>("/api/v1/status", init);
}

export { errorMessage };
