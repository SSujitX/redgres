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
  reason?: string;
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

export type RedisCreateCredential = {
  username?: string;
  password?: string;
  one_time?: boolean;
  urls?: { primary?: string };
};

export type RedisCreateUserPayload = {
  resource?: { type?: string; name?: string };
  user?: RedisAclUserListItem;
  credential?: RedisCreateCredential;
  request_id?: string;
};

export type RedisAclPreset = "cache-read-write" | "read-only" | "queue-worker";
export type RedisAclQueueKind = "lists" | "streams" | "sorted-sets";

export type RedisCreateUserOptions = {
  preset: RedisAclPreset;
  queueKind?: RedisAclQueueKind;
};

export async function createRedisUser(
  username: string,
  keyPattern: string,
  csrf: string,
  options: RedisCreateUserOptions,
  init: RequestInit = {},
) {
  const body: {
    username: string;
    key_pattern: string;
    preset: RedisAclPreset;
    queue_kind?: RedisAclQueueKind;
  } = {
    username,
    key_pattern: keyPattern,
    preset: options.preset,
  };
  if (options.preset === "queue-worker") {
    body.queue_kind = options.queueKind ?? "lists";
  }
  return apiRequest<RedisCreateUserPayload & ApiErrorBody>("/api/v1/redis/users", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify(body),
  });
}

export type RedisToggleUserPayload = {
  user?: RedisAclUserDetail;
  request_id?: string;
};

export async function enableRedisUser(username: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<RedisToggleUserPayload & ApiErrorBody>(
    `/api/v1/redis/users/${encodeURIComponent(username)}/enable`,
    {
      ...init,
      method: "POST",
      csrf,
    },
  );
}

export async function disableRedisUser(username: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<RedisToggleUserPayload & ApiErrorBody>(
    `/api/v1/redis/users/${encodeURIComponent(username)}/disable`,
    {
      ...init,
      method: "POST",
      csrf,
    },
  );
}

export type RedisRotateUserPayload = RedisCreateUserPayload;

export async function rotateRedisUser(username: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<RedisRotateUserPayload & ApiErrorBody>(
    `/api/v1/redis/users/${encodeURIComponent(username)}/credentials/rotate`,
    {
      ...init,
      method: "POST",
      csrf,
    },
  );
}

export type RedisPatchUserPayload = RedisToggleUserPayload;

export type RedisPatchUserOptions = {
  keyPattern: string;
  preset: RedisAclPreset;
  queueKind?: RedisAclQueueKind;
};

export async function patchRedisUser(
  username: string,
  csrf: string,
  options: RedisPatchUserOptions,
  init: RequestInit = {},
) {
  const body: {
    key_pattern: string;
    preset: RedisAclPreset;
    queue_kind?: RedisAclQueueKind;
  } = {
    key_pattern: options.keyPattern,
    preset: options.preset,
  };
  if (options.preset === "queue-worker") {
    body.queue_kind = options.queueKind ?? "lists";
  }
  return apiRequest<RedisPatchUserPayload & ApiErrorBody>(
    `/api/v1/redis/users/${encodeURIComponent(username)}`,
    {
      ...init,
      method: "PATCH",
      csrf,
      body: JSON.stringify(body),
    },
  );
}

export { errorMessage };
