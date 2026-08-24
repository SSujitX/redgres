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

export type RedisAclUserListItem = {
  username?: string;
  enabled?: boolean;
  key_pattern?: string;
  preset?: string;
  protected?: boolean;
  rule_fidelity?: string;
};

export type RedisAclUsersPayload = {
  state?: string;
  users?: RedisAclUserListItem[];
  truncated?: boolean;
  reason?: string;
  request_id?: string;
};

export type RedisAclUserDetail = RedisAclUserListItem & {
  queue_kind?: string;
  commands?: string[];
  categories?: string[];
};

export type RedisAclUserDetailPayload = {
  state?: string;
  user?: RedisAclUserDetail;
  request_id?: string;
};

export async function fetchRedisUsers(init: RequestInit = {}) {
  return apiRequest<RedisAclUsersPayload & ApiErrorBody>("/api/v1/redis/users", init);
}

export async function fetchRedisUser(username: string, init: RequestInit = {}) {
  return apiRequest<RedisAclUserDetailPayload & ApiErrorBody>(
    `/api/v1/redis/users/${encodeURIComponent(username)}`,
    init,
  );
}

export { errorMessage };
