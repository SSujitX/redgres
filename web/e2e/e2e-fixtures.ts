/**
 * Shared Playwright fixtures that intercept API routes to provide an
 * authenticated shell without a real Go backend. Every test that imports
 * `test` and `expect` from this file gets a `shellPage` fixture with
 * session/status/audit mocked and the authenticated shell rendered.
 */
import { test as base, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock API response payloads
// ---------------------------------------------------------------------------

export const CSRF_CANARY = "c".repeat(64);

const SESSION_BODY = {
  owner: { username: "owner" },
  csrf_token: CSRF_CANARY,
  capabilities: [
    "platform.read",
    "audit.read",
    "postgres.read",
    "postgres.provision",
    "postgres.credentials",
    "postgres.destructive",
    "redis.read",
    "redis.provision",
    "redis.credentials",
    "redis.destructive",
  ],
  tool_links: {},
  request_id: "00000000000000000000000000000001",
};

const STATUS_BODY = {
  components: [
    { id: "redgres_state", state: "ok" },
    { id: "postgres_direct", state: "ok" },
    { id: "pgbouncer", state: "not_configured" },
    { id: "redis", state: "ok" },
    { id: "tool_links", state: "not_configured" },
  ],
  request_id: "00000000000000000000000000000002",
};

const REDIS_STATUS_BODY = {
  state: "ok",
  metrics: {
    version: "8.8.2",
    uptime_seconds: 3600,
    connected_clients: 3,
    used_memory_bytes: 2097152,
    max_memory_bytes: 0,
    ops_per_sec: 42,
    db_size: 128,
    latency_ms: 0.85,
  },
  request_id: "00000000000000000000000000000003",
};

const AUDIT_BODY = {
  events: [
    {
      id: 1,
      actor: "owner",
      action: "owner.login",
      target: "owner",
      outcome: "success",
      request_id: "00000000000000000000000000000004",
      client_ip: "127.0.0.1",
      created_at: "2026-08-25T12:00:00Z",
    },
  ],
  has_more: false,
  limit: 8,
  request_id: "00000000000000000000000000000005",
};

const SEARCH_BODY = {
  groups: [
    {
      id: "postgres_databases",
      label: "PostgreSQL databases",
      service: "postgres",
      status: "ok",
      truncated: false,
      hits: [],
    },
    {
      id: "redis_acl_users",
      label: "Redis ACL users",
      service: "redis",
      status: "ok",
      truncated: false,
      hits: [],
    },
  ],
  limit: 20,
  request_id: "00000000000000000000000000000006",
};

const POSTGRES_DATABASES_BODY = {
  databases: [],
  truncated: false,
  request_id: "00000000000000000000000000000007",
};

const POSTGRES_SECURITY_BODY = {
  summary: {
    database_count: 0,
    public_connect_count: 0,
    active_connection_count: 0,
    connection_group_count: 0,
    missing_password_count: 0,
  },
  databases: [],
  connections: [],
  saved_credential: { status: "ok", reason: "" },
  truncated: false,
  request_id: "00000000000000000000000000000008",
};

const REDIS_USERS_BODY = {
  state: "ok",
  users: [],
  truncated: false,
  request_id: "00000000000000000000000000000009",
};

const REDIS_PRESETS_BODY = {
  presets: [
    { preset: "cache-read-write", commands: ["get", "set", "del"] },
    { preset: "read-only", commands: ["get", "exists"] },
    { preset: "queue-worker", queue_kind: "lists", commands: ["lpush", "rpush"] },
    { preset: "queue-worker", queue_kind: "streams", commands: ["xadd", "xread"] },
    { preset: "queue-worker", queue_kind: "sorted-sets", commands: ["zadd", "zrange"] },
  ],
  request_id: "0000000000000000000000000000000a",
};

function jsonRoute(route: Route, method: string, body: unknown) {
  if (route.request().method() !== method) {
    return route.fulfill({
      status: 405,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "method_not_allowed", message: "Method not allowed" } }),
    });
  }
  return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
}

// ---------------------------------------------------------------------------
// Route interception helper
// ---------------------------------------------------------------------------

async function interceptShellAPIs(page: Page) {
  // Session — establishes authentication
  await page.route("**/api/v1/session", (route) =>
    jsonRoute(route, "GET", SESSION_BODY),
  );

  // Platform status
  await page.route("**/api/v1/status", (route) =>
    jsonRoute(route, "GET", STATUS_BODY),
  );

  // Redis status/metrics
  await page.route("**/api/v1/redis/status", (route) =>
    jsonRoute(route, "GET", REDIS_STATUS_BODY),
  );

  // Audit (compact for Overview + paginated)
  await page.route("**/api/v1/audit**", (route) => {
    const requested = new URL(route.request().url()).searchParams.get("limit");
    const limit = requested === null ? 50 : Number(requested);
    return jsonRoute(route, "GET", { ...AUDIT_BODY, limit });
  });

  // Search
  await page.route("**/api/v1/search**", (route) =>
    jsonRoute(route, "GET", SEARCH_BODY),
  );

  // PostgreSQL databases list
  await page.route("**/api/v1/postgres/databases", (route) => jsonRoute(route, "GET", POSTGRES_DATABASES_BODY));

  // PostgreSQL security
  await page.route("**/api/v1/postgres/security", (route) =>
    jsonRoute(route, "GET", POSTGRES_SECURITY_BODY),
  );

  // Redis users
  await page.route("**/api/v1/redis/users**", (route) =>
    jsonRoute(route, "GET", REDIS_USERS_BODY),
  );

  // Redis presets
  await page.route("**/api/v1/redis/presets", (route) =>
    jsonRoute(route, "GET", REDIS_PRESETS_BODY),
  );

  // Redis commands
  await page.route("**/api/v1/redis/commands", (route) =>
    jsonRoute(route, "GET", {
      commands: ["del", "get", "set"],
      request_id: "0000000000000000000000000000000b",
    }),
  );

  // Logout
  await page.route("**/api/v1/auth/logout", (route) =>
    jsonRoute(route, "POST", { ok: true, request_id: "0000000000000000000000000000000c" }),
  );
}

// ---------------------------------------------------------------------------
// Custom fixture: shellPage
// ---------------------------------------------------------------------------

type ShellFixtures = {
  /** A Page with all shell API routes intercepted and the authenticated shell rendered. */
  shellPage: Page;
};

export const test = base.extend<ShellFixtures>({
  shellPage: async ({ page }, use) => {
    await interceptShellAPIs(page);
    await page.goto("/");
    // Wait for the authenticated shell to appear (owner button proves session 200 worked)
    await expect(page.getByRole("button", { name: "owner" })).toBeVisible({ timeout: 15_000 });
    await use(page);
  },
});

export { expect };
export { interceptShellAPIs };
