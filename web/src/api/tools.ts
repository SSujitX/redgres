import { apiRequest, type ApiErrorBody } from "./client";

export type ExpertTool = "pgadmin" | "redisinsight";

export type ToolLaunchPayload = {
  launch_url?: string;
  request_id?: string;
};

export type PgAdminRevealPayload = {
  email?: string;
  password?: string;
  master_password?: string;
  request_id?: string;
};

export async function launchExpertTool(tool: ExpertTool, csrf: string, init: RequestInit = {}) {
  return apiRequest<ToolLaunchPayload & ApiErrorBody>(`/api/v1/tools/${tool}/launch`, {
    ...init,
    method: "POST",
    csrf,
  });
}

export async function revealPgAdminCredentials(csrf: string, init: RequestInit = {}) {
  return apiRequest<PgAdminRevealPayload & ApiErrorBody>("/api/v1/tools/pgadmin/credentials/reveal", {
    ...init,
    method: "POST",
    csrf,
  });
}

export function isLaunchURL(value: unknown): value is string {
  if (typeof value !== "string" || value === "") {
    return false;
  }
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "https:" || parsed.protocol === "http:") &&
      parsed.pathname.endsWith("/__redgres/launch") &&
      parsed.searchParams.get("ticket") !== null
    );
  } catch {
    return false;
  }
}
