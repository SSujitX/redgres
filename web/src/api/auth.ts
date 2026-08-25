import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type ToolLinks = {
  pgadmin?: string;
  redisinsight?: string;
};

export type SessionPayload = {
  owner?: { username?: string };
  csrf_token?: string;
  tool_links?: ToolLinks;
};

export type LoginSuccess = {
  owner?: { username?: string };
  csrf_token?: string;
};

export function parseToolLinks(raw: unknown): ToolLinks {
  if (raw == null || typeof raw !== "object" || Array.isArray(raw)) {
    return {};
  }
  const source = raw as Record<string, unknown>;
  const parsed: ToolLinks = {};
  if (typeof source.pgadmin === "string" && source.pgadmin !== "") {
    parsed.pgadmin = source.pgadmin;
  }
  if (typeof source.redisinsight === "string" && source.redisinsight !== "") {
    parsed.redisinsight = source.redisinsight;
  }
  return parsed;
}

export async function fetchSession() {
  return apiRequest<SessionPayload & ApiErrorBody>("/api/v1/session");
}

export async function login(username: string, password: string) {
  return apiRequest<LoginSuccess & ApiErrorBody>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function logout(csrf: string) {
  return apiRequest<ApiErrorBody>("/api/v1/auth/logout", {
    method: "POST",
    csrf,
  });
}

export { errorMessage };
