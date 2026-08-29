import { afterEach, describe, expect, it, vi } from "vitest";
import { apiRequest, subscribeCsrf } from "./client";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("apiRequest CSRF recovery", () => {
  it("refreshes the session CSRF once after hash-invalid and retries the mutation", async () => {
    const stale = "stale".padEnd(64, "0");
    const next = "next".padEnd(64, "1");
    const published: string[] = [];
    const unsubscribe = subscribeCsrf((csrf) => {
      published.push(csrf);
    });
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      const header = new Headers(init?.headers).get("X-CSRF-Token");
      if (url === "/api/v1/domain/token" && header === stale) {
        return jsonResponse(403, {
          error: { code: "csrf_invalid", message: "CSRF token is invalid" },
        });
      }
      if (url === "/api/v1/session") {
        expect(init?.method ?? "GET").toBe("GET");
        return jsonResponse(200, { csrf_token: next, owner: { username: "admin" } });
      }
      if (url === "/api/v1/domain/token" && header === next) {
        return jsonResponse(200, { ok: true, request_id: "r2" });
      }
      throw new Error(`unexpected ${url} csrf=${header ?? ""}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiRequest<{ ok?: boolean }>("/api/v1/domain/token", {
      method: "POST",
      csrf: stale,
      body: JSON.stringify({ token: "cf-token" }),
    });

    unsubscribe();
    expect(result.status).toBe(200);
    expect(result.body).toEqual({ ok: true, request_id: "r2" });
    expect(published).toEqual([next]);
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("does not retry Origin check failed", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(403, {
        error: { code: "csrf_invalid", message: "Origin check failed" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiRequest("/api/v1/domain/token", {
      method: "POST",
      csrf: "csrf".padEnd(64, "0"),
      body: JSON.stringify({ token: "cf-token" }),
    });

    expect(result.status).toBe(403);
    expect(result.body).toEqual({
      error: { code: "csrf_invalid", message: "Origin check failed" },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("returns the hash-invalid error when session refresh fails", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const url = String(input);
      if (url === "/api/v1/domain/token") {
        return jsonResponse(403, {
          error: { code: "csrf_invalid", message: "CSRF token is invalid" },
        });
      }
      if (url === "/api/v1/session") {
        return jsonResponse(401, {
          error: { code: "unauthorized", message: "Authentication required" },
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiRequest("/api/v1/domain/token", {
      method: "POST",
      csrf: "csrf".padEnd(64, "0"),
      body: JSON.stringify({ token: "cf-token" }),
    });

    expect(result.status).toBe(403);
    expect(result.body).toEqual({
      error: { code: "csrf_invalid", message: "CSRF token is invalid" },
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not loop when the retried mutation is still hash-invalid", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const url = String(input);
      if (url === "/api/v1/domain/token") {
        return jsonResponse(403, {
          error: { code: "csrf_invalid", message: "CSRF token is invalid" },
        });
      }
      if (url === "/api/v1/session") {
        return jsonResponse(200, { csrf_token: "next".padEnd(64, "1") });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiRequest("/api/v1/domain/token", {
      method: "POST",
      csrf: "stale".padEnd(64, "0"),
      body: JSON.stringify({ token: "cf-token" }),
    });

    expect(result.status).toBe(403);
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });
});
