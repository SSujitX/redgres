import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";

afterEach(() => {
  vi.unstubAllGlobals();
})

function jsonResponse(status: number, body: unknown, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(headers),
    json: async () => body,
  };
}

function isTablesUrl(url: string, name: string): boolean {
  return (
    url.includes(`/api/v1/postgres/databases/${encodeURIComponent(name)}/tables`) && !url.includes("/rows")
  );
}

function isRowsUrl(url: string, db: string, schema: string, table: string): boolean {
  return url.includes(
    `/api/v1/postgres/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/rows`,
  );
}

function isDetailsUrl(url: string, name: string): boolean {
  const prefix = `/api/v1/postgres/databases/${encodeURIComponent(name)}`;
  return url.includes(prefix) && !url.includes("/tables");
}

function isAuditUrl(url: string): boolean {
  return url === "/api/v1/audit" || url.startsWith("/api/v1/audit?");
}

function isStatusUrl(url: string): boolean {
  return url === "/api/v1/status" || url.startsWith("/api/v1/status?");
}

function isRedisStatusUrl(url: string): boolean {
  return url === "/api/v1/redis/status";
}

function isSearchUrl(url: string): boolean {
  return url === "/api/v1/search" || url.startsWith("/api/v1/search?");
}

function disconnectedStatus() {
  return jsonResponse(200, {
    components: [
      { id: "redgres_state", state: "not_configured" },
      { id: "postgres_direct", state: "not_configured" },
      { id: "pgbouncer", state: "not_implemented" },
      { id: "redis", state: "not_configured" },
      { id: "tool_links", state: "not_configured" },
    ],
    request_id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  });
}

function mixedStatus() {
  return jsonResponse(200, {
    components: [
      { id: "redgres_state", state: "ok" },
      { id: "postgres_direct", state: "unavailable", reason: "unreachable" },
      { id: "pgbouncer", state: "not_implemented" },
      { id: "redis", state: "ok" },
      { id: "tool_links", state: "not_configured" },
    ],
    request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  });
}

function disconnectedRedisStatus() {
  return jsonResponse(200, {
    state: "not_configured",
    request_id: "99999999999999999999999999999999",
  });
}

function redisOkMetrics(extra: Record<string, unknown> = {}) {
  return {
    version: "8.2.1",
    uptime_seconds: 123,
    connected_clients: 4,
    used_memory_bytes: 1048576,
    max_memory_bytes: 0,
    ops_per_sec: 12,
    db_size: 50,
    latency_ms: 1.25,
    ...extra,
  };
}

function redisOkStatus(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    state: "ok",
    metrics: redisOkMetrics(),
    request_id: "22222222222222222222222222222222",
    ...extra,
  });
}

function redisUnavailableStatus(reason: "unreachable" | "auth_failed" | "permission_denied") {
  return jsonResponse(200, {
    state: "unavailable",
    reason,
    request_id: "33333333333333333333333333333333",
  });
}

function overviewOkStatus() {
  return jsonResponse(200, {
    components: [
      { id: "redgres_state", state: "ok" },
      { id: "postgres_direct", state: "ok" },
      { id: "pgbouncer", state: "not_implemented" },
      { id: "redis", state: "ok" },
      { id: "tool_links", state: "not_configured" },
    ],
    request_id: "11111111111111111111111111111111",
  });
}

function disconnectedSearch() {
  return jsonResponse(200, {
    groups: [
      {
        id: "postgres_databases",
        label: "PostgreSQL databases",
        service: "postgres",
        status: "not_configured",
        truncated: false,
        hits: [],
      },
      {
        id: "redis_acl_users",
        label: "Redis ACL users",
        service: "redis",
        status: "not_implemented",
        truncated: false,
        hits: [],
      },
    ],
    limit: 20,
    request_id: "cccccccccccccccccccccccccccccccc",
  });
}

function postgresHitSearch(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    groups: [
      {
        id: "postgres_databases",
        label: "PostgreSQL databases",
        service: "postgres",
        status: "ok",
        truncated: false,
        hits: [
          {
            id: "postgres_database:project_a",
            type: "postgres_database",
            label: "project_a",
            ...extra,
          },
        ],
      },
      {
        id: "redis_acl_users",
        label: "Redis ACL users",
        service: "redis",
        status: "not_implemented",
        truncated: false,
        hits: [],
      },
    ],
    limit: 20,
    request_id: "dddddddddddddddddddddddddddddddd",
  });
}

function unknownApi(url: string) {
  if (isRedisStatusUrl(url)) {
    return disconnectedRedisStatus();
  }
  if (isStatusUrl(url)) {
    return disconnectedStatus();
  }
  if (isSearchUrl(url)) {
    return disconnectedSearch();
  }
  return jsonResponse(500, {});
}

function auditEvent(overrides: Record<string, unknown> = {}) {
  return {
    id: 1421,
    actor: "admin",
    action: "owner.login",
    target: "admin",
    outcome: "success",
    request_id: "aabbccddeeff00112233445566778899",
    client_ip: "127.0.0.1",
    created_at: "2026-08-25T04:11:09.123456789Z",
    ...overrides,
  };
}

function stubFetch(
  impl: (url: string, init?: RequestInit) => ReturnType<typeof jsonResponse> | Promise<ReturnType<typeof jsonResponse>>,
) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.signal?.aborted) {
      throw new DOMException("The operation was aborted.", "AbortError");
    }
    const url = String(input);
    return impl(url, init);
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

