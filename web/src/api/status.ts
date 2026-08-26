import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type StatusComponent = {
  id: "redgres_state" | "postgres_direct" | "pgbouncer" | "redis" | "tool_links";
  state: "ok" | "unavailable" | "not_configured";
  reason?: string;
};

export type StatusPayload = {
  components: StatusComponent[];
  request_id: string;
};

export async function fetchStatus(init: RequestInit = {}) {
  return apiRequest<StatusPayload & ApiErrorBody>("/api/v1/status", init);
}

const componentIDs = ["redgres_state", "postgres_direct", "pgbouncer", "redis", "tool_links"] as const;
const componentStates = {
  redgres_state: new Set(["ok", "unavailable"]),
  postgres_direct: new Set(["ok", "unavailable", "not_configured"]),
  pgbouncer: new Set(["ok", "unavailable", "not_configured"]),
  redis: new Set(["ok", "unavailable", "not_configured"]),
  tool_links: new Set(["ok", "not_configured"]),
} as const;
const componentKeys = new Set(["id", "state", "reason"]);
const payloadKeys = new Set(["components", "request_id"]);
const requestIDPattern = /^[0-9a-f]{32}$/;

function isStatusComponents(value: unknown): value is StatusComponent[] {
  if (!Array.isArray(value) || value.length !== componentIDs.length) {
    return false;
  }
  return value.every((component, index) => {
    if (component === null || typeof component !== "object" || Array.isArray(component)) {
      return false;
    }
    const record = component as Record<string, unknown>;
    if (Object.keys(record).some((key) => !componentKeys.has(key))) {
      return false;
    }
    const expectedID = componentIDs[index];
    if (
      record.id !== expectedID ||
      typeof record.state !== "string" ||
      !componentStates[expectedID].has(record.state)
    ) {
      return false;
    }
    return record.state === "unavailable" ? record.reason === "unreachable" : record.reason === undefined;
  });
}

export function isStatusPayload(value: unknown): value is StatusPayload {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const record = value as Record<string, unknown>;
  if (Object.keys(record).some((key) => !payloadKeys.has(key))) {
    return false;
  }
  return (
    typeof record.request_id === "string" &&
    requestIDPattern.test(record.request_id) &&
    isStatusComponents(record.components)
  );
}

export { errorMessage };
