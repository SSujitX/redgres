import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type RedisStatusMetrics = {
  version: string;
  uptime_seconds: number;
  connected_clients: number;
  used_memory_bytes: number;
  max_memory_bytes: number;
  ops_per_sec: number;
  db_size: number;
  latency_ms: number;
};

export type RedisStatusPayload = {
  state?: string;
  reason?: string;
  metrics?: RedisStatusMetrics;
  request_id?: string;
};

export async function fetchRedisStatus(init: RequestInit = {}) {
  return apiRequest<RedisStatusPayload & ApiErrorBody>("/api/v1/redis/status", init);
}

export { errorMessage };