describe("App session and login", () => {
  it("shows login after an unauthenticated session and never calls healthz", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Redgres" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Primary" })).not.toBeInTheDocument();
    expect(screen.queryByText(/reachable/i)).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/healthz"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isStatusUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisStatusUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isSearchUrl(String(call[0])))).toBe(true);
  });

  it("shows the shell when the session is valid", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "a".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
  });

  it("shows a generic login failure without leaking the password or error code", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return jsonResponse(401, { error: { code: "unauthorized", message: "Invalid username or password." } });
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "wrong-password-x" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid username or password.");
    expect(screen.queryByText("unauthorized")).not.toBeInTheDocument();
    expect(screen.queryByText("wrong-password-x")).not.toBeInTheDocument();
  });

  it("surfaces lockout Retry-After seconds", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return jsonResponse(
        429,
        { error: { code: "rate_limited", message: "Too many login attempts. Try again later." } },
        { "Retry-After": "12" },
      );
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "wrong-password-x" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Too many login attempts. Try again later.");
    expect(screen.getByRole("status")).toHaveTextContent("Try again in 12 seconds.");
  });

  it("shows the origin check failure message", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return jsonResponse(403, { error: { code: "csrf_invalid", message: "Origin check failed" } });
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Origin check failed");
  });

  it("enters the shell after login and does not persist secrets", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "b".repeat(64) });
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(screen.queryByDisplayValue("owner-secret-15")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    expect(document.cookie).not.toContain("b".repeat(64));
  });

  it("logs out with the CSRF header and returns to login", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "c".repeat(64) });
      }
      if (url.includes("/api/v1/auth/logout")) {
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("c".repeat(64));
        return jsonResponse(200, { ok: true });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "admin" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.some((call) => String(call[0]).includes("/api/v1/auth/logout"))).toBe(true);
  });

  it("filters navigation locally and still calls bounded search", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "d".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "audit" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    expect(dialog.querySelector(".nav-result")).toHaveTextContent("Audit");
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => String(call[0]) === "/api/v1/search?q=audit")).toBe(true);
    });
    const searchCall = fetch.mock.calls.find((call) => String(call[0]) === "/api/v1/search?q=audit");
    const method = searchCall?.[1]?.method;
    expect(method === undefined || method === "GET").toBe(true);
    expect(fetch.mock.calls.every((call) => !isAuditUrl(String(call[0])))).toBe(true);
    expect(screen.getByText(/Redis ACL user search is not available yet/)).toBeInTheDocument();
    expect(screen.queryByText(/No matching Redis ACL users/i)).not.toBeInTheDocument();
  });

  it("hides nested PostgreSQL items until Databases is current", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "f".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    const firstDrawer = screen.getByRole("dialog", { name: "Navigation" });
    expect(within(firstDrawer).queryByRole("button", { name: "Create database" })).not.toBeInTheDocument();
    const databases = within(firstDrawer).getByRole("button", { name: "Databases" });
    expect(databases.querySelector("svg")).not.toBeNull();
    expect(databases).toHaveAttribute("title", "Databases");
    fireEvent.click(databases);
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    expect(
      within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", {
        name: "Create database",
      }),
    ).toBeInTheDocument();
  });

  it("traps focus in the navigation drawer", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "g".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    const dialog = screen.getByRole("dialog", { name: "Navigation" });
    const close = screen.getByRole("button", { name: "Close menu" });
    close.focus();
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toHaveAccessibleName("Overview");
  });

  it("restores focus to Search when the search dialog closes", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "h".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    const search = await screen.findByRole("button", { name: "Search" });
    fireEvent.click(search);
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "audit" } });
    expect(screen.getByRole("status")).toHaveTextContent("1 matching page.");
    fireEvent.click(screen.getByRole("button", { name: "Close search" }));
    await waitFor(() => {
      expect(search).toHaveFocus();
    });
  });

  it("opens a postgres search hit on Databases without mutating", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-pg".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return postgresHitSearch({ password: "canary-secret", url: "postgresql://canary-secret@10.0.0.1/db" });
      }
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables")) {
        if (isDetailsUrl(url, "project_a")) {
          return jsonResponse(200, { database: { name: "project_a", owner: "project_a_role", size: "12 MB" } });
        }
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "project" } });
    const dialog = await screen.findByRole("dialog", { name: "Search" });
    expect(within(dialog).getByRole("region", { name: "PostgreSQL databases" })).toBeInTheDocument();
    expect(within(dialog).getByRole("region", { name: "Redis ACL users" })).toBeInTheDocument();
    expect(within(dialog).getByRole("region", { name: "Navigation" })).toBeInTheDocument();
    expect(within(dialog).getByRole("region", { name: "Documentation" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Searching.");
    expect(screen.getByRole("status").textContent).not.toMatch(/No matching pages/);
    const hit = await screen.findByRole("button", { name: /project_a/ });
    expect(hit.className).toContain("nav-result-postgres");
    expect(screen.getByRole("status")).toHaveTextContent("1 matching database.");
    expect(screen.queryByText(/^No matching pages/)).not.toBeInTheDocument();
    expect(screen.queryByText("canary-secret")).not.toBeInTheDocument();
    expect(dialog.querySelector("input[type=password]")).toBeNull();
    fireEvent.click(hit);
    expect(await screen.findByRole("heading", { name: "Databases" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Database details" })).toBeInTheDocument();
    expect(
      fetch.mock.calls.every((call) => {
        const url = String(call[0]);
        return !/drop|truncate/i.test(url);
      }),
    ).toBe(true);
    expect(
      fetch.mock.calls.every((call) => {
        const method = call[1]?.method;
        return method === undefined || method === "GET";
      }),
    ).toBe(true);
  });

  it("clears stale postgres hits as soon as the query changes", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-stale".padEnd(64, "7") });
      }
      if (isSearchUrl(url)) {
        return postgresHitSearch();
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    const input = screen.getByLabelText("Search pages and databases");
    fireEvent.change(input, { target: { value: "project" } });
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    fireEvent.change(input, { target: { value: "zzz" } });
    expect(screen.queryByRole("button", { name: /project_a/ })).not.toBeInTheDocument();
  });

  it("reactivates the same postgres hit after another database is selected", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-re".padEnd(64, "8") });
      }
      if (isSearchUrl(url)) {
        return postgresHitSearch();
      }
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables")) {
        if (isDetailsUrl(url, "project_a")) {
          return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
        }
        if (isDetailsUrl(url, "project_b")) {
          return jsonResponse(200, { database: { name: "project_b", owner: "owner_b" } });
        }
        return jsonResponse(200, {
          databases: [
            { name: "project_a", owner: "owner_a" },
            { name: "project_b", owner: "owner_b" },
          ],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_a") || isTablesUrl(url, "project_b")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: /project_b/ }));
    expect(await screen.findByRole("heading", { name: "project_b" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
  });

  it("does not send mutations when searching for drop", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-drop".padEnd(64, "x") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "drop" } });
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => String(call[0]) === "/api/v1/search?q=drop")).toBe(true);
    });
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "truncate" } });
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => String(call[0]) === "/api/v1/search?q=truncate")).toBe(true);
    });
    expect(
      fetch.mock.calls.every((call) => {
        const method = call[1]?.method;
        const url = String(call[0]);
        return (method === undefined || method === "GET") && !/drop|truncate/i.test(url.split("?")[0] ?? "");
      }),
    ).toBe(true);
  });

  it("moves focus through search results with arrows and Enter", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-keys".padEnd(64, "1") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    const input = screen.getByLabelText("Search pages and databases");
    fireEvent.change(input, { target: { value: "audit" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    const audit = within(dialog).getByRole("button", { name: /Audit/ });
    input.focus();
    fireEvent.keyDown(dialog, { key: "ArrowDown" });
    expect(audit).toHaveFocus();
    fireEvent.keyDown(audit, { key: "Enter" });
    fireEvent.click(audit);
    expect(await screen.findByRole("heading", { name: "Audit" })).toBeInTheDocument();
  });

  it("aborts an in-flight search when the dialog closes", async () => {
    let finish: ((value: ReturnType<typeof disconnectedSearch>) => void) | undefined;
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-abort".padEnd(64, "2") });
      }
      if (isSearchUrl(url)) {
        return new Promise((resolve) => {
          finish = resolve;
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "ab" } });
    await waitFor(() => {
      expect(finish).toBeDefined();
    });
    fireEvent.click(screen.getByRole("button", { name: "Close search" }));
    finish?.(disconnectedSearch());
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Search" })).not.toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /project_a/ })).not.toBeInTheDocument();
  });

  it("does not fetch search for too-short or too-long queries", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-len".padEnd(64, "3") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    const input = screen.getByLabelText("Search pages and databases");
    fireEvent.change(input, { target: { value: " " } });
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(fetch.mock.calls.every((call) => !isSearchUrl(String(call[0])))).toBe(true);
    fireEvent.change(input, { target: { value: "x".repeat(129) } });
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(fetch.mock.calls.every((call) => !isSearchUrl(String(call[0])))).toBe(true);
    expect(screen.getByRole("status")).toHaveTextContent("Query is too long.");
    expect(input).toHaveAttribute("aria-invalid", "true");
  });

  it("clears postgres hits when search returns 401", async () => {
    let searches = 0;
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-401".padEnd(64, "4") });
      }
      if (isSearchUrl(url)) {
        searches += 1;
        if (searches === 1) {
          return postgresHitSearch();
        }
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    const input = screen.getByLabelText("Search pages and databases");
    fireEvent.change(input, { target: { value: "project" } });
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    fireEvent.change(input, { target: { value: "projec" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("button", { name: /project_a/ })).not.toBeInTheDocument();
  });

  it("keeps local navigation when postgres search is unavailable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-unavail".padEnd(64, "5") });
      }
      if (isSearchUrl(url)) {
        return jsonResponse(200, {
          groups: [
            {
              id: "postgres_databases",
              label: "PostgreSQL databases",
              service: "postgres",
              status: "unavailable",
              truncated: false,
              hits: [],
            },
            {
              id: "redis_acl_users",
              label: "Redis ACL users",
              service: "redis",
              status: "not_implemented",
              truncated: false,
              hits: [],
            },
          ],
          limit: 20,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "audit" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    expect(within(dialog).getByRole("button", { name: /Audit/ })).toBeInTheDocument();
    await waitFor(() => {
      expect(within(dialog).getByText("Unavailable")).toBeInTheDocument();
    });
  });

  it("clears search UI on logout", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-out".padEnd(64, "6") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("search-out".padEnd(64, "6"));
        return jsonResponse(200, { ok: true });
      }
      if (isSearchUrl(url)) {
        return postgresHitSearch();
      }
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables")) {
        if (isDetailsUrl(url, "project_a")) {
          return jsonResponse(200, { database: { name: "project_a", owner: "project_a_role" } });
        }
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages and databases"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Search" })).not.toBeInTheDocument();
    expect(screen.queryByText("project_a")).not.toBeInTheDocument();
  });

  it("shows a generic message when sign-in cannot reach the server", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      throw new TypeError("Failed to fetch");
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Sign-in is unavailable. Try again.");
    expect(screen.queryByText(/control-plane/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveAttribute("aria-invalid", "true");
  });

  it("lists manageable PostgreSQL databases without claiming they are healthy", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "i".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, {
          tables: [{ schema: "public", name: "items" }],
          truncated: false,
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            size: "12 MB",
            connection_count: 3,
            security: {
              public_can_connect: false,
              owner_is_superuser: true,
              owner_can_login: true,
              owner_createdb: true,
              owner_createrole: false,
              owner_replication: false,
            },
            saved_credential: { status: "not_available", reason: "vault_not_implemented" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Not available")).toBeInTheDocument();
    expect(screen.getByText("Owner is superuser").closest("div")).toHaveTextContent("Yes");
    expect(screen.getByText("Owner can create roles").closest("div")).toHaveTextContent("No");
    expect(await screen.findByText("items")).toBeInTheDocument();
    expect(screen.getByText("public")).toHaveClass("identifier");
    expect(screen.getByRole("button", { name: /Schema public Table items/ })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Database details" })).toHaveAttribute("aria-busy", "false");
    expect(screen.queryByText(/healthy/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/reachable/i)).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/rows"))).toBe(true);
  });

  it("clears previous details and ignores a slower first selection", async () => {
    const longName = `project_${"x".repeat(55)}`;
    let releaseA: () => void = () => {};
    const blockedA = new Promise<void>((resolve) => {
      releaseA = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "k".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, {
          databases: [
            { name: "project_a", owner: "owner_a" },
            { name: longName, owner: "owner_b" },
          ],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_a") || isTablesUrl(url, longName)) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        await new Promise<void>((resolve, reject) => {
          if (init?.signal?.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          const onAbort = () => {
            init?.signal?.removeEventListener("abort", onAbort);
            reject(new DOMException("The operation was aborted.", "AbortError"));
          };
          init?.signal?.addEventListener("abort", onAbort);
          void blockedA.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, {
          database: { name: "project_a", owner: "stale_owner_a", size: "1 MB" },
        });
      }
      if (isDetailsUrl(url, longName)) {
        return jsonResponse(200, {
          database: { name: longName, owner: "owner_b", size: "2 MB" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.getByText(longName)).toHaveClass("identifier");
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("status")).toHaveTextContent("Loading details.");
    fireEvent.click(screen.getByRole("button", { name: new RegExp(longName) }));
    expect(screen.queryByText("stale_owner_a")).not.toBeInTheDocument();
    expect(await screen.findByText("owner_b")).toBeInTheDocument();
    releaseA();
    await waitFor(() => {
      expect(screen.queryByText("stale_owner_a")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("heading", { name: longName })).toBeInTheDocument();
  });

  it("shows an empty table list without claiming the database is healthy", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "l".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a", size: "1 MB" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("No tables.")).toBeInTheDocument();
    expect(screen.queryByText("Tables are unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText(/healthy/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/reachable/i)).not.toBeInTheDocument();
  });

  it("shows a tables unavailable alert without an empty healthy table list", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "m".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a", size: "1 MB" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("owner_a")).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByText("No tables.")).not.toBeInTheDocument();
  });

  it("warns when the table list is truncated", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "n".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, {
          tables: [{ schema: "public", name: "items" }],
          truncated: true,
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Table list truncated at 500 tables.")).toBeInTheDocument();
    expect(screen.getByText("items")).toBeInTheDocument();
  });

  it("ignores a slower first table list after selection change", async () => {
    const longSchema = `schema_${"y".repeat(56)}`;
    const longTable = `table_${"z".repeat(57)}`;
    let releaseA: () => void = () => {};
    const blockedA = new Promise<void>((resolve) => {
      releaseA = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "o".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, {
          databases: [
            { name: "project_a", owner: "owner_a" },
            { name: "project_b", owner: "owner_b" },
          ],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_a")) {
        await new Promise<void>((resolve, reject) => {
          if (init?.signal?.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          const onAbort = () => {
            init?.signal?.removeEventListener("abort", onAbort);
            reject(new DOMException("The operation was aborted.", "AbortError"));
          };
          init?.signal?.addEventListener("abort", onAbort);
          void blockedA.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, {
          tables: [{ schema: "stale_schema", name: "stale_items" }],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_b")) {
        return jsonResponse(200, {
          tables: [{ schema: longSchema, name: longTable }],
          truncated: false,
        });
      }
      if (isDetailsUrl(url, "project_a") || isDetailsUrl(url, "project_b")) {
        const name = url.includes("project_b") ? "project_b" : "project_a";
        return jsonResponse(200, { database: { name, owner: name === "project_b" ? "owner_b" : "owner_a" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Loading tables.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(screen.queryByText("stale_items")).not.toBeInTheDocument();
    expect(await screen.findByText(longTable)).toBeInTheDocument();
    expect(screen.getByText(longSchema)).toHaveClass("identifier");
    expect(screen.getByText(longTable)).toHaveClass("identifier");
    releaseA();
    await waitFor(() => {
      expect(screen.queryByText("stale_items")).not.toBeInTheDocument();
    });
  });

  it("shows an unavailable PostgreSQL inventory without a fake empty cluster", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "j".repeat(64) });
      }
      if (url.includes("/api/v1/postgres/databases")) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByText("No manageable project databases.")).not.toBeInTheDocument();
  });

  it("loads bounded rows after a table is activated", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "p".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(200, {
          columns: ["id", "name", "blob", "note"],
          rows: [{ id: 1, name: "a", blob: "\\xdead", note: null }],
          total: 1,
          offset: 0,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByRole("columnheader", { name: "id" })).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("a")).toBeInTheDocument();
    expect(screen.getByText("\\xdead")).toBeInTheDocument();
    expect(screen.getByText("Null")).toBeInTheDocument();
    expect(screen.getByText("1–1 of 1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back to tables" })).toBeInTheDocument();
    const rowsCall = fetch.mock.calls.find((call) => String(call[0]).includes("/rows"));
    expect(rowsCall).toBeDefined();
    expect(String(rowsCall?.[0])).toBe("/api/v1/postgres/databases/project_a/tables/public/items/rows");
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
    fireEvent.click(screen.getByRole("button", { name: "Back to tables" }));
    expect(screen.queryByRole("region", { name: /Rows for/ })).not.toBeInTheDocument();
    expect(screen.queryByText("1–1 of 1")).not.toBeInTheDocument();
  });

  it("renders markup-looking cells as text and keeps q off the location bar", async () => {
    const hrefBefore = window.location.href;
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "y".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(200, {
          columns: ["note"],
          rows: [{ note: "<img src=x onerror=alert(1)>" }],
          total: 1,
          offset: 0,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
    fireEvent.change(screen.getByLabelText("Search rows"), { target: { value: "tokenish" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(window.location.href).toBe(hrefBefore);
    expect(window.location.search).not.toMatch(/[?&]q=/);
  });

  it("shows No rows for an empty existing table", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "q".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(200, { columns: ["id"], rows: [], total: 0, offset: 0, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("No rows.")).toBeInTheDocument();
    expect(screen.queryByText("PostgreSQL is unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText("Not found")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows rows unavailable without an empty healthy grid", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "r".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByText("No rows.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows a not-found alert for a missing table without No rows", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "s".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByText("No rows.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("ignores a slower first row page after table change", async () => {
    let releaseA: () => void = () => {};
    const blockedA = new Promise<void>((resolve) => {
      releaseA = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "t".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, {
          tables: [
            { schema: "public", name: "items" },
            { schema: "public", name: "orders" },
          ],
          truncated: false,
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        await new Promise<void>((resolve, reject) => {
          if (init?.signal?.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          const onAbort = () => {
            init?.signal?.removeEventListener("abort", onAbort);
            reject(new DOMException("The operation was aborted.", "AbortError"));
          };
          init?.signal?.addEventListener("abort", onAbort);
          void blockedA.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, {
          columns: ["id"],
          rows: [{ id: "stale_row" }],
          total: 1,
          offset: 0,
          limit: 50,
        });
      }
      if (isRowsUrl(url, "project_a", "public", "orders")) {
        return jsonResponse(200, {
          columns: ["id"],
          rows: [{ id: "fresh_row" }],
          total: 1,
          offset: 0,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("Loading rows.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Schema public Table orders/ }));
    expect(screen.queryByText("stale_row")).not.toBeInTheDocument();
    expect(await screen.findByText("fresh_row")).toBeInTheDocument();
    releaseA();
    await waitFor(() => {
      expect(screen.queryByText("stale_row")).not.toBeInTheDocument();
    });
  });

  it("clears selected table and rows when the database changes", async () => {
    let releaseA: () => void = () => {};
    const blockedA = new Promise<void>((resolve) => {
      releaseA = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "u".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, {
          databases: [
            { name: "project_a", owner: "owner_a" },
            { name: "project_b", owner: "owner_b" },
          ],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_a") || isTablesUrl(url, "project_b")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a") || isDetailsUrl(url, "project_b")) {
        const name = url.includes("project_b") ? "project_b" : "project_a";
        return jsonResponse(200, { database: { name, owner: name === "project_b" ? "owner_b" : "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        await new Promise<void>((resolve, reject) => {
          if (init?.signal?.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          const onAbort = () => {
            init?.signal?.removeEventListener("abort", onAbort);
            reject(new DOMException("The operation was aborted.", "AbortError"));
          };
          init?.signal?.addEventListener("abort", onAbort);
          void blockedA.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, {
          columns: ["secret"],
          rows: [{ secret: "should-not-paint" }],
          total: 1,
          offset: 0,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("Loading rows.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(screen.queryByText("should-not-paint")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /Rows for/ })).not.toBeInTheDocument();
    releaseA();
    await waitFor(() => {
      expect(screen.queryByText("should-not-paint")).not.toBeInTheDocument();
    });
    expect(await screen.findByRole("heading", { name: "project_b" })).toBeInTheDocument();
    expect(screen.getAllByText("owner_b").length).toBeGreaterThan(0);
  });

  it("rejects an overlong row search before fetching", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "v".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        return jsonResponse(200, { columns: ["id"], rows: [{ id: 1 }], total: 1, offset: 0, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("1")).toBeInTheDocument();
    const before = fetch.mock.calls.filter((call) => String(call[0]).includes("/rows")).length;
    fireEvent.change(screen.getByLabelText("Search rows"), { target: { value: "🙂" + "x".repeat(128) } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(await screen.findByText("Query is too long")).toBeInTheDocument();
    expect(screen.getByLabelText("Search rows")).toHaveAttribute("aria-invalid", "true");
    expect(fetch.mock.calls.filter((call) => String(call[0]).includes("/rows"))).toHaveLength(before);
  });

  it("shows a server query validation error without a healthy grid", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "w".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        if (url.includes("q=")) {
          return jsonResponse(400, {
            error: { code: "validation_error", message: "Query is too long", fields: { q: "too_long" } },
          });
        }
        return jsonResponse(200, { columns: ["id"], rows: [{ id: 1 }], total: 1, offset: 0, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("1")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search rows"), { target: { value: "ok" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(await screen.findByText("Query is too long")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("requests the next row page from the last response offset and limit", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "x".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "owner_a" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [{ schema: "public", name: "items" }], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "owner_a" } });
      }
      if (isRowsUrl(url, "project_a", "public", "items")) {
        if (url.includes("offset=50")) {
          return jsonResponse(200, {
            columns: ["id"],
            rows: [{ id: "page_two" }],
            total: 51,
            offset: 50,
            limit: 50,
          });
        }
        return jsonResponse(200, {
          columns: ["id"],
          rows: [{ id: "page_one" }],
          total: 51,
          offset: 0,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Schema public Table items/ }));
    expect(await screen.findByText("page_one")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("page_two")).toBeInTheDocument();
    const nextCall = fetch.mock.calls.find((call) => String(call[0]).includes("offset=50"));
    expect(nextCall).toBeDefined();
  });

  it("opens and closes the navigation drawer", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "e".repeat(64) });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    expect(screen.getByRole("dialog", { name: "Navigation" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close menu" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Navigation" })).not.toBeInTheDocument();
    });
  });

  it("shows the audit history view instead of the placeholder (AC-1)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-a".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent()],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("heading", { name: "Audit" })).toBeInTheDocument();
    expect(await screen.findByRole("table", { name: "Audit events" })).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Audit events" })).toBeInTheDocument();
    expect(screen.queryByText("This view is not available yet.")).not.toBeInTheDocument();
  });

  it("requests the first audit page with no query string (AC-2)", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-b".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, { events: [auditEvent()], has_more: false, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("table", { name: "Audit events" })).toBeInTheDocument();
    const auditCalls = fetch.mock.calls.map((call) => String(call[0])).filter((url) => isAuditUrl(url));
    expect(auditCalls[0]).toBe("/api/v1/audit");
  });

  it("pages older with the verbatim next_cursor and disables Older without a usable cursor (AC-3)", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-c".padEnd(64, "0") });
      }
      if (url === "/api/v1/audit") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 10, actor: "page-one" })],
          has_more: true,
          next_cursor: "YTE6MTQyMQ",
          limit: 50,
        });
      }
      if (url === "/api/v1/audit?cursor=YTE6MTQyMQ") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 9, actor: "page-two" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("page-one")).not.toHaveLength(0);
    expect(screen.getByRole("button", { name: "Older" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findAllByText("page-two")).not.toHaveLength(0);
    expect(fetch.mock.calls.map((call) => String(call[0])).filter((url) => isAuditUrl(url))).toEqual([
      "/api/v1/audit",
      "/api/v1/audit?cursor=YTE6MTQyMQ",
    ]);
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();
  });

  it("disables Older when has_more is true without a next_cursor (AC-3)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-d".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent()],
          has_more: true,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Audit history is unavailable. Try again.");
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("disables Older when has_more is true and next_cursor is empty (AC-3)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-e".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent()],
          has_more: true,
          next_cursor: "",
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Audit history is unavailable. Try again.");
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();
  });

  it("replays consumed cursors in reverse when moving newer (AC-4)", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-f".padEnd(64, "0") });
      }
      if (url === "/api/v1/audit") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 30, actor: "newest" })],
          has_more: true,
          next_cursor: "cursor-one",
          limit: 50,
        });
      }
      if (url === "/api/v1/audit?cursor=cursor-one") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 20, actor: "middle" })],
          has_more: true,
          next_cursor: "cursor-two",
          limit: 50,
        });
      }
      if (url === "/api/v1/audit?cursor=cursor-two") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 10, actor: "oldest" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("newest")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findAllByText("middle")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findAllByText("oldest")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Newer" }));
    expect(await screen.findAllByText("middle")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Newer" }));
    expect(await screen.findAllByText("newest")).not.toHaveLength(0);
    expect(fetch.mock.calls.map((call) => String(call[0])).filter((url) => isAuditUrl(url))).toEqual([
      "/api/v1/audit",
      "/api/v1/audit?cursor=cursor-one",
      "/api/v1/audit?cursor=cursor-two",
      "/api/v1/audit?cursor=cursor-one",
      "/api/v1/audit",
    ]);
  });

  it("renders audit events in response array order (AC-5)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-g".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [
            auditEvent({ id: 9, actor: "first-shown" }),
            auditEvent({ id: 3, actor: "second-shown" }),
            auditEvent({ id: 7, actor: "third-shown" }),
          ],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    const table = await screen.findByRole("table", { name: "Audit events" });
    const tableText = table.textContent ?? "";
    expect(tableText.indexOf("first-shown")).toBeLessThan(tableText.indexOf("second-shown"));
    expect(tableText.indexOf("second-shown")).toBeLessThan(tableText.indexOf("third-shown"));
  });

  it("replaces bidi controls in every rendered audit field (AC-6)", async () => {
    const poisoned = "admin\u202Enimda";
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-h".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [
            auditEvent({
              actor: poisoned,
              action: `act\u202Eion`,
              target: `tgt\u202E`,
              outcome: `ok\u202E`,
              request_id: `aa\u202Ebb`,
              client_ip: `1.2.3.4\u202E`,
              created_at: `2026-08-25T04:11:09.123456789\u202EZ`,
            }),
          ],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("table", { name: "Audit events" })).toBeInTheDocument();
    const forbidden = ["\u200E", "\u200F", "\u061C", "\u202A", "\u202B", "\u202C", "\u202D", "\u202E", "\u2066", "\u2067", "\u2068", "\u2069"];
    const text = document.body.textContent ?? "";
    for (const point of forbidden) {
      expect(text).not.toContain(point);
    }
    const isolates = [...document.querySelectorAll(".bidi-isolate")];
    expect(isolates.some((node) => (node.textContent ?? "").includes("\uFFFD"))).toBe(true);
    expect(isolates.some((node) => (node.textContent ?? "").includes("admin\uFFFD") && node.classList.contains("bidi-isolate"))).toBe(
      true,
    );
    expect(isolates.length).toBeGreaterThan(0);
    for (const stamp of document.querySelectorAll("time")) {
      expect(stamp.getAttribute("dateTime") ?? "").not.toContain("\u202E");
      expect(stamp.getAttribute("dateTime") ?? "").toContain("\uFFFD");
    }
  });

  it("renders markup-looking actor values as text nodes (AC-6)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-i".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent({ actor: "<img src=x onerror=alert(1)>" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("<img src=x onerror=alert(1)>")).not.toHaveLength(0);
    expect(document.querySelector("img")).toBeNull();
  });

  it("renders the stored created_at string with a UTC marker (AC-7)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-j".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent({ created_at: "2026-08-25T04:11:09.123456789Z" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    const stamp = await screen.findAllByText(/2026-08-25T04:11:09\.123456789Z/);
    expect(stamp.length).toBeGreaterThan(0);
    const time = document.querySelector("time");
    expect(time).toHaveAttribute("dateTime", "2026-08-25T04:11:09.123456789Z");
    expect(time).toHaveTextContent("2026-08-25T04:11:09.123456789Z");
    expect(time?.textContent).toContain("UTC");
    expect(time).toHaveClass("identifier");
    expect(document.body.textContent ?? "").not.toMatch(/\b(?:AM|PM)\b/);
    expect(time?.textContent ?? "").not.toContain("/");
  });

  it("shows an accessible dash for empty actor, target, and source address (AC-8)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-k".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent({ actor: "", target: "", client_ip: "" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("Not recorded")).not.toHaveLength(0);
    expect(screen.getAllByText("—").length).toBeGreaterThanOrEqual(3);
    expect(screen.queryByText("Null")).not.toBeInTheDocument();
  });

  it("discloses source address without hover and does not label the column Client IP (AC-9)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-l".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, { events: [auditEvent()], has_more: false, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByText(/tunnel connector/)).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Source address" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Client IP" })).not.toBeInTheDocument();
  });

  it("shows a session-expired alert without an empty audit log (AC-10)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-m".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByText("No audit events.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
  });

  it("recovers from a bad cursor without echoing the submitted value (AC-10)", async () => {
    const submitted = "bad-cursor-echo-canary";
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-n".padEnd(64, "0") });
      }
      if (url === "/api/v1/audit") {
        return jsonResponse(200, {
          events: [auditEvent({ actor: "page-one" })],
          has_more: true,
          next_cursor: submitted,
          limit: 50,
        });
      }
      if (url.includes("cursor=")) {
        return jsonResponse(400, {
          error: { code: "validation_error", message: "Invalid cursor", fields: { cursor: "invalid" } },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("page-one")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This audit page could not be loaded. Return to the newest events.");
    expect(screen.getByRole("button", { name: "Newest" })).toBeEnabled();
    expect(document.body.textContent ?? "").not.toContain(submitted);
    expect(screen.queryByText("No audit events.")).not.toBeInTheDocument();
  });

  it("shows control-plane storage unavailability without an empty audit log (AC-10)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-o".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "Control-plane storage is unavailable" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Control-plane storage is unavailable");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryByText("No audit events.")).not.toBeInTheDocument();
  });

  it("shows a generic alert when the audit request throws (AC-10)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-p".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        throw new TypeError("Failed to fetch");
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Audit history is unavailable. Try again.");
    expect(screen.queryByText("No audit events.")).not.toBeInTheDocument();
    expect(screen.queryByText("Control-plane storage is unavailable")).not.toBeInTheDocument();
  });

  it("shows empty audit history only after a successful empty page (AC-11)", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-q".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, { events: [], has_more: false, limit: 50 });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findByText("No audit events.")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("does not persist the cursor and clears audit rows on logout (AC-12)", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-r".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (url === "/api/v1/audit") {
        return jsonResponse(200, {
          events: [auditEvent({ actor: "visible-audit-actor" })],
          has_more: true,
          next_cursor: "YTE6MTQyMQ",
          limit: 50,
        });
      }
      if (url === "/api/v1/audit?cursor=YTE6MTQyMQ") {
        return jsonResponse(200, {
          events: [auditEvent({ id: 9, actor: "older-audit-actor" })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    expect(await screen.findAllByText("visible-audit-actor")).not.toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findAllByText("older-audit-actor")).not.toHaveLength(0);
    expect(setItem).not.toHaveBeenCalled();
    expect(window.location.search).not.toMatch(/cursor/);
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByText("visible-audit-actor")).not.toBeInTheDocument();
    expect(screen.queryByText("older-audit-actor")).not.toBeInTheDocument();
    setItem.mockRestore();
  });

  it("keeps the audit table in a bounded identifier grid without a service rail (AC-14)", async () => {
    const actor = "a".repeat(64);
    const requestId = "aabbccddeeff00112233445566778899";
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "audit-s".padEnd(64, "0") });
      }
      if (isAuditUrl(url)) {
        return jsonResponse(200, {
          events: [auditEvent({ actor, request_id: requestId })],
          has_more: false,
          limit: 50,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Audit" }));
    const table = await screen.findByRole("table", { name: "Audit events" });
    expect(within(table).getByText(actor)).toHaveClass("identifier");
    expect(within(table).getByText(requestId)).toHaveClass("identifier");
    expect(document.querySelector(".audit-grid-wrap")).not.toBeNull();
    const page = document.querySelector(".audit-page");
    expect(page?.querySelector(".service-rail")).toBeNull();
    expect(page?.querySelector(".service-rail-postgres")).toBeNull();
    expect(page?.querySelector(".service-rail-redis")).toBeNull();
  });

  it("shows mixed Overview status without blanking Redis", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-a".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Redgres state" })).toBeInTheDocument();
    expect(screen.getByLabelText("Redgres state: Reachable")).toBeInTheDocument();
    expect(screen.getByLabelText("PostgreSQL direct: Unavailable")).toBeInTheDocument();
    expect(screen.getByLabelText("Redis: Reachable")).toBeInTheDocument();
    expect(screen.getAllByText("Reachable").length).toBeGreaterThan(1);
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    expect(screen.getAllByText("Not connected").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "Redis" })).toBeInTheDocument();
    expect(screen.queryByText("Adapters are not connected in this release slice.")).not.toBeInTheDocument();
    expect(screen.getByText("Independent component status.")).toBeInTheDocument();
  });

  it("shows a session-expired Overview alert without status cards", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-b".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("heading", { name: "Redgres state" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
  });

  it("shows a generic Overview alert when status fetch throws", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-c".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        throw new TypeError("Failed to fetch");
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Component status is unavailable. Try again.");
    expect(screen.queryByRole("heading", { name: "Redgres state" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Redis" })).not.toBeInTheDocument();
  });

  it("renders Redis as Unavailable when the status payload omits it", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-d".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "cccccccccccccccccccccccccccccccc",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redis: Unavailable")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Redis" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Redis: Reachable")).not.toBeInTheDocument();
  });

  it("shows Overview loading status then replaces it with cards", async () => {
    let release: () => void = () => {};
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-e".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        await new Promise<void>((resolve, reject) => {
          if (init?.signal?.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          const onAbort = () => {
            init?.signal?.removeEventListener("abort", onAbort);
            reject(new DOMException("The operation was aborted.", "AbortError"));
          };
          init?.signal?.addEventListener("abort", onAbort);
          void blocked.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return mixedStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("status")).toHaveTextContent("Loading component status.");
    expect(screen.queryByRole("heading", { name: "Redgres state" })).not.toBeInTheDocument();
    release();
    expect(await screen.findByLabelText("Redgres state: Reachable")).toBeInTheDocument();
    expect(screen.queryByText("Loading component status.")).not.toBeInTheDocument();
  });

  it("refetches /api/v1/status with no query when Refresh is used", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-f".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redgres state: Reachable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findByLabelText("Redgres state: Reachable")).toBeInTheDocument();
    const urls = fetch.mock.calls.map((call) => String(call[0]));
    const statusCalls = urls.filter((url) => isStatusUrl(url));
    const redisStatusCalls = urls.filter((url) => isRedisStatusUrl(url));
    expect(isStatusUrl("/api/v1/redis/status")).toBe(false);
    expect(statusCalls.length).toBeGreaterThanOrEqual(2);
    expect(statusCalls.every((url) => url === "/api/v1/status")).toBe(true);
    expect(redisStatusCalls.length).toBeGreaterThanOrEqual(2);
    expect(redisStatusCalls.every((url) => url === "/api/v1/redis/status")).toBe(true);
  });

  it("clears Overview cards on logout", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-g".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("PostgreSQL direct: Unavailable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByLabelText("PostgreSQL direct: Unavailable")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Redgres state" })).not.toBeInTheDocument();
  });

  it("treats an unknown Overview state as Unavailable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-h".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "mystery" },
            { id: "postgres_direct", state: "not_configured" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "not_implemented" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "dddddddddddddddddddddddddddddddd",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redgres state: Unavailable")).toBeInTheDocument();
    expect(screen.queryByLabelText("Redgres state: Reachable")).not.toBeInTheDocument();
  });

  it("does not paint PostgreSQL unavailable with Redis identity red", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-i".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    const card = await screen.findByLabelText("PostgreSQL direct: Unavailable");
    expect(card).toHaveClass("status-card-postgres");
    expect(card).not.toHaveClass("status-card-redis");
    expect(card.querySelector(".service-rail-redis")).toBeNull();
    expect(card.querySelector(".status-unavailable")).not.toBeNull();
    expect(card.querySelector(".status-unavailable")).not.toHaveClass("status-card-redis");
    const status = card.querySelector(".status-unavailable");
    expect(status).not.toHaveClass("service-rail-redis");
  });

  it("shows an Overview alert without cards for a malformed status payload", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-j".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return {
          ok: true,
          status: 200,
          headers: new Headers(),
          json: async () => {
            throw new SyntaxError("Unexpected end of JSON input");
          },
        };
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Component status is unavailable. Try again.");
    expect(screen.queryByRole("heading", { name: "Redgres state" })).not.toBeInTheDocument();
  });

  it("shows Redis Unavailable independently when PostgreSQL is reachable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-redis-unavail".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "unavailable", reason: "unreachable" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    const card = await screen.findByLabelText("Redis: Unavailable");
    expect(card).toHaveClass("status-card-redis");
    expect(screen.getByLabelText("PostgreSQL direct: Reachable")).toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("shows Redis as Not configured from not_configured", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-redis-noconf".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "not_configured" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "ffffffffffffffffffffffffffffffff",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redis: Not configured")).toBeInTheDocument();
    expect(screen.getByLabelText("PostgreSQL direct: Reachable")).toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("shows Redis as Reachable when redis is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-redis-ok".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "ok" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "11111111111111111111111111111111",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redis: Reachable")).toBeInTheDocument();
    expect(screen.getByLabelText("PostgreSQL direct: Reachable")).toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("shows Redis metrics when /status and /redis/status are both ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-metrics-ok".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return overviewOkStatus();
      }
      if (isRedisStatusUrl(url)) {
        return redisOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Reachable");
    expect(redis).toHaveClass("status-card-redis");
    expect(within(redis).getByText("Reachable")).toHaveClass("status-ok");
    expect(within(redis).getByText("Version")).toBeInTheDocument();
    expect(within(redis).getByText("8.2.1")).toHaveClass("bidi-isolate");
    expect(within(redis).getByText("8.2.1")).toHaveClass("identifier");
    expect(within(redis).getByText("Uptime")).toBeInTheDocument();
    expect(within(redis).getByText("2m 3s")).toHaveClass("metric");
    expect(within(redis).getByText("Clients")).toBeInTheDocument();
    expect(within(redis).getByText("4")).toHaveClass("metric");
    expect(within(redis).getByText("Used / max memory")).toBeInTheDocument();
    expect(within(redis).getByText("1.0 MiB / Unlimited")).toHaveClass("metric");
    expect(within(redis).getByText("Ops/s")).toBeInTheDocument();
    expect(within(redis).getByText("12")).toHaveClass("metric");
    expect(within(redis).getByText("DB size")).toBeInTheDocument();
    expect(within(redis).getByText("50")).toHaveClass("metric");
    expect(within(redis).getByText("Latency")).toBeInTheDocument();
    expect(within(redis).getByText("1.25 ms")).toHaveClass("metric");
    expect(screen.queryByText("Metrics unavailable")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
    const postgres = screen.getByLabelText("PostgreSQL direct: Reachable");
    expect(within(postgres).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(postgres).queryByText("Used / max memory")).not.toBeInTheDocument();
  });

  it("shows Authentication failed from /redis/status auth_failed", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-auth-fail".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "unavailable" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "44444444444444444444444444444444",
        });
      }
      if (isRedisStatusUrl(url)) {
        return redisUnavailableStatus("auth_failed");
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Unavailable");
    expect(within(redis).getByText("Authentication failed")).toBeInTheDocument();
    expect(within(redis).queryByText("Permission denied")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Unreachable")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("shows Permission denied from /redis/status permission_denied", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-perm-fail".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "unavailable" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "55555555555555555555555555555555",
        });
      }
      if (isRedisStatusUrl(url)) {
        return redisUnavailableStatus("permission_denied");
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Unavailable");
    expect(within(redis).getByText("Permission denied")).toBeInTheDocument();
    expect(within(redis).queryByText("Authentication failed")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Unreachable")).not.toBeInTheDocument();
  });

  it("shows Unreachable from /redis/status unreachable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-unreach".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "unavailable" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "66666666666666666666666666666666",
        });
      }
      if (isRedisStatusUrl(url)) {
        return redisUnavailableStatus("unreachable");
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Unavailable");
    expect(within(redis).getByText("Unreachable")).toBeInTheDocument();
    expect(within(redis).queryByText("Authentication failed")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Permission denied")).not.toBeInTheDocument();
  });

  it("keeps Reachable and shows Metrics unavailable without fake zeros", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-metrics-unavail".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return overviewOkStatus();
      }
      if (isRedisStatusUrl(url)) {
        return redisUnavailableStatus("auth_failed");
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Reachable");
    expect(within(redis).getByText("Reachable")).toHaveClass("status-ok");
    expect(within(redis).getByText("Metrics unavailable")).toBeInTheDocument();
    expect(within(redis).getByText("Authentication failed")).toBeInTheDocument();
    expect(within(redis).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Unlimited")).not.toBeInTheDocument();
    expect(within(redis).queryByText("0")).not.toBeInTheDocument();
    expect(within(redis).queryByText("8.2.1")).not.toBeInTheDocument();
  });

  it("omits Redis metric rows when /redis/status is not_configured", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-metrics-omit".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "not_implemented" },
            { id: "redis", state: "not_configured" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "77777777777777777777777777777777",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    const redis = await screen.findByLabelText("Redis: Not configured");
    expect(within(redis).queryByText("Version")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Uptime")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(redis).queryByText("Metrics unavailable")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("keeps PostgreSQL cards when /redis/status fails alone", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-fail-alone".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      if (isRedisStatusUrl(url)) {
        throw new TypeError("Failed to fetch");
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("PostgreSQL direct: Unavailable")).toBeInTheDocument();
    const redis = screen.getByLabelText("Redis: Reachable");
    expect(within(redis).getByText("Metrics unavailable")).toBeInTheDocument();
    expect(within(redis).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
  });

  it("does not render a canary secret from /redis/status", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-canary".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return overviewOkStatus();
      }
      if (isRedisStatusUrl(url)) {
        return redisOkStatus({ password: "canary-secret", url: "rediss://canary-secret@10.0.0.1/0" });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Redis: Reachable")).toBeInTheDocument();
    expect(screen.getByText("8.2.1")).toBeInTheDocument();
    expect(screen.queryByText("canary-secret")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-secret");
  });

  it("keeps postgres Unavailable and Redis Reachable independent with Redis-only metrics", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "redis-indep".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return mixedStatus();
      }
      if (isRedisStatusUrl(url)) {
        return redisOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    const postgres = await screen.findByLabelText("PostgreSQL direct: Unavailable");
    const redis = screen.getByLabelText("Redis: Reachable");
    expect(within(postgres).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(postgres).queryByText("8.2.1")).not.toBeInTheDocument();
    expect(within(redis).getByText("8.2.1")).toBeInTheDocument();
    expect(within(redis).getByText("1.0 MiB / Unlimited")).toBeInTheDocument();
    expect(redis).toHaveClass("status-card-redis");
    expect(postgres).not.toHaveClass("status-card-redis");
  });
});
