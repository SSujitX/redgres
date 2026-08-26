import { apiRequest, type ApiErrorBody } from "./client";

export type OperationResult = {
  database?: string;
  owner?: string;
  source?: string;
};

export type OperationError = {
  code?: string;
  message?: string;
  fields?: Record<string, string>;
};

export type OperationRecord = {
  id?: string;
  action?: string;
  status?: string;
  phase?: string;
  actor?: string;
  target?: string;
  accepted_request_id?: string;
  result?: OperationResult;
  error?: OperationError;
  created_at?: string;
  updated_at?: string;
  started_at?: string | null;
  finished_at?: string | null;
};

export type OperationPayload = {
  operation?: OperationRecord;
  request_id?: string;
};

export async function fetchOperation(id: string, init: RequestInit = {}) {
  return apiRequest<OperationPayload & ApiErrorBody>(`/api/v1/operations/${encodeURIComponent(id)}`, init);
}
