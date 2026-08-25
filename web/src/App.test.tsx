import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import globalsCss from "./styles/globals.css?raw";

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
  return (
    url.includes(prefix) &&
    !url.includes("/tables") &&
    !url.includes("/connection") &&
    !url.includes("/credentials") &&
    !url.includes("/duplicate")
  );
}

function isConnectionUrl(url: string, name?: string): boolean {
  if (name !== undefined) {
    const path = `/api/v1/postgres/databases/${encodeURIComponent(name)}/connection`;
    return url.includes(path) && !url.includes("/reveal");
  }
  return url.includes("/api/v1/postgres/databases/") && url.includes("/connection") && !url.includes("/reveal");
}

function isConnectionRevealUrl(url: string, name?: string, init?: RequestInit): boolean {
  const isPost = String(init?.method ?? "").toUpperCase() === "POST";
  if (name !== undefined) {
    return url === `/api/v1/postgres/databases/${encodeURIComponent(name)}/connection/reveal` && isPost;
  }
  return url.includes("/connection/reveal") && isPost;
}

const maskedDirectUrl =
  "postgresql://project_a_role:********@db.example.com:5432/project_a?sslmode=require";
const maskedPooledUrl =
  "postgresql://project_a_role:********@db.example.com:6432/project_a?sslmode=require";

function postgresConnectionAbsent(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    database: "project_a",
    owner: "project_a_role",
    saved_credential: { status: "missing", reason: "" },
    request_id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    ...extra,
  });
}

function postgresConnectionPresent(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    database: "project_a",
    owner: "project_a_role",
    saved_credential: { status: "present", reason: "" },
    masked_direct_url: maskedDirectUrl,
    masked_pooled_url: maskedPooledUrl,
    request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    ...extra,
  });
}

const revealedDirectUrl =
  "postgresql://project_a_role:canary-pg-reveal-password-32chars!!@db.example.com:5432/project_a?sslmode=require";
const revealedPooledUrl =
  "postgresql://project_a_role:canary-pg-reveal-password-32chars!!@db.example.com:6432/project_a?sslmode=require";

function postgresReveal200(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    resource: { type: "postgres_database", name: "project_a" },
    credential: {
      username: "project_a_role",
      password: "canary-pg-reveal-password-32chars!!",
      one_time: false,
      urls: {
        direct: revealedDirectUrl,
        pooled: revealedPooledUrl,
      },
    },
    request_id: "dddddddddddddddddddddddddddddddd",
    ...extra,
  });
}

const createdDirectUrl =
  "postgresql://app_project_a:canary-pg-create-password-32chars!!@db.example.com:5432/project_a?sslmode=require";
const createdPooledUrl =
  "postgresql://app_project_a:canary-pg-create-password-32chars!!@db.example.com:6432/project_a?sslmode=require";

function isPostgresDatabasesCreate(url: string, init?: RequestInit): boolean {
  return (
    (url === "/api/v1/postgres/databases" || url.endsWith("/api/v1/postgres/databases")) &&
    String(init?.method ?? "").toUpperCase() === "POST"
  );
}

function isPostgresCredentialsRotate(url: string, name?: string, init?: RequestInit): boolean {
  const isPost = String(init?.method ?? "").toUpperCase() === "POST";
  if (name !== undefined) {
    return url === `/api/v1/postgres/databases/${encodeURIComponent(name)}/credentials/rotate` && isPost;
  }
  return url.includes("/api/v1/postgres/databases/") && url.includes("/credentials/rotate") && isPost;
}

function isPostgresDatabaseDuplicate(url: string, name?: string, init?: RequestInit): boolean {
  const isPost = String(init?.method ?? "").toUpperCase() === "POST";
  if (name !== undefined) {
    return url === `/api/v1/postgres/databases/${encodeURIComponent(name)}/duplicate` && isPost;
  }
  return url.includes("/api/v1/postgres/databases/") && url.includes("/duplicate") && isPost;
}

const rotatedDirectUrl =
  "postgresql://project_a_role:canary-pg-rotate-password-32chars!!@db.example.com:5432/project_a?sslmode=require";
const rotatedPooledUrl =
  "postgresql://project_a_role:canary-pg-rotate-password-32chars!!@db.example.com:6432/project_a?sslmode=require";
const vaultOutOfSyncCopy =
  "The PostgreSQL password was changed but the vault could not be saved. Rotate again.";

function postgresRotateEligibleDatabase(extra: Record<string, unknown> = {}) {
  return {
    name: "project_a",
    owner: "project_a_role",
    security: {
      public_can_connect: false,
      owner_is_superuser: false,
      owner_can_login: true,
      owner_createdb: false,
      owner_createrole: false,
      owner_replication: false,
    },
    saved_credential: { status: "present", reason: "" },
    ...extra,
  };
}

function postgresRotate200(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    resource: { type: "postgres_database", name: "project_a" },
    credential: {
      username: "project_a_role",
      password: "canary-pg-rotate-password-32chars!!",
      one_time: false,
      urls: {
        direct: rotatedDirectUrl,
        pooled: rotatedPooledUrl,
      },
    },
    request_id: "ffffffffffffaaaaaaaaaaaaaaaaaaaa",
    ...extra,
  });
}

function postgresRotateInspectorFetch(
  csrf: string,
  extras: {
    details?: Record<string, unknown>;
    connection?: ReturnType<typeof postgresConnectionPresent>;
    rotate?: (
      init?: RequestInit,
    ) => ReturnType<typeof jsonResponse> | Promise<ReturnType<typeof jsonResponse>>;
  } = {},
) {
  return (url: string, init?: RequestInit) => {
    if (url.includes("/api/v1/session")) {
      return jsonResponse(200, { owner: { username: "admin" }, csrf_token: csrf });
    }
    if (url.endsWith("/api/v1/postgres/databases")) {
      return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
    }
    if (isTablesUrl(url, "project_a")) {
      return jsonResponse(200, { tables: [], truncated: false });
    }
    if (isConnectionUrl(url, "project_a")) {
      return extras.connection ?? postgresConnectionPresent();
    }
    if (isDetailsUrl(url, "project_a")) {
      return jsonResponse(200, { database: extras.details ?? postgresRotateEligibleDatabase() });
    }
    if (isPostgresCredentialsRotate(url, "project_a", init) && extras.rotate) {
      return extras.rotate(init);
    }
    return unknownApi(url);
  };
}

const duplicatedDirectUrl =
  "postgresql://app_project_a_copy:canary-pg-duplicate-password-32chars!!@db.example.com:5432/project_a_copy?sslmode=require";
const duplicatedPooledUrl =
  "postgresql://app_project_a_copy:canary-pg-duplicate-password-32chars!!@db.example.com:6432/project_a_copy?sslmode=require";
const isolationRollbackCopy =
  "The source database ownership or CONNECT ACL changed during duplicate. The clone was rolled back.";

function postgresDuplicateEligibleDatabase(extra: Record<string, unknown> = {}) {
  return postgresRotateEligibleDatabase({ connection_count: 0, ...extra });
}

function postgresDuplicate201(extra: Record<string, unknown> = {}) {
  return jsonResponse(201, {
    resource: { type: "postgres_database", name: "project_a_copy" },
    credential: {
      username: "app_project_a_copy",
      password: "canary-pg-duplicate-password-32chars!!",
      one_time: false,
      urls: {
        direct: duplicatedDirectUrl,
        pooled: duplicatedPooledUrl,
      },
    },
    request_id: "11111111222222223333333344444444",
    ...extra,
  });
}

function postgresDuplicateInspectorFetch(
  csrf: string,
  extras: {
    details?: Record<string, unknown>;
    connection?: ReturnType<typeof postgresConnectionPresent>;
    duplicate?: (
      init?: RequestInit,
    ) => ReturnType<typeof jsonResponse> | Promise<ReturnType<typeof jsonResponse>>;
    listAfterDuplicate?: () => ReturnType<typeof jsonResponse>;
  } = {},
) {
  let duplicated = false;
  return (url: string, init?: RequestInit) => {
    if (url.includes("/api/v1/session")) {
      return jsonResponse(200, { owner: { username: "admin" }, csrf_token: csrf });
    }
    if (isPostgresDatabaseDuplicate(url, "project_a", init) && extras.duplicate) {
      duplicated = true;
      return extras.duplicate(init);
    }
    if (url.endsWith("/api/v1/postgres/databases")) {
      if (duplicated && extras.listAfterDuplicate) {
        return extras.listAfterDuplicate();
      }
      return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
    }
    if (isTablesUrl(url, "project_a") || isTablesUrl(url, "project_a_copy")) {
      return jsonResponse(200, { tables: [], truncated: false });
    }
    if (isConnectionUrl(url, "project_a") || isConnectionUrl(url, "project_a_copy")) {
      return extras.connection ?? postgresConnectionPresent();
    }
    if (isDetailsUrl(url, "project_a_copy")) {
      return jsonResponse(200, {
        database: postgresDuplicateEligibleDatabase({ name: "project_a_copy", owner: "app_project_a_copy" }),
      });
    }
    if (isDetailsUrl(url, "project_a")) {
      return jsonResponse(200, { database: extras.details ?? postgresDuplicateEligibleDatabase() });
    }
    return unknownApi(url);
  };
}

function postgresCreate201(extra: Record<string, unknown> = {}) {
  return jsonResponse(201, {
    resource: { type: "postgres_database", name: "project_a" },
    credential: {
      username: "app_project_a",
      password: "canary-pg-create-password-32chars!!",
      one_time: false,
      urls: {
        direct: createdDirectUrl,
        pooled: createdPooledUrl,
      },
    },
    request_id: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
    ...extra,
  });
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

function isRedisUsersListUrl(url: string): boolean {
  return url === "/api/v1/redis/users";
}

function isRedisUsersCreate(url: string, init?: RequestInit): boolean {
  return isRedisUsersListUrl(url) && String(init?.method ?? "").toUpperCase() === "POST";
}

function isRedisUserDetailUrl(url: string, username: string): boolean {
  return url === `/api/v1/redis/users/${encodeURIComponent(username)}`;
}

function isRedisUserEnable(url: string, username: string, init?: RequestInit): boolean {
  return (
    url === `/api/v1/redis/users/${encodeURIComponent(username)}/enable` &&
    String(init?.method ?? "").toUpperCase() === "POST"
  );
}

function isRedisUserDisable(url: string, username: string, init?: RequestInit): boolean {
  return (
    url === `/api/v1/redis/users/${encodeURIComponent(username)}/disable` &&
    String(init?.method ?? "").toUpperCase() === "POST"
  );
}

function isRedisUserRotate(url: string, username: string, init?: RequestInit): boolean {
  return (
    url === `/api/v1/redis/users/${encodeURIComponent(username)}/credentials/rotate` &&
    String(init?.method ?? "").toUpperCase() === "POST"
  );
}

function isRedisUserPatch(url: string, username: string, init?: RequestInit): boolean {
  return (
    url === `/api/v1/redis/users/${encodeURIComponent(username)}` &&
    String(init?.method ?? "").toUpperCase() === "PATCH"
  );
}

function isRedisUserDelete(url: string, username: string, init?: RequestInit): boolean {
  return (
    url === `/api/v1/redis/users/${encodeURIComponent(username)}` &&
    String(init?.method ?? "").toUpperCase() === "DELETE"
  );
}

function isRedisPresetsUrl(url: string): boolean {
  return url === "/api/v1/redis/presets" || url.startsWith("/api/v1/redis/presets?");
}

function isRedisCommandsUrl(url: string): boolean {
  return url === "/api/v1/redis/commands" || url.startsWith("/api/v1/redis/commands?");
}

function isPostgresSecurityUrl(url: string): boolean {
  return url === "/api/v1/postgres/security";
}

function postgresSecurityDatabase(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    name: "postgres",
    owner: "postgres",
    protected: true,
    public_can_connect: false,
    owner_is_superuser: true,
    owner_can_login: true,
    owner_createdb: true,
    owner_createrole: true,
    owner_replication: true,
    active_connections: 1,
    rotation_eligible: false,
    ...extra,
  };
}

function postgresSecurityConnection(extra: Record<string, unknown> = {}) {
  return {
    database: "postgres",
    user: "postgres",
    client: "local",
    application: "redgres",
    state: "idle",
    count: 1,
    ...extra,
  };
}

function postgresSecurityOk(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    summary: {
      database_count: 2,
      public_connect_count: 1,
      active_connection_count: 3,
      connection_group_count: 2,
      missing_password_count: 1,
    },
    saved_credential: { status: "ok", reason: "" },
    databases: [
      postgresSecurityDatabase(),
      postgresSecurityDatabase({
        name: "project_a",
        owner: "project_a_role",
        protected: false,
        public_can_connect: true,
        owner_is_superuser: false,
        owner_can_login: true,
        owner_createdb: false,
        owner_createrole: false,
        owner_replication: false,
        active_connections: 2,
        rotation_eligible: true,
      }),
    ],
    connections: [
      postgresSecurityConnection(),
      postgresSecurityConnection({
        database: "project_a",
        user: "project_a_role",
        client: "10.0.0.2",
        application: "app",
        state: "active",
        count: 2,
      }),
    ],
    truncated: false,
    request_id: "ffffffffffffffffffffffffffffffff",
    ...extra,
  });
}

function redisAclCommandsOk(commands: string[] = ["echo", "get", "ping", "set"]) {
  return jsonResponse(200, {
    commands,
    request_id: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
  });
}

function redisAclPatch200(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    user: {
      username: "project_a",
      enabled: true,
      key_pattern: "other:*",
      preset: "read-only",
      protected: false,
      rule_fidelity: "exact",
      commands: ["echo", "get", "ping"],
      categories: [],
      ...extra,
    },
    request_id: "dddddddddddddddddddddddddddddddd",
  });
}

function redisAclToggleOk(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    user: {
      username: "project_a",
      enabled: true,
      key_pattern: "project_a:*",
      preset: "cache-read-write",
      protected: false,
      rule_fidelity: "exact",
      commands: ["echo", "get", "ping"],
      categories: [],
      ...extra,
    },
    request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  });
}

function redisAclRotate200(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    resource: { type: "redis_user", name: "project_a" },
    user: {
      username: "project_a",
      enabled: true,
      key_pattern: "project_a:*",
      preset: "cache-read-write",
      protected: false,
      rule_fidelity: "exact",
      commands: ["echo", "get", "ping"],
      categories: [],
    },
    credential: {
      username: "project_a",
      password: "canary-rotated-password-32chars!!",
      one_time: true,
    },
    request_id: "cccccccccccccccccccccccccccccccc",
    ...extra,
  });
}

function redisAclCreate201(extra: Record<string, unknown> = {}) {
  return jsonResponse(201, {
    resource: { type: "redis_user", name: "project_a" },
    user: {
      username: "project_a",
      enabled: true,
      key_pattern: "project_a:*",
      preset: "cache-read-write",
      protected: false,
      rule_fidelity: "exact",
    },
    credential: {
      username: "project_a",
      password: "canary-one-time-password-32chars!!",
      one_time: true,
    },
    request_id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    ...extra,
  });
}

function redisAclListItem(extra: Record<string, unknown> = {}) {
  return {
    username: "project_a",
    enabled: true,
    key_pattern: "project_a:*",
    preset: "cache-read-write",
    protected: false,
    rule_fidelity: "exact",
    ...extra,
  };
}

function redisAclListOk(users: unknown[], extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    state: "ok",
    users,
    truncated: false,
    request_id: "44444444444444444444444444444444",
    ...extra,
  });
}

function redisAclDetailOk(extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    state: "ok",
    user: {
      username: "project_a",
      enabled: true,
      key_pattern: "project_a:*",
      preset: "cache-read-write",
      protected: false,
      rule_fidelity: "exact",
      commands: ["get", "set"],
      categories: [],
      ...extra,
    },
    request_id: "55555555555555555555555555555555",
  });
}

function goToAclUsers() {
  fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
  fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "ACL users" }));
}

function fillDeleteDialog(dialog: HTMLElement, username = "project_a", password = "owner-secret-15") {
  fireEvent.change(within(dialog).getByLabelText("Confirm username"), { target: { value: username } });
  fireEvent.change(within(dialog).getByLabelText("Owner password"), { target: { value: password } });
}

function goToSecurityOverview() {
  fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
  fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
  fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
  fireEvent.click(
    within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Security overview" }),
  );
}

async function goToDatabases() {
  fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
  fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
}

function databasesHeader() {
  return screen.getByRole("heading", { name: "Databases" }).closest("header") as HTMLElement;
}

async function openCreateDatabaseDialog() {
  await goToDatabases();
  const header = await screen.findByRole("heading", { name: "Databases" });
  const headerEl = header.closest("header") as HTMLElement;
  fireEvent.click(await within(headerEl).findByRole("button", { name: "Create database" }));
  return screen.findByRole("dialog", { name: "Create database" });
}

async function openDuplicateDatabaseDialog() {
  await goToDatabases();
  fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
  const details = await screen.findByRole("region", { name: "Database details" });
  fireEvent.click(await within(details).findByRole("button", { name: "Duplicate" }));
  return screen.findByRole("dialog", { name: "Duplicate database" });
}

function fillDuplicateForm(dialog: HTMLElement, database = "project_a_copy") {
  fireEvent.change(within(dialog).getByLabelText("New database name"), { target: { value: database } });
}

function factValue(label: string) {
  return screen.getByText(label).closest("div")?.querySelector("dd");
}

function expectNoVaultReasonCopy() {
  expect(screen.queryByText("vault_not_implemented")).not.toBeInTheDocument();
  expect(screen.queryByText("vault_unavailable")).not.toBeInTheDocument();
}

function isSearchUrl(url: string): boolean {
  return url === "/api/v1/search" || url.startsWith("/api/v1/search?");
}

function disconnectedStatus() {
  return jsonResponse(200, {
    components: [
      { id: "redgres_state", state: "not_configured" },
      { id: "postgres_direct", state: "not_configured" },
      { id: "pgbouncer", state: "not_configured" },
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
      { id: "pgbouncer", state: "not_configured" },
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
      { id: "pgbouncer", state: "not_configured" },
      { id: "redis", state: "ok" },
      { id: "tool_links", state: "not_configured" },
    ],
    request_id: "11111111111111111111111111111111",
  });
}

function toolLinksOkStatus() {
  return jsonResponse(200, {
    components: [
      { id: "redgres_state", state: "ok" },
      { id: "postgres_direct", state: "not_configured" },
      { id: "pgbouncer", state: "not_configured" },
      { id: "redis", state: "not_configured" },
      { id: "tool_links", state: "ok" },
    ],
    request_id: "toollinksok0000000000000000000000",
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
        status: "not_configured",
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
        status: "not_configured",
        truncated: false,
        hits: [],
      },
    ],
    limit: 20,
    request_id: "dddddddddddddddddddddddddddddddd",
  });
}

function redisHitSearch(extra: Record<string, unknown> = {}) {
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
        status: "ok",
        truncated: false,
        hits: [
          {
            id: "redis_acl_user:project_a",
            type: "redis_acl_user",
            label: "project_a",
            ...extra,
          },
        ],
      },
    ],
    limit: 20,
    request_id: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
  });
}

function unknownApi(url: string) {
  if (isConnectionUrl(url)) {
    return postgresConnectionAbsent();
  }
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
    expect(fetch.mock.calls.every((call) => !isConnectionUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/connection/reveal"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
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
    let authed = false;
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "b".repeat(64) });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "login-b".padEnd(64, "0") });
      }
      return unknownApi(url);
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "audit" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    expect(dialog.querySelector(".nav-result")).toHaveTextContent("Audit");
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => String(call[0]) === "/api/v1/search?q=audit")).toBe(true);
    });
    const searchCall = fetch.mock.calls.find((call) => String(call[0]) === "/api/v1/search?q=audit");
    const method = searchCall?.[1]?.method;
    expect(method === undefined || method === "GET").toBe(true);
    expect(fetch.mock.calls.every((call) => !isAuditUrl(String(call[0])))).toBe(true);
    const redisGroup = within(dialog).getByRole("region", { name: "Redis ACL users" });
    expect(await within(redisGroup).findByText("Not configured")).toBeInTheDocument();
    expect(within(dialog).queryByText(/Redis ACL user search is not available yet/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/No matching Redis ACL users/i)).not.toBeInTheDocument();
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "audit" } });
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
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables") && !url.includes("/connection")) {
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
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
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/connection/reveal"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
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
    const input = screen.getByLabelText("Search pages, databases, and ACL users");
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
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables") && !url.includes("/connection")) {
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: /project_b/ }));
    expect(await screen.findByRole("heading", { name: "project_b" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "drop" } });
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => String(call[0]) === "/api/v1/search?q=drop")).toBe(true);
    });
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "truncate" } });
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
    const input = screen.getByLabelText("Search pages, databases, and ACL users");
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "ab" } });
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
    const input = screen.getByLabelText("Search pages, databases, and ACL users");
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
    const input = screen.getByLabelText("Search pages, databases, and ACL users");
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
              status: "not_configured",
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "audit" } });
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
      if (url.includes("/api/v1/postgres/databases") && !url.includes("/tables") && !url.includes("/connection")) {
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("heading", { name: "project_a" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Search" })).not.toBeInTheDocument();
    expect(screen.queryByText("project_a")).not.toBeInTheDocument();
  });

  it("opens a redis ACL search hit and inspects that username", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-redis".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return redisHitSearch({
          password: "canary-secret",
          hash: "canary-secret",
          acl_rule: "+@all canary-secret",
          canary: "canary-secret",
        });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["get", "set"], categories: [] });
      }
      return unknownApi(url);
    });
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const dialog = await screen.findByRole("dialog", { name: "Search" });
    expect(within(dialog).getByRole("region", { name: "Redis ACL users" })).toBeInTheDocument();
    const hit = await screen.findByRole("button", { name: /project_a/ });
    expect(hit.className).toContain("nav-result-redis");
    expect(within(dialog).getByRole("status")).toHaveTextContent("1 matching ACL user.");
    expect(within(dialog).queryByText(/Redis ACL user search is not available yet/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText("canary-secret")).not.toBeInTheDocument();
    expect(dialog.textContent).not.toContain("canary-secret");
    fireEvent.click(hit);
    expect(await screen.findByRole("heading", { name: "ACL users" })).toBeInTheDocument();
    expect(await screen.findByText("get")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create ACL user" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    expect(screen.queryByText("canary-secret")).not.toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    const urls = fetch.mock.calls.map((call) => String(call[0]));
    const listIndex = urls.findIndex((url) => url === "/api/v1/redis/users");
    const detailIndex = urls.findIndex((url) => url === "/api/v1/redis/users/project_a");
    expect(listIndex).toBeGreaterThanOrEqual(0);
    expect(detailIndex).toBeGreaterThan(listIndex);
    setItem.mockRestore();
  });

  it("shows Unavailable for a redis search group and never unimplemented copy", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-redis-unavail".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return jsonResponse(200, {
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
              status: "unavailable",
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
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "audit" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    expect(await within(dialog).findByText("Unavailable")).toBeInTheDocument();
    expect(within(dialog).queryByText(/Redis ACL user search is not available yet/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/No matching Redis ACL users/i)).not.toBeInTheDocument();
  });

  it("still shows redis search hits when postgres search is unavailable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-pg-unavail-redis".padEnd(64, "0") });
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
              status: "ok",
              truncated: false,
              hits: [{ id: "redis_acl_user:project_a", type: "redis_acl_user", label: "project_a" }],
            },
          ],
          limit: 20,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const dialog = await screen.findByRole("dialog", { name: "Search" });
    expect(await within(dialog).findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(within(dialog).getByText("Unavailable")).toBeInTheDocument();
    expect(within(dialog).getByRole("status")).toHaveTextContent("1 matching ACL user.");
    expect(within(dialog).queryByText(/Redis ACL user search is not available yet/)).not.toBeInTheDocument();
  });

  it("moves focus through a redis search result with arrows and Enter", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-redis-keys".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return redisHitSearch();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    const input = screen.getByLabelText("Search pages, databases, and ACL users");
    fireEvent.change(input, { target: { value: "project" } });
    const dialog = screen.getByRole("dialog", { name: "Search" });
    const hit = await within(dialog).findByRole("button", { name: /project_a/ });
    input.focus();
    fireEvent.keyDown(dialog, { key: "ArrowDown" });
    expect(hit).toHaveFocus();
    fireEvent.keyDown(hit, { key: "Enter" });
    fireEvent.click(hit);
    expect(await screen.findByRole("heading", { name: "ACL users" })).toBeInTheDocument();
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
  });

  it("clears the redis inspector after arriving from search and logging out", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-redis-out".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isSearchUrl(url)) {
        return redisHitSearch();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["visible-acl-command"], categories: [] });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("visible-acl-command")).toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByText("visible-acl-command")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "ACL users" })).not.toBeInTheDocument();
    setItem.mockRestore();
  });

  it("reselects the same redis search hit after Back to users", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "search-redis-re".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return redisHitSearch();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["visible-acl-command"], categories: [] });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("visible-acl-command")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to users" }));
    expect(screen.queryByText("visible-acl-command")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const again = await screen.findByRole("dialog", { name: "Search" });
    fireEvent.click(await within(again).findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("visible-acl-command")).toBeInTheDocument();
    const detailCalls = fetch.mock.calls.filter((call) => String(call[0]) === "/api/v1/redis/users/project_a");
    expect(detailCalls.length).toBeGreaterThanOrEqual(2);
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
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionAbsent();
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
            saved_credential: { status: "present", reason: "" },
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
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Saved");
    expect(screen.queryByText("present")).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
    expect(screen.getByText("Owner is superuser").closest("div")).toHaveTextContent("Yes");
    expect(screen.getByText("Owner can create roles").closest("div")).toHaveTextContent("No");
    expect(await screen.findByText("items")).toBeInTheDocument();
    expect(screen.getByText("public")).toHaveClass("identifier");
    expect(screen.getByRole("button", { name: /Schema public Table items/ })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Database details" })).toHaveAttribute("aria-busy", "false");
    expect(screen.queryByText(/healthy/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/reachable/i)).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/rows"))).toBe(true);
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).queryByRole("button", { name: "Reveal" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
  });

  it("shows details saved-credential Not saved without reason codes", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "i".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionAbsent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            saved_credential: { status: "missing", reason: "" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not saved");
    expect(screen.queryByText("missing")).not.toBeInTheDocument();
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows details saved-credential Not available without leaking vault_unavailable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "i".repeat(64) });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionAbsent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            saved_credential: { status: "not_available", reason: "vault_unavailable" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Databases" }));
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not available");
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("does not treat /connection as a details URL", () => {
    expect(isDetailsUrl("/api/v1/postgres/databases/project_a", "project_a")).toBe(true);
    expect(isDetailsUrl("/api/v1/postgres/databases/project_a/tables", "project_a")).toBe(false);
    expect(isDetailsUrl("/api/v1/postgres/databases/project_a/connection", "project_a")).toBe(false);
    expect(isDetailsUrl("/api/v1/postgres/databases/project_a/credentials/rotate", "project_a")).toBe(false);
    expect(isDetailsUrl("/api/v1/postgres/databases/project_a/duplicate", "project_a")).toBe(false);
    expect(isConnectionUrl("/api/v1/postgres/databases/project_a/connection", "project_a")).toBe(true);
    expect(isConnectionUrl("/api/v1/postgres/databases/project_a/connection/reveal", "project_a")).toBe(false);
    expect(isConnectionRevealUrl("/api/v1/postgres/databases/project_a/connection/reveal", "project_a", { method: "POST" })).toBe(
      true,
    );
    expect(
      isPostgresCredentialsRotate("/api/v1/postgres/databases/project_a/credentials/rotate", "project_a", {
        method: "POST",
      }),
    ).toBe(true);
    expect(
      isPostgresDatabaseDuplicate("/api/v1/postgres/databases/project_a/duplicate", "project_a", {
        method: "POST",
      }),
    ).toBe(true);
  });

  it("shows copy-safe Direct URL and Pooled URL from the connection GET", async () => {
    const writeText = vi.fn();
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-a".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            saved_credential: { status: "present", reason: "" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(await within(details).findByText("Direct URL")).toBeInTheDocument();
    expect(within(details).getByText("Pooled URL")).toBeInTheDocument();
    expect(factValue("Direct URL")).toHaveTextContent(maskedDirectUrl);
    expect(factValue("Pooled URL")).toHaveTextContent(maskedPooledUrl);
    expect(factValue("Direct URL")).toHaveClass("bidi-isolate", "identifier");
    expect(factValue("Pooled URL")).toHaveClass("bidi-isolate", "identifier");
    expect(within(details).getByRole("button", { name: "Copy Direct URL" })).toHaveClass("text-button");
    expect(within(details).getByRole("button", { name: "Copy Pooled URL" })).toHaveClass("text-button");
    expect(writeText).not.toHaveBeenCalled();
    fireEvent.click(within(details).getByRole("button", { name: "Copy Direct URL" }));
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(maskedDirectUrl);
    fireEvent.click(within(details).getByRole("button", { name: "Copy Pooled URL" }));
    expect(writeText).toHaveBeenCalledWith(maskedPooledUrl);
    expect(screen.queryByText("YOUR_PASSWORD")).not.toBeInTheDocument();
    const reveal = within(details).getByRole("button", { name: "Reveal" });
    expect(reveal).toHaveClass("text-button");
    expect(reveal).not.toHaveClass("danger-button");
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Saved");
    expect(screen.queryByText("present")).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
    const connectionCalls = fetch.mock.calls.filter((call) => isConnectionUrl(String(call[0]), "project_a"));
    expect(connectionCalls.length).toBeGreaterThan(0);
    expect(connectionCalls.every((call) => String(call[0]) === "/api/v1/postgres/databases/project_a/connection")).toBe(
      true,
    );
    const method = connectionCalls[0]?.[1]?.method;
    expect(method === undefined || method === "GET").toBe(true);
    expect(new Headers(connectionCalls[0]?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("omits URL rows and reason strings when connection keys are absent", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-b".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionAbsent({
          saved_credential: { status: "missing", reason: "" },
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            saved_credential: { status: "missing", reason: "" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not saved");
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).queryByText("Direct URL")).not.toBeInTheDocument();
    expect(within(details).queryByText("Pooled URL")).not.toBeInTheDocument();
    expect(screen.queryByText("not configured")).not.toBeInTheDocument();
    expect(screen.queryByText("YOUR_PASSWORD")).not.toBeInTheDocument();
    expect(screen.queryByText("********")).not.toBeInTheDocument();
    expect(screen.queryByText("missing")).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("omits URL rows when connection vault status is not available", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-c".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionAbsent({
          saved_credential: { status: "not_available", reason: "vault_unavailable" },
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: {
            name: "project_a",
            owner: "project_a_role",
            saved_credential: { status: "not_available", reason: "vault_unavailable" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not available");
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).queryByText("Direct URL")).not.toBeInTheDocument();
    expect(within(details).queryByText("Pooled URL")).not.toBeInTheDocument();
    expect(screen.queryByText("not_available")).not.toBeInTheDocument();
    expect(screen.queryByText("********")).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows session-expired copy on connection 401 and paints no URLs", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-d".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return jsonResponse(401, {
          error: { code: "unauthorized", message: "Authentication required" },
          masked_direct_url: maskedDirectUrl,
          masked_pooled_url: maskedPooledUrl,
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your session has expired. Sign in again to continue.",
    );
    expect(screen.queryByText("Direct URL")).not.toBeInTheDocument();
    expect(screen.queryByText("Pooled URL")).not.toBeInTheDocument();
    expect(screen.queryByText(maskedDirectUrl)).not.toBeInTheDocument();
    expect(screen.queryByText(maskedPooledUrl)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
  });

  it("shows PostgreSQL is unavailable for a connection 503", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-e".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
        });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByText("Direct URL")).not.toBeInTheDocument();
    expect(screen.queryByText("Pooled URL")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
  });

  it("clears previous masked URLs when the selected database changes", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-f".padEnd(64, "0") });
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
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isConnectionUrl(url, "project_b")) {
        return postgresConnectionAbsent({
          database: "project_b",
          owner: "owner_b",
          saved_credential: { status: "missing", reason: "" },
        });
      }
      if (isDetailsUrl(url, "project_a") || isDetailsUrl(url, "project_b")) {
        const name = url.includes("project_b") ? "project_b" : "project_a";
        return jsonResponse(200, { database: { name, owner: name === "project_b" ? "owner_b" : "owner_a" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Direct URL")).toBeInTheDocument();
    expect(screen.getByText(maskedDirectUrl)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(await screen.findByText("owner_b")).toBeInTheDocument();
    expect(screen.queryByText("Direct URL")).not.toBeInTheDocument();
    expect(screen.queryByText("Pooled URL")).not.toBeInTheDocument();
    expect(screen.queryByText(maskedDirectUrl)).not.toBeInTheDocument();
    expect(screen.queryByText(maskedPooledUrl)).not.toBeInTheDocument();
  });

  it("clears masked connection URLs on logout", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-g".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText(maskedDirectUrl)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Database details" })).not.toBeInTheDocument();
    expect(screen.queryByText(maskedDirectUrl)).not.toBeInTheDocument();
    expect(screen.queryByText(maskedPooledUrl)).not.toBeInTheDocument();
    expect(screen.queryByText("Direct URL")).not.toBeInTheDocument();
  });

  it("does not fetch PostgreSQL connection on the login route", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isConnectionUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/connection/reveal"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
  });

  it("hides Reveal while details are loading", async () => {
    let releaseDetails: () => void = () => {};
    const blockedDetails = new Promise<void>((resolve) => {
      releaseDetails = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "reveal-load-d".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
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
          void blockedDetails.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Loading details.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reveal" })).not.toBeInTheDocument();
    releaseDetails();
    expect(await screen.findByRole("button", { name: "Reveal" })).toBeInTheDocument();
  });

  it("hides Reveal while connection is loading", async () => {
    let releaseConnection: () => void = () => {};
    const blockedConnection = new Promise<void>((resolve) => {
      releaseConnection = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "reveal-load-c".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionUrl(url, "project_a")) {
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
          void blockedConnection.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return postgresConnectionPresent();
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Loading connection.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reveal" })).not.toBeInTheDocument();
    releaseConnection();
    expect(await screen.findByRole("button", { name: "Reveal" })).toBeInTheDocument();
  });

  it("POSTs reveal with CSRF, encoded path, and empty body without a confirm dialog", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return postgresReveal200();
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const reveal = await screen.findByRole("button", { name: "Reveal" });
    fireEvent.click(reveal);
    expect(screen.queryByRole("dialog", { name: /reveal/i })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isConnectionRevealUrl(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const revealCall = fetch.mock.calls.find((call) => isConnectionRevealUrl(String(call[0]), "project_a", call[1]));
    expect(revealCall?.[0]).toBe("/api/v1/postgres/databases/project_a/connection/reveal");
    expect(revealCall?.[0]).toBe(`/api/v1/postgres/databases/${encodeURIComponent("project_a")}/connection/reveal`);
    expect(new Headers(revealCall?.[1]?.headers).get("X-CSRF-Token")).toBe("conn-reveal".padEnd(64, "0"));
    expect(revealCall?.[1]?.body == null || revealCall?.[1]?.body === "").toBe(true);
    expect(String(revealCall?.[1]?.body ?? "")).not.toContain("password");
  });

  it("opens a PostgreSQL credential ticket on reveal 200 and does not paint Redis one-time copy", async () => {
    const writeText = vi.fn();
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-200".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return postgresReveal200({
          credential: {
            username: "project_a_role",
            password: "canary-pg-reveal-password-32chars!!",
            one_time: false,
            extra_secret: "should-not-render",
            urls: {
              direct: revealedDirectUrl,
              pooled: revealedPooledUrl,
            },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    const ticket = await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." });
    expect(ticket).toHaveTextContent("Redgres can show this password again from the encrypted vault.");
    expect(ticket).toHaveTextContent("It is not a one-time Redis credential.");
    expect(ticket).not.toHaveTextContent(/shown now/i);
    expect(ticket).not.toHaveTextContent(/cannot show the password again/i);
    expect(ticket).not.toHaveTextContent(
      "Update every application using this project user. The previous password stops working.",
    );
    expect(ticket).toHaveTextContent("canary-pg-reveal-password-32chars!!");
    expect(ticket).toHaveTextContent("project_a_role");
    expect(within(ticket).getByRole("button", { name: "Copy username" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy password" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy Direct URL" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy Pooled URL" })).toBeInTheDocument();
    expect(within(ticket).queryByRole("button", { name: "Copy URL" })).not.toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "I have copied it — dismiss" })).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render");
    expect(writeText).not.toHaveBeenCalled();
    expect(localSet).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Reveal" })).toBeDisabled();
    fireEvent.click(within(ticket).getByRole("button", { name: "Copy Direct URL" }));
    expect(writeText).toHaveBeenCalledWith(revealedDirectUrl);
    localSet.mockRestore();
  });

  it("shows Direct URL and Pooled URL copy on reveal only when those keys are present", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-urls".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return postgresReveal200({
          credential: {
            username: "project_a_role",
            password: "canary-pg-reveal-password-32chars!!",
            one_time: false,
            urls: { direct: revealedDirectUrl },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    const ticket = await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." });
    expect(within(ticket).getByRole("button", { name: "Copy Direct URL" })).toBeInTheDocument();
    expect(within(ticket).queryByRole("button", { name: "Copy Pooled URL" })).not.toBeInTheDocument();
    expect(ticket).toHaveTextContent(revealedDirectUrl);
    expect(ticket).not.toHaveTextContent(revealedPooledUrl);
  });

  it("clears secrets on reveal 401 and does not leave a leftover password", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-401".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return jsonResponse(401, {
          error: { code: "unauthorized", message: "Authentication required" },
          credential: { username: "project_a_role", password: "should-not-render-401" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your session has expired. Sign in again to continue.",
    );
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("should-not-render-401")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-401");
  });

  it("shows not-found copy on reveal 404 and does not open a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-404".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return jsonResponse(404, {
          error: { code: "not_found", message: "Not found" },
          credential: { password: "should-not-render-404" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-404");
  });

  it("shows PostgreSQL is unavailable on reveal 503 and does not open a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-503".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
          credential: { password: "should-not-render-503" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-503");
  });

  it("clears the PostgreSQL ticket on dismiss, selection change, and logout", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-clear".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
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
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isConnectionUrl(url, "project_b")) {
        return postgresConnectionAbsent({
          database: "project_b",
          owner: "owner_b",
          saved_credential: { status: "missing", reason: "" },
        });
      }
      if (isDetailsUrl(url, "project_a") || isDetailsUrl(url, "project_b")) {
        const name = url.includes("project_b") ? "project_b" : "project_a";
        return jsonResponse(200, {
          database: {
            name,
            owner: name === "project_b" ? "owner_b" : "owner_a",
            saved_credential: { status: name === "project_b" ? "missing" : "present", reason: "" },
          },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
        return postgresReveal200();
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    expect(await screen.findByText("canary-pg-reveal-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(screen.queryByText("canary-pg-reveal-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-pg-reveal-password-32chars!!");
    fireEvent.click(screen.getByRole("button", { name: "Reveal" }));
    expect(await screen.findByText("canary-pg-reveal-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(await screen.findByText("owner_b")).toBeInTheDocument();
    expect(screen.queryByText("canary-pg-reveal-password-32chars!!")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Reveal" }));
    expect(await screen.findByText("canary-pg-reveal-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByText("canary-pg-reveal-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-pg-reveal-password-32chars!!");
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("disables Reveal while reveal is in flight", async () => {
    let releaseReveal: () => void = () => {};
    const blockedReveal = new Promise<void>((resolve) => {
      releaseReveal = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "conn-reveal-flight".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "project_a_role", saved_credential: { status: "present", reason: "" } },
        });
      }
      if (isConnectionRevealUrl(url, "project_a", init)) {
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
          void blockedReveal.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return postgresReveal200();
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const reveal = await screen.findByRole("button", { name: "Reveal" });
    fireEvent.click(reveal);
    expect(reveal).toBeDisabled();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    releaseReveal();
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
  });

  it("shows inspector Rotate when security flags are eligible", async () => {
    stubFetch(postgresRotateInspectorFetch("pg-rotate-show".padEnd(64, "0")));
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    const rotate = await within(details).findByRole("button", { name: "Rotate" });
    expect(rotate).toHaveClass("text-button");
    expect(rotate).not.toHaveClass("danger-button");
    expect(rotate.className).not.toMatch(/danger/);
    expect(rotate).toBeEnabled();
  });

  it("shows Rotate when saved credential is missing if security flags are eligible", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-missing".padEnd(64, "0"), {
        details: postgresRotateEligibleDatabase({ saved_credential: { status: "missing", reason: "" } }),
        connection: postgresConnectionAbsent(),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(await within(details).findByRole("button", { name: "Rotate" })).toBeEnabled();
    expect(within(details).queryByRole("button", { name: "Reveal" })).not.toBeInTheDocument();
  });

  it("hides Rotate when the owner is a superuser", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-super".padEnd(64, "0"), {
        details: postgresRotateEligibleDatabase({
          security: {
            public_can_connect: false,
            owner_is_superuser: true,
            owner_can_login: true,
            owner_createdb: false,
            owner_createrole: false,
            owner_replication: false,
          },
        }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(await within(details).findByText("Saved credential")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
  });

  it("hides Rotate when the owner cannot log in", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-nologin".padEnd(64, "0"), {
        details: postgresRotateEligibleDatabase({
          security: {
            public_can_connect: false,
            owner_is_superuser: false,
            owner_can_login: false,
            owner_createdb: false,
            owner_createrole: false,
            owner_replication: false,
          },
        }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(await within(details).findByText("Saved credential")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
  });

  it("hides Rotate when the owner is empty", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-noowner".padEnd(64, "0"), {
        details: postgresRotateEligibleDatabase({ owner: "" }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(await within(details).findByText("Saved credential")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
  });

  it("hides Rotate while details are loading", async () => {
    let releaseDetails: () => void = () => {};
    const blockedDetails = new Promise<void>((resolve) => {
      releaseDetails = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-rotate-load-d".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
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
          void blockedDetails.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, { database: postgresRotateEligibleDatabase() });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Loading details.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
    releaseDetails();
    expect(await screen.findByRole("button", { name: "Rotate" })).toBeInTheDocument();
  });

  it("does not POST rotate until the typed database name matches and Rotate now is clicked", async () => {
    const fetch = stubFetch(postgresRotateInspectorFetch("pg-rotate-typed".padEnd(64, "0")));
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    expect(dialog).toHaveTextContent(/stop working/i);
    expect(dialog).toHaveTextContent(/saved in the encrypted vault/i);
    expect(dialog).toHaveTextContent(/update every application/i);
    expect(dialog).not.toHaveTextContent(/cannot be recovered/i);
    expect(dialog.querySelector("input[type=password]")).toBeNull();
    const rotateNow = within(dialog).getByRole("button", { name: "Rotate now" });
    expect(rotateNow).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project" } });
    expect(rotateNow).toBeDisabled();
    fireEvent.click(rotateNow);
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), "project_a", call[1]))).toBe(
      true,
    );
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    expect(rotateNow).toBeEnabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), "project_a", call[1]))).toBe(
      true,
    );
  });

  it("POSTs rotate with CSRF, encoded path, and JSON confirmation only", async () => {
    const fetch = stubFetch(
      postgresRotateInspectorFetch("pg-rotate-csrf".padEnd(64, "0"), {
        rotate: () => postgresRotate200(),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isPostgresCredentialsRotate(String(call[0]), "project_a", call[1]))).toBe(
        true,
      );
    });
    const rotateCall = fetch.mock.calls.find((call) =>
      isPostgresCredentialsRotate(String(call[0]), "project_a", call[1]),
    );
    expect(rotateCall?.[0]).toBe("/api/v1/postgres/databases/project_a/credentials/rotate");
    expect(rotateCall?.[0]).toBe(
      `/api/v1/postgres/databases/${encodeURIComponent("project_a")}/credentials/rotate`,
    );
    expect(new Headers(rotateCall?.[1]?.headers).get("X-CSRF-Token")).toBe("pg-rotate-csrf".padEnd(64, "0"));
    expect(JSON.parse(String(rotateCall?.[1]?.body))).toEqual({ confirmation: "project_a" });
    const body = JSON.parse(String(rotateCall?.[1]?.body));
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("owner_password");
    expect(body).not.toHaveProperty("role_password");
  });

  it("opens a vault-repeatable PostgreSQL ticket with rotate warning on HTTP 200", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-200".padEnd(64, "0"), {
        rotate: () =>
          postgresRotate200({
            credential: {
              username: "project_a_role",
              password: "canary-pg-rotate-password-32chars!!",
              one_time: false,
              extra_secret: "should-not-render-rotate",
              urls: {
                direct: rotatedDirectUrl,
                pooled: rotatedPooledUrl,
              },
            },
          }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    const ticket = screen.getByRole("alertdialog", { name: "This PostgreSQL password is still saved." });
    expect(ticket).toHaveTextContent("Redgres can show this password again from the encrypted vault.");
    expect(ticket).not.toHaveTextContent(/shown now/i);
    expect(ticket).not.toHaveTextContent(/cannot show the password again/i);
    expect(ticket).toHaveTextContent(
      "Update every application using this project user. The previous password stops working.",
    );
    expect(ticket.querySelector(".form-warning")).not.toBeNull();
    expect(ticket).toHaveTextContent("canary-pg-rotate-password-32chars!!");
    expect(document.body.textContent).not.toContain("should-not-render-rotate");
    expect(localSet).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Rotate" })).toBeDisabled();
    localSet.mockRestore();
  });

  it("clears secrets on rotate 401 and does not leave a leftover password", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-401".padEnd(64, "0"), {
        rotate: () =>
          jsonResponse(401, {
            error: { code: "unauthorized", message: "Authentication required" },
            credential: { username: "project_a_role", password: "should-not-render-rotate-401" },
          }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your session has expired. Sign in again to continue.",
    );
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-rotate-401");
  });

  it.each([
    [400, "Type the database name exactly to confirm rotation"],
    [403, "This PostgreSQL name is protected"],
  ] as const)("stays on the rotate dialog for HTTP %s", async (status, message) => {
    stubFetch(
      postgresRotateInspectorFetch(`pg-rotate-${status}`.padEnd(64, "0"), {
        rotate: () => jsonResponse(status, { error: { message } }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(message);
    expect(screen.getByRole("dialog", { name: "Rotate password?" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("shows not-found copy on rotate 404 and does not open a ticket", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-404".padEnd(64, "0"), {
        rotate: () =>
          jsonResponse(404, {
            error: { code: "not_found", message: "Not found" },
            credential: { password: "should-not-render-rotate-404" },
          }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-rotate-404");
  });

  it("shows PostgreSQL is unavailable on rotate 503 and does not open a ticket", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-503".padEnd(64, "0"), {
        rotate: () =>
          jsonResponse(503, {
            error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
            credential: { password: "should-not-render-rotate-503" },
          }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-rotate-503");
  });

  it("shows vault-out-of-sync copy on rotate 503 when the vault could not be saved", async () => {
    stubFetch(
      postgresRotateInspectorFetch("pg-rotate-vault".padEnd(64, "0"), {
        rotate: () =>
          jsonResponse(503, {
            error: { code: "dependency_unavailable", message: vaultOutOfSyncCopy },
            credential: { password: "should-not-render-rotate-vault" },
          }),
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(vaultOutOfSyncCopy);
    expect(screen.getByRole("dialog", { name: "Rotate password?" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-rotate-vault");
  });

  it("disables Rotate while rotate is in flight", async () => {
    let releaseRotate: () => void = () => {};
    const blockedRotate = new Promise<void>((resolve) => {
      releaseRotate = resolve;
    });
    stubFetch(async (url, init) => {
      const base = postgresRotateInspectorFetch("pg-rotate-flight".padEnd(64, "0"), {
        rotate: async () => {
          await blockedRotate;
          return postgresRotate200();
        },
      });
      return base(url, init);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.change(within(dialog).getByLabelText("Confirm database name"), { target: { value: "project_a" } });
    const rotateNow = within(dialog).getByRole("button", { name: "Rotate now" });
    fireEvent.click(rotateNow);
    await waitFor(() => {
      expect(rotateNow).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Rotate" })).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Reveal" })).toBeDisabled();
    });
    releaseRotate();
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
  });

  it("never POSTs postgres rotate from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-rotate-login".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-rotate-login".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(screen.queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
  });

  it("never POSTs postgres rotate from Security overview", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-rotate-sec".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    const article = screen.getByRole("heading", { name: "Security overview" }).closest("article");
    expect(article).not.toBeNull();
    expect(within(article as HTMLElement).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresCredentialsRotate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
  });

  it("shows inspector Duplicate when details are loaded and security flags are eligible", async () => {
    stubFetch(postgresDuplicateInspectorFetch("pg-dup-show".padEnd(64, "0")));
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    const duplicate = await within(details).findByRole("button", { name: "Duplicate" });
    expect(duplicate).toHaveClass("text-button");
    expect(duplicate).not.toHaveClass("danger-button");
    expect(duplicate.className).not.toMatch(/danger/);
    expect(duplicate).toBeEnabled();
    expect(within(databasesHeader()).queryByRole("button", { name: "Duplicate" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    const nav = screen.getByRole("dialog", { name: "Navigation" });
    expect(within(nav).queryByRole("button", { name: "Duplicate" })).not.toBeInTheDocument();
  });

  it("hides Duplicate while details are loading", async () => {
    let releaseDetails: () => void = () => {};
    const blockedDetails = new Promise<void>((resolve) => {
      releaseDetails = resolve;
    });
    stubFetch(async (url, init) => {
      const base = postgresDuplicateInspectorFetch("pg-dup-load".padEnd(64, "0"));
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
          void blockedDetails.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            resolve();
          });
        });
        return jsonResponse(200, { database: postgresDuplicateEligibleDatabase() });
      }
      return base(url, init);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("Loading details.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Duplicate" })).not.toBeInTheDocument();
    releaseDetails();
    expect(await screen.findByRole("button", { name: "Duplicate" })).toBeInTheDocument();
  });

  it.each([
    [
      "owner is a superuser",
      postgresDuplicateEligibleDatabase({
        security: {
          public_can_connect: false,
          owner_is_superuser: true,
          owner_can_login: true,
          owner_createdb: false,
          owner_createrole: false,
          owner_replication: false,
        },
      }),
    ],
    [
      "owner cannot log in",
      postgresDuplicateEligibleDatabase({
        security: {
          public_can_connect: false,
          owner_is_superuser: false,
          owner_can_login: false,
          owner_createdb: false,
          owner_createrole: false,
          owner_replication: false,
        },
      }),
    ],
    ["owner is empty", postgresDuplicateEligibleDatabase({ owner: "" })],
    [
      "security flags are missing",
      postgresDuplicateEligibleDatabase({
        security: { public_can_connect: false },
      }),
    ],
  ] as const)("hides Duplicate when %s", async (_label, details) => {
    stubFetch(
      postgresDuplicateInspectorFetch(`pg-dup-hide-${_label}`.replace(/\s+/g, "-").padEnd(64, "0"), {
        details,
      }),
    );
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const inspector = await screen.findByRole("region", { name: "Database details" });
    expect(await within(inspector).findByText("Saved credential")).toBeInTheDocument();
    expect(within(inspector).queryByRole("button", { name: "Duplicate" })).not.toBeInTheDocument();
  });

  it("discloses terminated connections including 0 in the duplicate dialog warning", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-warn0".padEnd(64, "0"), {
        details: postgresDuplicateEligibleDatabase({ connection_count: 0 }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    expect(dialog).toHaveAttribute("role", "dialog");
    expect(within(dialog).getByRole("heading", { name: "Duplicate database" })).toBeInTheDocument();
    const warning = dialog.querySelector(".form-warning");
    expect(warning).not.toBeNull();
    expect(warning).toHaveClass("form-warning");
    expect(warning?.className).not.toMatch(/danger/);
    expect(dialog).toHaveTextContent("A unique project user is required.");
    expect(dialog).toHaveTextContent("0 active connections to project_a will be terminated");
    expect(dialog).toHaveTextContent("Object owners inside the copy change.");
    expect(dialog).toHaveTextContent("Source ownership is verified unchanged.");
    expect(dialog).toHaveTextContent("Redgres generates the password and saves it in the encrypted vault.");
    expect(within(dialog).queryByLabelText("Password")).not.toBeInTheDocument();
    expect(dialog.querySelector("input[type=password]")).toBeNull();
  });

  it("discloses a non-zero connection count in the duplicate dialog warning", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-warn3".padEnd(64, "0"), {
        details: postgresDuplicateEligibleDatabase({ connection_count: 3 }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    expect(dialog).toHaveTextContent("3 active connections to project_a will be terminated");
  });

  it("does not POST duplicate until identifiers are valid, the name differs from the source, and Duplicate is clicked", async () => {
    const fetch = stubFetch(postgresDuplicateInspectorFetch("pg-dup-typed".padEnd(64, "0")));
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    const submit = within(dialog).getByRole("button", { name: "Duplicate" });
    expect(submit).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("New database name"), { target: { value: "1db" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("");
    expect(submit).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("New database name"), { target: { value: "project_a" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("app_project_a");
    expect(submit).toBeDisabled();
    fireEvent.click(submit);
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), "project_a", call[1]))).toBe(
      true,
    );
    fireEvent.change(within(dialog).getByLabelText("New database name"), { target: { value: "project_a_copy" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("app_project_a_copy");
    expect(submit).toBeEnabled();
    fireEvent.change(within(dialog).getByLabelText("Project user"), { target: { value: "custom_owner" } });
    fireEvent.change(within(dialog).getByLabelText("New database name"), { target: { value: "project_b" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("custom_owner");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Duplicate database" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), "project_a", call[1]))).toBe(
      true,
    );
  });

  it("POSTs duplicate with CSRF, encoded source, and JSON { database, owner } only", async () => {
    const fetch = stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-csrf".padEnd(64, "0"), {
        duplicate: () => postgresDuplicate201(),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isPostgresDatabaseDuplicate(String(call[0]), "project_a", call[1]))).toBe(
        true,
      );
    });
    const duplicateCall = fetch.mock.calls.find((call) =>
      isPostgresDatabaseDuplicate(String(call[0]), "project_a", call[1]),
    );
    expect(duplicateCall?.[0]).toBe("/api/v1/postgres/databases/project_a/duplicate");
    expect(duplicateCall?.[0]).toBe(
      `/api/v1/postgres/databases/${encodeURIComponent("project_a")}/duplicate`,
    );
    expect(new Headers(duplicateCall?.[1]?.headers).get("X-CSRF-Token")).toBe("pg-dup-csrf".padEnd(64, "0"));
    const body = JSON.parse(String(duplicateCall?.[1]?.body));
    expect(body).toEqual({ database: "project_a_copy", owner: "app_project_a_copy" });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("new_owner_password");
    expect(body).not.toHaveProperty("confirmation");
    expect(body).not.toHaveProperty("create_owner_role");
    expect(body).not.toHaveProperty("owner_password");
  });

  it("opens a vault-repeatable PostgreSQL ticket on duplicate 201, refreshes the list, and selects the new database after dismiss", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    const fetch = stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-201".padEnd(64, "0"), {
        duplicate: () =>
          postgresDuplicate201({
            credential: {
              username: "app_project_a_copy",
              password: "canary-pg-duplicate-password-32chars!!",
              one_time: false,
              extra_secret: "should-not-render-duplicate",
              urls: {
                direct: duplicatedDirectUrl,
                pooled: duplicatedPooledUrl,
              },
            },
          }),
        listAfterDuplicate: () =>
          jsonResponse(200, {
            databases: [
              { name: "project_a", owner: "project_a_role" },
              { name: "project_a_copy", owner: "app_project_a_copy" },
            ],
            truncated: false,
          }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Duplicate database" })).not.toBeInTheDocument();
    const ticket = screen.getByRole("alertdialog", { name: "This PostgreSQL password is still saved." });
    expect(ticket).toHaveTextContent("Redgres can show this password again from the encrypted vault.");
    expect(ticket).toHaveTextContent("It is not a one-time Redis credential.");
    expect(ticket).not.toHaveTextContent(/shown now/i);
    expect(ticket).not.toHaveTextContent(/cannot show the password again/i);
    expect(ticket).not.toHaveTextContent(
      "Update every application using this project user. The previous password stops working.",
    );
    expect(ticket).toHaveTextContent("canary-pg-duplicate-password-32chars!!");
    expect(document.body.textContent).not.toContain("should-not-render-duplicate");
    expect(localSet).not.toHaveBeenCalled();
    const details = screen.getByRole("region", { name: "Database details" });
    expect(within(details).getByRole("button", { name: "Duplicate" })).toBeDisabled();
    expect(within(details).getByRole("button", { name: "Rotate" })).toBeDisabled();
    expect(within(details).getByRole("button", { name: "Reveal" })).toBeDisabled();
    expect(within(databasesHeader()).getByRole("button", { name: "Create database" })).toBeDisabled();
    const listGets = fetch.mock.calls.filter(
      (call) =>
        String(call[0]).endsWith("/api/v1/postgres/databases") &&
        !isPostgresDatabasesCreate(String(call[0]), call[1]) &&
        !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]),
    );
    expect(listGets.length).toBeGreaterThan(1);
    fireEvent.click(within(ticket).getByRole("button", { name: "I have copied it — dismiss" }));
    expect(screen.queryByText("canary-pg-duplicate-password-32chars!!")).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "project_a_copy" })).toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("clears secrets on duplicate 401 and does not leave a leftover password", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-401".padEnd(64, "0"), {
        duplicate: () =>
          jsonResponse(401, {
            error: { code: "unauthorized", message: "Authentication required" },
            credential: { username: "app_project_a_copy", password: "should-not-render-duplicate-401" },
          }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your session has expired. Sign in again to continue.",
    );
    expect(screen.queryByRole("dialog", { name: "Duplicate database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-duplicate-401");
  });

  it.each([
    [400, "Invalid database name"],
    [403, "This PostgreSQL name is protected"],
    [409, "A PostgreSQL database with this name already exists"],
  ] as const)("stays on the duplicate dialog for HTTP %s", async (status, message) => {
    stubFetch(
      postgresDuplicateInspectorFetch(`pg-dup-${status}`.padEnd(64, "0"), {
        duplicate: () => jsonResponse(status, { error: { message } }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(message);
    expect(screen.getByRole("dialog", { name: "Duplicate database" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("shows not-found copy on duplicate 404 and does not open a ticket", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-404".padEnd(64, "0"), {
        duplicate: () =>
          jsonResponse(404, {
            error: { code: "not_found", message: "Not found" },
            credential: { password: "should-not-render-duplicate-404" },
          }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("dialog", { name: "Duplicate database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-duplicate-404");
  });

  it("shows PostgreSQL is unavailable on duplicate 503 and does not open a ticket", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-503".padEnd(64, "0"), {
        duplicate: () =>
          jsonResponse(503, {
            error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
            credential: { password: "should-not-render-duplicate-503" },
          }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByRole("dialog", { name: "Duplicate database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-duplicate-503");
  });

  it("stays on the duplicate dialog for isolation-rollback 503", async () => {
    stubFetch(
      postgresDuplicateInspectorFetch("pg-dup-isol".padEnd(64, "0"), {
        duplicate: () =>
          jsonResponse(503, {
            error: { code: "dependency_unavailable", message: isolationRollbackCopy },
            credential: { password: "should-not-render-duplicate-isol" },
          }),
      }),
    );
    render(<App />);
    const dialog = await openDuplicateDatabaseDialog();
    fillDuplicateForm(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(isolationRollbackCopy);
    expect(screen.getByRole("dialog", { name: "Duplicate database" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render-duplicate-isol");
  });

  it("disables Duplicate, Create, Rotate, and Reveal while duplicate is in flight", async () => {
    let releaseDuplicate: () => void = () => {};
    const blockedDuplicate = new Promise<void>((resolve) => {
      releaseDuplicate = resolve;
    });
    stubFetch(async (url, init) => {
      const base = postgresDuplicateInspectorFetch("pg-dup-flight".padEnd(64, "0"), {
        duplicate: async () => {
          await blockedDuplicate;
          return postgresDuplicate201();
        },
      });
      return base(url, init);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Duplicate" }));
    const dialog = await screen.findByRole("dialog", { name: "Duplicate database" });
    fillDuplicateForm(dialog);
    const submit = within(dialog).getByRole("button", { name: "Duplicate" });
    fireEvent.click(submit);
    await waitFor(() => {
      expect(submit).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Duplicate" })).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Rotate" })).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Reveal" })).toBeDisabled();
      expect(within(databasesHeader()).getByRole("button", { name: "Create database" })).toBeDisabled();
    });
    releaseDuplicate();
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
  });

  it("never POSTs postgres duplicate from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-dup-login".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-dup-login".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
    expect(screen.queryByRole("button", { name: "Duplicate" })).not.toBeInTheDocument();
  });

  it("never POSTs postgres duplicate from Security overview", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-dup-sec".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    const article = screen.getByRole("heading", { name: "Security overview" }).closest("article");
    expect(article).not.toBeNull();
    expect(within(article as HTMLElement).queryByRole("button", { name: /duplicate/i })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
  });

  it("never POSTs duplicate from search results", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-dup-search".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return postgresHitSearch();
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isConnectionUrl(url, "project_a")) {
        return postgresConnectionPresent();
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: postgresDuplicateEligibleDatabase() });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const search = await screen.findByRole("dialog", { name: "Search" });
    fireEvent.click(await within(search).findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("region", { name: "Database details" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Duplicate" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabaseDuplicate(String(call[0]), undefined, call[1]))).toBe(
      true,
    );
  });

  it("shows Create database in the Databases header, not the topbar, including an empty list", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-empty".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    expect(await screen.findByText("No manageable project databases.")).toBeInTheDocument();
    const header = databasesHeader();
    expect(within(header).getByRole("button", { name: "Create database" })).toBeInTheDocument();
    expect(document.querySelector(".topbar")).not.toBeNull();
    expect(
      within(document.querySelector(".topbar") as HTMLElement).queryByRole("button", { name: /create/i }),
    ).not.toBeInTheDocument();
  });

  it("hides Create database while the list is loading", async () => {
    let releaseList: () => void = () => {};
    const blockedList = new Promise<void>((resolve) => {
      releaseList = resolve;
    });
    stubFetch(async (url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-load".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        await blockedList;
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    expect(await screen.findByText("Loading databases.")).toBeInTheDocument();
    expect(within(databasesHeader()).queryByRole("button", { name: "Create database" })).not.toBeInTheDocument();
    releaseList();
    expect(await within(databasesHeader()).findByRole("button", { name: "Create database" })).toBeInTheDocument();
  });

  it("does not offer Create on the inspector", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-insp".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [{ name: "project_a", owner: "project_a_role" }], truncated: false });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, { database: { name: "project_a", owner: "project_a_role" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "Database details" });
    expect(within(details).queryByRole("button", { name: "Create database" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Create" })).not.toBeInTheDocument();
  });

  it("disables Create for invalid identifiers and suggests app_${database} until Project user is edited", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-valid".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "1db" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("");
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "bad-name" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("");
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "a".repeat(64) } });
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("app_project_a");
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeEnabled();
    fireEvent.change(within(dialog).getByLabelText("Project user"), { target: { value: "custom_owner" } });
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_b" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("custom_owner");
    fireEvent.change(within(dialog).getByLabelText("Project user"), { target: { value: "1owner" } });
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    expect(within(dialog).queryByLabelText("Password")).not.toBeInTheDocument();
    expect(dialog.querySelector("input[type=password]")).toBeNull();
    expect(dialog).toHaveTextContent("Redgres generates the password and saves it in the encrypted vault.");
    expect(dialog).toHaveTextContent(
      "Direct 5432 vs pooled 6432; TLS required; PUBLIC CONNECT revoked; 20-connection role limit.",
    );
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
  });

  it("POSTs CSRF JSON { database, owner } and opens the PostgreSQL vault ticket on 201", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    let created = false;
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-201".padEnd(64, "0") });
      }
      if (isPostgresDatabasesCreate(url, init)) {
        created = true;
        return postgresCreate201({
          credential: {
            username: "app_project_a",
            password: "canary-pg-create-password-32chars!!",
            one_time: false,
            extra_secret: "should-not-render",
            urls: {
              direct: createdDirectUrl,
              pooled: createdPooledUrl,
            },
          },
        });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, {
          databases: created ? [{ name: "project_a", owner: "app_project_a" }] : [],
          truncated: false,
        });
      }
      if (isTablesUrl(url, "project_a")) {
        return jsonResponse(200, { tables: [], truncated: false });
      }
      if (isDetailsUrl(url, "project_a")) {
        return jsonResponse(200, {
          database: { name: "project_a", owner: "app_project_a", saved_credential: { status: "present", reason: "" } },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    expect(within(dialog).getByLabelText("Project user")).toHaveValue("app_project_a");
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
    });
    const createCall = fetch.mock.calls.find((call) => isPostgresDatabasesCreate(String(call[0]), call[1]));
    expect(createCall?.[0]).toBe("/api/v1/postgres/databases");
    expect(new Headers(createCall?.[1]?.headers).get("X-CSRF-Token")).toBe("pg-create-201".padEnd(64, "0"));
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body).toEqual({ database: "project_a", owner: "app_project_a" });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("role_password");
    expect(body).not.toHaveProperty("create_role");
    expect(screen.queryByRole("dialog", { name: "Create database" })).not.toBeInTheDocument();
    const ticket = await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." });
    expect(ticket).toHaveTextContent("Redgres can show this password again from the encrypted vault.");
    expect(ticket).toHaveTextContent("It is not a one-time Redis credential.");
    expect(ticket).not.toHaveTextContent(/shown now/i);
    expect(ticket).not.toHaveTextContent(
      "Update every application using this project user. The previous password stops working.",
    );
    expect(ticket).toHaveTextContent("canary-pg-create-password-32chars!!");
    expect(ticket).toHaveTextContent("app_project_a");
    expect(within(ticket).getByRole("button", { name: "Copy Direct URL" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy Pooled URL" })).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render");
    expect(localSet).not.toHaveBeenCalled();
    expect(within(databasesHeader()).getByRole("button", { name: "Create database" })).toBeDisabled();
    const listGets = fetch.mock.calls.filter(
      (call) =>
        String(call[0]).endsWith("/api/v1/postgres/databases") && !isPostgresDatabasesCreate(String(call[0]), call[1]),
    );
    expect(listGets.length).toBeGreaterThan(1);
    fireEvent.click(within(ticket).getByRole("button", { name: "I have copied it — dismiss" }));
    expect(screen.queryByText("canary-pg-create-password-32chars!!")).not.toBeInTheDocument();
    expect(await screen.findByRole("region", { name: "Database details" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "project_a" })).toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("does not open Create from nav while a credential ticket is open", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-nav-ticket".padEnd(64, "0") });
      }
      if (isPostgresDatabasesCreate(url, init)) {
        return postgresCreate201();
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
    const postsBeforeNav = fetch.mock.calls.filter((call) => isPostgresDatabasesCreate(String(call[0]), call[1])).length;
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Create database" }),
    );
    expect(screen.queryByRole("dialog", { name: "Create database" })).not.toBeInTheDocument();
    expect(screen.getByRole("alertdialog", { name: "This PostgreSQL password is still saved." })).toBeInTheDocument();
    expect(screen.getByText("canary-pg-create-password-32chars!!")).toBeInTheDocument();
    expect(fetch.mock.calls.filter((call) => isPostgresDatabasesCreate(String(call[0]), call[1])).length).toBe(
      postsBeforeNav,
    );
  });

  it("clears the create ticket when the post-create list GET is 401", async () => {
    let created = false;
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-list-401".padEnd(64, "0") });
      }
      if (isPostgresDatabasesCreate(url, init)) {
        created = true;
        return postgresCreate201();
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        if (created) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("dialog", { name: "Create database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-pg-create-password-32chars!!")).not.toBeInTheDocument();
  });

  it("shows session-expired on create 401 and clears secrets", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-401".padEnd(64, "0") });
      }
      if (isPostgresDatabasesCreate(url, init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("dialog", { name: "Create database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-pg-create-password-32chars!!")).not.toBeInTheDocument();
  });

  it.each([
    [409, "A PostgreSQL database with this name already exists"],
    [403, "This PostgreSQL name is protected"],
    [400, "Invalid database name"],
    [503, "PostgreSQL is unavailable"],
  ] as const)("stays on the create dialog for HTTP %s", async (status, message) => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `pg-create-${status}`.padEnd(64, "0") });
      }
      if (isPostgresDatabasesCreate(url, init)) {
        return jsonResponse(status, { error: { message } });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    const dialog = await openCreateDatabaseDialog();
    fireEvent.change(within(dialog).getByLabelText("Database name"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(message);
    expect(screen.getByRole("dialog", { name: "Create database" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("renders the create form from nav postgres-create instead of the adapter placeholder", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-nav".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "Navigation" })).getByRole("button", { name: "Create database" }),
    );
    expect(screen.queryByText("This adapter is not available yet.")).not.toBeInTheDocument();
    expect(screen.queryByText("This view is not available yet.")).not.toBeInTheDocument();
    expect(await screen.findByRole("dialog", { name: "Create database" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Databases" })).toBeInTheDocument();
  });

  it("never POSTs /api/v1/postgres/databases from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-login".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-login".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
  });

  it("never POSTs create from search results", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-search".padEnd(64, "0") });
      }
      if (isSearchUrl(url)) {
        return disconnectedSearch();
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "create" } });
    const search = await screen.findByRole("dialog", { name: "Search" });
    fireEvent.click(within(search).getByRole("button", { name: /Create database/ }));
    expect(await screen.findByRole("dialog", { name: "Create database" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
  });

  it("never POSTs create from Security overview", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-sec".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isPostgresDatabasesCreate(String(call[0]), call[1]))).toBe(true);
  });

  it("keeps Redis create tickets as one-time shown now after the PostgreSQL create dialog exists", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "pg-create-redis".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    await goToDatabases();
    expect(await within(databasesHeader()).findByRole("button", { name: "Create database" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(ticket).toHaveTextContent(/shown now/i);
    expect(ticket).not.toHaveTextContent("This PostgreSQL password is still saved.");
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
    expect(within(databasesHeader()).queryByRole("button", { name: "Create database" })).not.toBeInTheDocument();
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

  it("shows the Security overview page instead of the placeholder", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-a".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    expect(screen.queryByText("This adapter is not available yet.")).not.toBeInTheDocument();
    expect(screen.queryByText("This view is not available yet.")).not.toBeInTheDocument();
  });

  it("requests Security overview from exactly GET /api/v1/postgres/security without CSRF", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-b".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    const calls = fetch.mock.calls.filter((call) => String(call[0]).includes("/api/v1/postgres/security"));
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.every((call) => String(call[0]) === "/api/v1/postgres/security")).toBe(true);
    const method = calls[0]?.[1]?.method;
    expect(method === undefined || method === "GET").toBe(true);
    expect(new Headers(calls[0]?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
  });

  it("shows Security overview loading then the frozen 200 payload", async () => {
    let release: () => void = () => {};
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-c".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
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
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    expect(await screen.findByRole("status")).toHaveTextContent("Loading security overview.");
    expect(screen.queryByRole("table", { name: "Connection groups" })).not.toBeInTheDocument();
    release();
    expect(await screen.findByRole("table", { name: "Database security" })).toBeInTheDocument();
    expect(screen.queryByText("Loading security overview.")).not.toBeInTheDocument();
  });

  it("shows protected and project database rows with PUBLIC CONNECT and owner flags", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-d".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    const table = await screen.findByRole("table", { name: "Database security" });
    const rows = within(table).getAllByRole("row");
    expect(rows.length).toBe(3);
    const postgresCells = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(postgresCells[0]).toHaveTextContent("postgres");
    expect(postgresCells[0].querySelector(".identifier.bidi-isolate")).not.toBeNull();
    expect(postgresCells[2]).toHaveTextContent("Protected");
    expect(postgresCells[3]).toHaveTextContent("No");
    expect(postgresCells[4]).toHaveTextContent("Yes");
    const projectCells = within(rows[2] as HTMLElement).getAllByRole("cell");
    expect(projectCells[0]).toHaveTextContent("project_a");
    expect(projectCells[0].querySelector(".identifier.bidi-isolate")).not.toBeNull();
    expect(projectCells[2]).not.toHaveTextContent("Protected");
    expect(projectCells[3]).toHaveTextContent("Yes");
    expect(projectCells[4]).toHaveTextContent("No");
    const article = screen.getByRole("heading", { name: "Security overview" }).closest("article");
    expect(article).not.toBeNull();
    expect(within(article as HTMLElement).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
  });

  it("shows Rotation eligible Yes and No as the last ledger and stack column", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-rot".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("heading", { name: "Security overview" })).toBeInTheDocument();
    expect(screen.getByText(/Rotation is not available/)).toBeInTheDocument();

    const table = screen.getByRole("table", { name: "Database security" });
    const rows = within(table).getAllByRole("row");
    const headers = within(rows[0] as HTMLElement).getAllByRole("columnheader");
    expect(headers[headers.length - 2]).toHaveTextContent("Connections");
    expect(headers[headers.length - 1]).toHaveTextContent("Rotation eligible");
    const postgresCells = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(postgresCells[0]).toHaveTextContent("postgres");
    expect(postgresCells[postgresCells.length - 1]).toHaveTextContent("No");
    const projectCells = within(rows[2] as HTMLElement).getAllByRole("cell");
    expect(projectCells[0]).toHaveTextContent("project_a");
    expect(projectCells[projectCells.length - 1]).toHaveTextContent("Yes");

    const stack = screen.getByRole("list", { name: "Database security" });
    const items = within(stack).getAllByRole("listitem");
    const postgresTerms = within(items[0] as HTMLElement).getAllByRole("term");
    expect(postgresTerms[postgresTerms.length - 1]).toHaveTextContent("Rotation eligible");
    expect(within(items[0] as HTMLElement).getByText("Rotation eligible").closest("div")).toHaveTextContent("No");
    const projectTerms = within(items[1] as HTMLElement).getAllByRole("term");
    expect(projectTerms[projectTerms.length - 1]).toHaveTextContent("Rotation eligible");
    expect(within(items[1] as HTMLElement).getByText("Rotation eligible").closest("div")).toHaveTextContent("Yes");

    const article = screen.getByRole("heading", { name: "Security overview" }).closest("article");
    expect(article).not.toBeNull();
    expect(within(article as HTMLElement).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByText("Rotate")).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByText("Reveal")).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByText("Create")).not.toBeInTheDocument();
  });

  it("shows Rotation eligible as an em dash when the field is missing", async () => {
    const missingEligibility = postgresSecurityDatabase();
    delete missingEligibility.rotation_eligible;
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-rot-miss".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({ databases: [missingEligibility] });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    const table = await screen.findByRole("table", { name: "Database security" });
    const rows = within(table).getAllByRole("row");
    const postgresCells = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(postgresCells[postgresCells.length - 1]).toHaveTextContent("—");
    const stack = screen.getByRole("list", { name: "Database security" });
    const items = within(stack).getAllByRole("listitem");
    expect(within(items[0] as HTMLElement).getByText("Rotation eligible").closest("div")).toHaveTextContent("—");
  });

  it("shows Missing vault entries when cluster saved_credential is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-e".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByText("Missing vault entries")).toBeInTheDocument();
    expect(factValue("Missing vault entries")).toHaveTextContent("1");
    expect(screen.queryByText("Saved credential")).not.toBeInTheDocument();
    expect(screen.queryByText("Not available")).not.toBeInTheDocument();
    expect(screen.queryByText(/not loaded in this slice/i)).not.toBeInTheDocument();
    expect(screen.getByText(/Passwords are not revealed/)).toBeInTheDocument();
    const article = screen.getByRole("heading", { name: "Security overview" }).closest("article");
    expect(article).not.toBeNull();
    expect(within(article as HTMLElement).queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument();
    expect(within(article as HTMLElement).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows Missing vault entries 0 when the count is zero", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-e0".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({
          summary: {
            database_count: 2,
            public_connect_count: 1,
            active_connection_count: 3,
            connection_group_count: 2,
            missing_password_count: 0,
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByText("Missing vault entries")).toBeInTheDocument();
    expect(factValue("Missing vault entries")).toHaveTextContent("0");
    expect(screen.queryByText("Saved credential")).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows vault Not available when existence is unavailable and does not invent a count", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-e1".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({
          summary: {
            database_count: 2,
            public_connect_count: 1,
            active_connection_count: 3,
            connection_group_count: 2,
          },
          saved_credential: { status: "not_available", reason: "vault_unavailable" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not available");
    expect(screen.queryByText("Missing vault entries")).not.toBeInTheDocument();
    expect(screen.queryByText(/not loaded in this slice/i)).not.toBeInTheDocument();
    expect(screen.getByText(/Passwords are not revealed/)).toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows Saved credential Not available when saved_credential status is omitted", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-e2".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({
          saved_credential: {},
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByText("Saved credential")).toBeInTheDocument();
    expect(factValue("Saved credential")).toHaveTextContent("Not available");
    expect(screen.queryByText("Missing vault entries")).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("renders the connection groups table from the frozen payload", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-f".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    const table = await screen.findByRole("table", { name: "Connection groups" });
    const rows = within(table).getAllByRole("row");
    expect(rows.length).toBe(3);
    const first = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(first[0].querySelector(".identifier.bidi-isolate")).toHaveTextContent("postgres");
    expect(first[1].querySelector(".identifier.bidi-isolate")).toHaveTextContent("postgres");
    expect(first[2].querySelector(".identifier.bidi-isolate")).toHaveTextContent("local");
    expect(first[3].querySelector(".identifier.bidi-isolate")).toHaveTextContent("redgres");
    expect(first[4]).toHaveTextContent("idle");
    expect(first[5]).toHaveTextContent("1");
    const second = within(rows[2] as HTMLElement).getAllByRole("cell");
    expect(second[0]).toHaveTextContent("project_a");
    expect(second[2].querySelector(".identifier.bidi-isolate")).toHaveTextContent("10.0.0.2");
    expect(second[3].querySelector(".identifier.bidi-isolate")).toHaveTextContent("app");
    expect(second[4]).toHaveTextContent("active");
    expect(second[5]).toHaveTextContent("2");
  });

  it("warns when the security overview is truncated", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-g".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({ truncated: true });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Security overview truncated at 500 databases or connection groups.",
    );
    expect(screen.getByRole("table", { name: "Database security" })).toBeInTheDocument();
  });

  it("shows PostgreSQL unavailable without an empty healthy security overview", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-h".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return jsonResponse(503, {
          error: { code: "dependency_unavailable", message: "PostgreSQL is unavailable" },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("alert")).toHaveTextContent("PostgreSQL is unavailable");
    expect(screen.queryByText("No databases.")).not.toBeInTheDocument();
    expect(screen.queryByText("No connection groups.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Database security" })).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Connection groups" })).not.toBeInTheDocument();
    expect(screen.queryByText("Not available")).not.toBeInTheDocument();
  });

  it("shows a session-expired security alert without overview keys from 401", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-i".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return jsonResponse(401, {
          error: { code: "unauthorized", message: "Authentication required" },
          summary: { database_count: 99 },
          saved_credential: { status: "not_available", reason: "vault_unavailable" },
          databases: [{ name: "should-not-appear" }],
          connections: [{ database: "leaked-connection" }],
          truncated: true,
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your session has expired. Sign in again to continue.",
    );
    expect(screen.queryByText("should-not-appear")).not.toBeInTheDocument();
    expect(screen.queryByText("leaked-connection")).not.toBeInTheDocument();
    expect(screen.queryByText("99")).not.toBeInTheDocument();
    expect(screen.queryByText("No databases.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Database security" })).not.toBeInTheDocument();
    expect(screen.queryByText("Missing vault entries")).not.toBeInTheDocument();
    expect(screen.queryByText("Not available")).not.toBeInTheDocument();
    expectNoVaultReasonCopy();
  });

  it("shows empty security lists only after HTTP 200 with empty arrays", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-j".padEnd(64, "0") });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk({
          summary: {
            database_count: 0,
            public_connect_count: 0,
            active_connection_count: 0,
            connection_group_count: 0,
            missing_password_count: 0,
          },
          databases: [],
          connections: [],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByText("No databases.")).toBeInTheDocument();
    expect(screen.getByText("No connection groups.")).toBeInTheDocument();
    expect(screen.getByText("Missing vault entries")).toBeInTheDocument();
    expect(factValue("Missing vault entries")).toHaveTextContent("0");
    expect(screen.queryByText("Saved credential")).not.toBeInTheDocument();
    expect(screen.queryByText("Not available")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText("PostgreSQL is unavailable")).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Database security" })).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Connection groups" })).not.toBeInTheDocument();
  });

  it("does not fetch PostgreSQL security on the login route", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/postgres/security"))).toBe(true);
  });

  it("does not persist security overview state in localStorage or sessionStorage", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "sec-k".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (url.endsWith("/api/v1/postgres/databases")) {
        return jsonResponse(200, { databases: [], truncated: false });
      }
      if (isPostgresSecurityUrl(url)) {
        return postgresSecurityOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToSecurityOverview();
    expect(await screen.findByRole("table", { name: "Database security" })).toBeInTheDocument();
    expect(screen.getAllByText("project_a").length).toBeGreaterThan(0);
    expect(localSet).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Security overview" })).not.toBeInTheDocument();
    expect(screen.queryByText("project_a")).not.toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
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

  it("shows PgBouncer as Not configured from the default disconnected status", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-pgb-noconf".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
    expect(screen.queryByLabelText("PgBouncer: Not connected")).not.toBeInTheDocument();
    const pgbouncerDefault = screen.getByLabelText("PgBouncer: Not configured");
    expect(within(pgbouncerDefault).queryByText("Version")).not.toBeInTheDocument();
    expect(within(pgbouncerDefault).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(pgbouncerDefault).queryByText("Metrics unavailable")).not.toBeInTheDocument();
  });

  it("shows PgBouncer as Reachable when pgbouncer is ok without postgres or redis rails", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-pgb-ok".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "unavailable", reason: "unreachable" },
            { id: "pgbouncer", state: "ok" },
            { id: "redis", state: "ok" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "pgbouncerok0000000000000000000000",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    const pgbouncer = await screen.findByLabelText("PgBouncer: Reachable");
    expect(pgbouncer).not.toHaveClass("status-card-postgres");
    expect(pgbouncer).not.toHaveClass("status-card-redis");
    expect(within(pgbouncer).getByText("Reachable")).toHaveClass("status-ok");
    expect(within(pgbouncer).queryByText("Version")).not.toBeInTheDocument();
    expect(within(pgbouncer).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PostgreSQL direct: Unavailable")).toHaveClass("status-card-postgres");
    expect(screen.getByLabelText("Redis: Reachable")).toHaveClass("status-card-redis");
  });

  it("shows PgBouncer Unavailable independently when PostgreSQL and Redis are reachable", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-pgb-unavail".padEnd(64, "0") });
      }
      if (isStatusUrl(url)) {
        return jsonResponse(200, {
          components: [
            { id: "redgres_state", state: "ok" },
            { id: "postgres_direct", state: "ok" },
            { id: "pgbouncer", state: "unavailable", reason: "unreachable" },
            { id: "redis", state: "ok" },
            { id: "tool_links", state: "not_configured" },
          ],
          request_id: "pgbouncerunavail0000000000000000",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    const pgbouncer = await screen.findByLabelText("PgBouncer: Unavailable");
    expect(pgbouncer).not.toHaveClass("status-card-postgres");
    expect(pgbouncer).not.toHaveClass("status-card-redis");
    expect(pgbouncer.querySelector(".status-unavailable")).not.toBeNull();
    expect(within(pgbouncer).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(screen.getByLabelText("PostgreSQL direct: Reachable")).toHaveClass("status-card-postgres");
    expect(screen.getByLabelText("Redis: Reachable")).toHaveClass("status-card-redis");
  });

  it("does not fetch /api/v1/status on the login route", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isStatusUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/healthz"))).toBe(true);
    expect(screen.queryByLabelText("PgBouncer: Not configured")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("PgBouncer: Not connected")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Tool links: Not configured")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
  });

  it("does not persist Overview status in localStorage or sessionStorage", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "status-pgb-storage".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByLabelText("PgBouncer: Not configured")).not.toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
    const pgbouncer = screen.getByLabelText("PgBouncer: Not configured");
    expect(pgbouncer).not.toHaveClass("status-card-postgres");
    expect(pgbouncer).not.toHaveClass("status-card-redis");
    expect(pgbouncer.querySelector(".service-rail-postgres")).toBeNull();
    expect(pgbouncer.querySelector(".service-rail-redis")).toBeNull();
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
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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

  it("paints no tool link anchors when session omits tool_links and status is not_configured", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "tools-missing".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Tool links: Not configured")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
  });

  it("paints no tool link anchors when session tool_links is empty", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: "tools-empty".padEnd(64, "0"),
          tool_links: {},
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Tool links: Not configured")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
  });

  it("renders a pgAdmin anchor when only that href is configured", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: "tools-one".padEnd(64, "0"),
          tool_links: { pgadmin: "https://pgadmin.example.com" },
        });
      }
      if (isStatusUrl(url)) {
        return toolLinksOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    const card = await screen.findByLabelText("Tool links: Reachable");
    const link = within(card).getByRole("link", { name: "pgAdmin" });
    expect(link).toHaveAttribute("href", "https://pgadmin.example.com");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(within(card).queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
  });

  it("renders pgAdmin and RedisInsight anchors with opener isolation when both hrefs are configured", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: "tools-both".padEnd(64, "0"),
          tool_links: {
            pgadmin: "https://pgadmin.example.com",
            redisinsight: "https://redis-insight.example.com",
          },
        });
      }
      if (isStatusUrl(url)) {
        return toolLinksOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    const card = await screen.findByLabelText("Tool links: Reachable");
    const pgAdmin = within(card).getByRole("link", { name: "pgAdmin" });
    const redisInsight = within(card).getByRole("link", { name: "RedisInsight" });
    expect(pgAdmin).toHaveAttribute("href", "https://pgadmin.example.com");
    expect(pgAdmin).toHaveAttribute("target", "_blank");
    expect(pgAdmin).toHaveAttribute("rel", "noopener noreferrer");
    expect(redisInsight).toHaveAttribute("href", "https://redis-insight.example.com");
    expect(redisInsight).toHaveAttribute("target", "_blank");
    expect(redisInsight).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("does not persist tool link hrefs in localStorage or sessionStorage", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: "tools-storage".padEnd(64, "0"),
          tool_links: {
            pgadmin: "https://pgadmin.example.com",
            redisinsight: "https://redis-insight.example.com",
          },
        });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isStatusUrl(url)) {
        return toolLinksOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("link", { name: "pgAdmin" })).toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "RedisInsight" })).not.toBeInTheDocument();
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("does not refetch /session when Overview Refresh is used", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: "tools-refresh".padEnd(64, "0"),
          tool_links: { pgadmin: "https://pgadmin.example.com" },
        });
      }
      if (isStatusUrl(url)) {
        return toolLinksOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("link", { name: "pgAdmin" })).toBeInTheDocument();
    const sessionCallsBefore = fetch.mock.calls.filter((call) => String(call[0]).includes("/api/v1/session")).length;
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await waitFor(() => {
      expect(fetch.mock.calls.filter((call) => isStatusUrl(String(call[0]))).length).toBeGreaterThanOrEqual(2);
    });
    expect(screen.getByRole("link", { name: "pgAdmin" })).toBeInTheDocument();
    const sessionCallsAfter = fetch.mock.calls.filter((call) => String(call[0]).includes("/api/v1/session")).length;
    expect(sessionCallsAfter).toBe(sessionCallsBefore);
  });

  it("GETs /session after login and uses that CSRF plus tool links before the shell", async () => {
    const loginCsrf = "login-csrf-token".padEnd(64, "L");
    const sessionCsrf = "session-csrf-token".padEnd(64, "S");
    let authed = false;
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, {
          owner: { username: "admin" },
          csrf_token: sessionCsrf,
          tool_links: {
            pgadmin: "https://pgadmin.example.com",
            redisinsight: "https://redis-insight.example.com",
          },
        });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: loginCsrf });
      }
      if (url.includes("/api/v1/auth/logout")) {
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe(sessionCsrf);
        return jsonResponse(200, { ok: true });
      }
      if (isStatusUrl(url)) {
        return toolLinksOkStatus();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isStatusUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/healthz"))).toBe(true);
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("link", { name: "pgAdmin" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "RedisInsight" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "admin" })).toBeInTheDocument();
    const urls = fetch.mock.calls.map((call) => String(call[0]));
    const loginIndex = urls.findIndex((url) => url.includes("/api/v1/auth/login"));
    const sessionAfterLogin = urls.findIndex(
      (url, index) => index > loginIndex && url.includes("/api/v1/session"),
    );
    const statusIndex = urls.findIndex((url) => isStatusUrl(url));
    expect(loginIndex).toBeGreaterThanOrEqual(0);
    expect(sessionAfterLogin).toBeGreaterThan(loginIndex);
    expect(statusIndex).toBeGreaterThan(sessionAfterLogin);
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
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
    expect(screen.getByLabelText("PgBouncer: Not connected")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "PgBouncer" })).toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
    const pgbouncer = screen.getByLabelText("PgBouncer: Not configured");
    expect(within(pgbouncer).queryByText("Version")).not.toBeInTheDocument();
    expect(within(pgbouncer).queryByText("Ops/s")).not.toBeInTheDocument();
    expect(within(pgbouncer).queryByText("Used / max memory")).not.toBeInTheDocument();
    expect(within(pgbouncer).queryByText("8.2.1")).not.toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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
            { id: "pgbouncer", state: "not_configured" },
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
            { id: "pgbouncer", state: "not_configured" },
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
            { id: "pgbouncer", state: "not_configured" },
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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
    expect(screen.getByLabelText("PgBouncer: Not configured")).toBeInTheDocument();
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

  it("shows the ACL users ledger instead of the placeholder", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-a".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("heading", { name: "ACL users" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.queryByText("This adapter is not available yet.")).not.toBeInTheDocument();
    const header = screen.getByRole("heading", { name: "ACL users" }).closest("header");
    expect(header).not.toBeNull();
    expect(within(header as HTMLElement).getByRole("button", { name: "Create ACL user" })).toBeInTheDocument();
    expect(document.querySelector(".topbar")).not.toBeNull();
    expect(within(document.querySelector(".topbar") as HTMLElement).queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
  });

  it("requests the first ACL user list from exactly /api/v1/redis/users", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-b".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    const listCalls = fetch.mock.calls.filter((call) => isRedisUsersListUrl(String(call[0])));
    expect(listCalls.length).toBeGreaterThan(0);
    expect(listCalls.every((call) => String(call[0]) === "/api/v1/redis/users")).toBe(true);
    const method = listCalls[0]?.[1]?.method;
    expect(method === undefined || method === "GET").toBe(true);
    expect(new Headers(listCalls[0]?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
  });

  it("loads ACL user details from /api/v1/redis/users/project_a", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-c".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(await screen.findByText("get")).toBeInTheDocument();
    expect(screen.getByText("set")).toBeInTheDocument();
    const detailCalls = fetch.mock.calls.filter((call) => isRedisUserDetailUrl(String(call[0]), "project_a"));
    expect(detailCalls.some((call) => String(call[0]) === "/api/v1/redis/users/project_a")).toBe(true);
  });

  it("shows protected and limited ACL users in the ledger", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-d".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem({
            protected: true,
            rule_fidelity: "limited",
            enabled: false,
            preset: "custom",
            key_pattern: "*",
          }),
        ]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({
          protected: true,
          rule_fidelity: "limited",
          enabled: false,
          preset: "custom",
          key_pattern: "*",
          commands: ["get"],
          categories: ["@read"],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    const row = await screen.findByRole("button", { name: /project_a/ });
    expect(row.querySelector(".ledger-badge")).toHaveTextContent("Protected");
    expect(row).toHaveTextContent("Protected");
    expect(row).toHaveTextContent("Limited");
    expect(row).toHaveTextContent("Disabled");
    expect(row).toHaveTextContent("custom");
    expect(row).toHaveTextContent("*");
    fireEvent.click(row);
    expect(await screen.findByText(/cannot model these rules exactly/)).toBeInTheDocument();
    expect(screen.getByText("get")).toBeInTheDocument();
    expect(screen.getByText("@read")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create ACL user" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });

  it("shows not_configured without an empty ACL user list", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-e".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return jsonResponse(200, {
          state: "not_configured",
          users: [],
          request_id: "66666666666666666666666666666666",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("alert")).toHaveTextContent("Redis is not configured.");
    expect(screen.queryByText("No ACL users.")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
  });

  it.each([
    ["unreachable", "Redis is unreachable."],
    ["auth_failed", "Redis authentication failed."],
    ["permission_denied", "Redis permission denied."],
  ] as const)("shows unavailable %s without an empty ACL user list", async (reason, copy) => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-${reason}`.padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return jsonResponse(200, {
          state: "unavailable",
          reason,
          request_id: "77777777777777777777777777777777",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("alert")).toHaveTextContent(copy);
    expect(screen.queryByText("No ACL users.")).not.toBeInTheDocument();
    expect(screen.queryByText("Redis is not configured.")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /create/i })).not.toBeInTheDocument();
  });

  it("shows No ACL users only for an empty healthy list", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-empty".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByText("No ACL users.")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText("Redis is not configured.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create ACL user" })).toBeInTheDocument();
  });

  it("shows a truncated ACL user list warning", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-trunc".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()], { truncated: true });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("ACL user list truncated.");
  });

  it("hides sibling ACL rows in inspect mode and restores them with Back to users", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-inspect".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /project_b/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(details).toHaveFocus();
    const list = details.closest("article")?.querySelector(".ledger-list");
    expect(list).toHaveClass("ledger-list-inspecting");
    expect(list?.querySelector(".is-selected")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to users" }));
    expect(screen.queryByRole("region", { name: "ACL user details" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /project_b/ })).toBeInTheDocument();
  });

  it("maps ACL user detail unavailable reasons", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-detail-unavail".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return jsonResponse(200, {
          state: "unavailable",
          reason: "auth_failed",
          request_id: "77777777777777777777777777777777",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Redis authentication failed.");
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
  });

  it("shows a session-expired ACL users alert without an empty list", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-401".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByText("No ACL users.")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
  });

  it("shows a not-found alert for a missing ACL user without a healthy inspector", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-404".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
    expect(screen.queryByText("No categories.")).not.toBeInTheDocument();
  });

  it("does not render a canary secret from Redis ACL payloads", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-canary".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk(
          [
            redisAclListItem({
              password: "canary-secret",
              hash: "canary-secret",
              acl_rule: "+@all canary-secret",
            }),
          ],
          { password: "canary-secret" },
        );
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({
          password: "canary-secret",
          hash: "canary-secret",
          acl_rule: "+@all canary-secret",
          commands: ["get", "<img src=x>"],
          categories: [],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("get")).toBeInTheDocument();
    expect(screen.getByText("<img src=x>")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-secret")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-secret");
  });

  it("does not fetch Redis ACL users on the login route", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/redis/users"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
  });

  it("shows honest Redis ACL search states from the ACL users page", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-search".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isSearchUrl(url)) {
        return redisHitSearch();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const dialog = await screen.findByRole("dialog", { name: "Search" });
    const hit = await within(dialog).findByRole("button", { name: /project_a/ });
    expect(hit.className).toContain("nav-result-redis");
    expect(within(dialog).getByRole("status")).toHaveTextContent("1 matching ACL user.");
    expect(within(dialog).queryByText(/Redis ACL user search is not available yet/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/No matching Redis ACL users/i)).not.toBeInTheDocument();
  });

  it("does not persist ACL users and clears the inspector on logout", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-out".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["visible-acl-command"], categories: [] });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("visible-acl-command")).toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByText("visible-acl-command")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "ACL users" })).not.toBeInTheDocument();
    setItem.mockRestore();
  });

  it("ignores a slower first ACL user selection", async () => {
    let releaseA: () => void = () => {};
    const blockedA = new Promise<void>((resolve) => {
      releaseA = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-stale".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem(), redisAclListItem({ username: "project_b", key_pattern: "project_b:*" })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
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
            if (init?.signal?.aborted) {
              reject(new DOMException("The operation was aborted.", "AbortError"));
              return;
            }
            resolve();
          });
        });
        return redisAclDetailOk({ commands: ["stale-command"], categories: [] });
      }
      if (isRedisUserDetailUrl(url, "project_b")) {
        return redisAclDetailOk({
          username: "project_b",
          key_pattern: "project_b:*",
          commands: ["fresh-command"],
          categories: [],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: /project_b/ }));
    expect(await screen.findByText("fresh-command")).toBeInTheDocument();
    releaseA();
    await waitFor(() => {
      expect(screen.queryByText("stale-command")).not.toBeInTheDocument();
    });
    expect(screen.getByText("fresh-command")).toBeInTheDocument();
  });

  it("POSTs a create ACL user body with CSRF and no password", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    expect(within(dialog).getByLabelText("Key prefix")).toHaveValue("project_a:*");
    expect(within(dialog).getByLabelText("Permission preset")).toHaveDisplayValue("Cache read/write");
    expect(within(dialog).getByLabelText("Permission preset")).toHaveValue("cache-read-write");
    expect(within(dialog).queryByLabelText("Queue type")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("option", { name: "Custom" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("group", { name: "Commands" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    });
    const createCall = fetch.mock.calls.find((call) => isRedisUsersCreate(String(call[0]), call[1]));
    expect(createCall?.[0]).toBe("/api/v1/redis/users");
    expect(new Headers(createCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-create".padEnd(64, "0"));
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body).toEqual({ username: "project_a", key_pattern: "project_a:*", preset: "cache-read-write" });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("commands");
    expect(body).not.toHaveProperty("categories");
    expect(body).not.toHaveProperty("enabled");
    expect(body).not.toHaveProperty("queue_kind");
    expect(body).not.toHaveProperty("custom");
    const getCalls = fetch.mock.calls.filter(
      (call) => isRedisUsersListUrl(String(call[0])) && !isRedisUsersCreate(String(call[0]), call[1]),
    );
    expect(getCalls.length).toBeGreaterThan(1);
    expect(getCalls.every((call) => new Headers(call[1]?.headers).get("X-CSRF-Token") === null)).toBe(true);
  });

  it("POSTs read-only without queue_kind and queue-worker with queue_kind", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-preset".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const first = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(first).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.change(within(first).getByLabelText("Permission preset"), { target: { value: "read-only" } });
    expect(within(first).queryByLabelText("Queue type")).not.toBeInTheDocument();
    fireEvent.click(within(first).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    });
    const readOnlyBody = JSON.parse(
      String(fetch.mock.calls.find((call) => isRedisUsersCreate(String(call[0]), call[1]))?.[1]?.body),
    );
    expect(readOnlyBody).toEqual({ username: "project_a", key_pattern: "project_a:*", preset: "read-only" });
    expect(readOnlyBody).not.toHaveProperty("queue_kind");
    expect(readOnlyBody).not.toHaveProperty("password");
    expect(readOnlyBody).not.toHaveProperty("commands");
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const second = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(second).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.change(within(second).getByLabelText("Permission preset"), { target: { value: "queue-worker" } });
    expect(within(second).getByLabelText("Queue type")).toBeInTheDocument();
    expect(within(second).getByLabelText("Queue type")).toHaveDisplayValue("Lists");
    fireEvent.change(within(second).getByLabelText("Queue type"), { target: { value: "streams" } });
    fireEvent.click(within(second).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.filter((call) => isRedisUsersCreate(String(call[0]), call[1])).length).toBe(2);
    });
    const createBodies = fetch.mock.calls
      .filter((call) => isRedisUsersCreate(String(call[0]), call[1]))
      .map((call) => JSON.parse(String(call[1]?.body)));
    expect(createBodies[1]).toEqual({
      username: "project_a",
      key_pattern: "project_a:*",
      preset: "queue-worker",
      queue_kind: "streams",
    });
    expect(createBodies[1]).not.toHaveProperty("password");
    expect(createBodies[1]).not.toHaveProperty("commands");
    expect(createBodies[1]).not.toHaveProperty("custom");
  });

  it("loads Custom from GET /commands without CSRF and disables Create until a command is checked", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create-custom".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201({
          user: {
            username: "project_a",
            enabled: true,
            key_pattern: "project_a:*",
            preset: "custom",
            protected: false,
            rule_fidelity: "exact",
          },
        });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping", "set"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
    expect(within(dialog).queryByLabelText("Queue type")).not.toBeInTheDocument();
    expect(await within(dialog).findByRole("checkbox", { name: "echo" })).not.toBeChecked();
    expect(within(dialog).getByRole("group", { name: "Commands" })).toHaveClass("command-checklist");
    expect(within(dialog).getByRole("checkbox", { name: "get" })).not.toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "ping" })).not.toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "set" })).not.toBeChecked();
    expect(within(dialog).queryByRole("textbox", { name: "Commands" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
    const commandsCall = fetch.mock.calls.find((call) => isRedisCommandsUrl(String(call[0])));
    expect(commandsCall?.[0]).toBe("/api/v1/redis/commands");
    expect(String(commandsCall?.[1]?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(new Headers(commandsCall?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
    expect(fetch.mock.calls.every((call) => !isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
  });

  it("POSTs Custom with CSRF, catalog commands, and no password or queue_kind, then shows the ticket", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create-cmds".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201({
          user: {
            username: "project_a",
            enabled: true,
            key_pattern: "project_a:*",
            preset: "custom",
            protected: false,
            rule_fidelity: "exact",
          },
        });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping", "set"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
    fireEvent.click(await within(dialog).findByRole("checkbox", { name: "echo" }));
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "get" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    });
    const createCall = fetch.mock.calls.find((call) => isRedisUsersCreate(String(call[0]), call[1]));
    expect(createCall?.[0]).toBe("/api/v1/redis/users");
    expect(new Headers(createCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-create-cmds".padEnd(64, "0"));
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body).toEqual({
      username: "project_a",
      key_pattern: "project_a:*",
      preset: "custom",
      commands: ["echo", "get"],
    });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("queue_kind");
    expect(body).not.toHaveProperty("categories");
    expect(body).not.toHaveProperty("enabled");
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(ticket).toHaveTextContent("canary-one-time-password-32chars!!");
    expect(ticket).toHaveTextContent("project_a");
  });

  it("omits commands when switching from Custom back to a named preset", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create-switch".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
    fireEvent.click(await within(dialog).findByRole("checkbox", { name: "get" }));
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "read-only" } });
    expect(within(dialog).queryByRole("group", { name: "Commands" })).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Queue type")).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    });
    const body = JSON.parse(
      String(fetch.mock.calls.find((call) => isRedisUsersCreate(String(call[0]), call[1]))?.[1]?.body),
    );
    expect(body).toEqual({ username: "project_a", key_pattern: "project_a:*", preset: "read-only" });
    expect(body).not.toHaveProperty("commands");
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("queue_kind");
  });

  it.each([
    {
      status: 401,
      body: { error: { code: "unauthorized", message: "Authentication required" } },
      copy: "Your session has expired. Sign in again to continue.",
    },
    {
      status: 503,
      body: { error: { code: "dependency_unavailable", message: "Redis is unavailable." } },
      copy: "Redis is unavailable.",
    },
  ] as const)(
    "disables Create and invents no commands when create GET /commands is $status",
    async ({ status, body, copy }) => {
      const fetch = stubFetch((url, init) => {
        if (url.includes("/api/v1/session")) {
          return jsonResponse(200, {
            owner: { username: "admin" },
            csrf_token: `acl-create-cmds-${status}`.padEnd(64, "0"),
          });
        }
        if (isRedisUsersCreate(url, init)) {
          return redisAclCreate201();
        }
        if (isRedisUsersListUrl(url)) {
          return redisAclListOk([redisAclListItem()]);
        }
        if (isRedisCommandsUrl(url)) {
          return jsonResponse(status, body);
        }
        return unknownApi(url);
      });
      render(<App />);
      expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
      goToAclUsers();
      fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
      const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
      fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
      fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
      expect(await within(dialog).findByRole("alert")).toHaveTextContent(copy);
      expect(within(dialog).getByLabelText("Key prefix")).not.toHaveAttribute("aria-invalid");
      expect(within(dialog).getByRole("button", { name: "Create" })).toBeDisabled();
      expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
      expect(within(dialog).queryByRole("checkbox", { name: "get" })).not.toBeInTheDocument();
      expect(within(dialog).queryByRole("checkbox", { name: "set" })).not.toBeInTheDocument();
      expect(fetch.mock.calls.some((call) => isRedisCommandsUrl(String(call[0])))).toBe(true);
      expect(fetch.mock.calls.every((call) => !isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    },
  );

  it("aborts create GET /commands when leaving Custom", async () => {
    let releaseCommands: () => void = () => {};
    const blockedCommands = new Promise<void>((resolve) => {
      releaseCommands = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create-abort".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisCommandsUrl(url)) {
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
          void blockedCommands.then(() => {
            init?.signal?.removeEventListener("abort", onAbort);
            if (init?.signal?.aborted) {
              reject(new DOMException("The operation was aborted.", "AbortError"));
              return;
            }
            resolve();
          });
        });
        return redisAclCommandsOk(["stale-create-cmd"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
    expect(await within(dialog).findByText("Loading commands.")).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "cache-read-write" } });
    expect(within(dialog).queryByRole("group", { name: "Commands" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    releaseCommands();
    await waitFor(() => {
      expect(within(dialog).queryByText("stale-create-cmd")).not.toBeInTheDocument();
    });
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Create" })).not.toBeDisabled();
  });

  it("shows the one-time ticket password after 201 and ignores extra secret fields", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-ticket".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201({
          credential: {
            username: "project_a",
            password: "canary-one-time-password-32chars!!",
            one_time: true,
            extra_secret: "should-not-render",
            private_key: "-----BEGIN PRIVATE KEY-----",
          },
          extra_secret: "top-level-should-not-render",
        });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(ticket).toHaveTextContent("canary-one-time-password-32chars!!");
    expect(ticket).toHaveTextContent("project_a");
    expect(ticket).toHaveTextContent(/shown now/i);
    expect(within(ticket).getByRole("button", { name: "Copy username" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy password" })).toBeInTheDocument();
    expect(within(ticket).queryByRole("button", { name: "Copy URL" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render");
    expect(document.body.textContent).not.toContain("-----BEGIN PRIVATE KEY-----");
    expect(document.body.textContent).not.toContain("top-level-should-not-render");
  });

  it("shows URL copy only when credential.urls.primary is present", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-url".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201({
          credential: {
            username: "project_a",
            password: "canary-one-time-password-32chars!!",
            one_time: true,
            urls: { primary: "rediss://project_a:canary-one-time-password-32chars!!@127.0.0.1:6380/0" },
          },
        });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(within(ticket).getByRole("button", { name: "Copy URL" })).toBeInTheDocument();
    expect(ticket).toHaveTextContent("rediss://project_a:canary-one-time-password-32chars!!@127.0.0.1:6380/0");
  });

  it("clears the ticket password on dismiss and inspects the new user afterward", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-dismiss".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["get"], categories: [] });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByText("canary-one-time-password-32chars!!")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "ACL user details" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-one-time-password-32chars!!");
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(await screen.findByText("get")).toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("clears the ticket on logout and section change", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-ticket-out".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByText("canary-one-time-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const again = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(again).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(again).getByRole("button", { name: "Create" }));
    expect(await screen.findByText("canary-one-time-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-one-time-password-32chars!!");
  });

  it("clears the ticket when inspecting another ACL user", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-ticket-inspect".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return redisAclCreate201();
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserDetailUrl(url, "project_b")) {
        return redisAclDetailOk({
          username: "project_b",
          key_pattern: "project_b:*",
          commands: ["fresh-command"],
          categories: [],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByText("canary-one-time-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(await screen.findByText("fresh-command")).toBeInTheDocument();
  });

  it.each([
    [409, "ACL user already exists."],
    [403, "You do not have permission to create ACL users."],
    [400, "Username is invalid."],
  ] as const)("shows create %s copy", async (status, message) => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-${status}`.padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return jsonResponse(status, { error: { message } });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(message);
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
  });

  it("shows session expired on create 401 and does not keep a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-create-401".padEnd(64, "0") });
      }
      if (isRedisUsersCreate(url, init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: "Create ACL user" }));
    const dialog = await screen.findByRole("dialog", { name: "Create ACL user" });
    fireEvent.change(within(dialog).getByLabelText("Username"), { target: { value: "project_a" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
  });

  it("never POSTs /api/v1/redis/users from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-session".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisUsersCreate(String(call[0]), call[1]))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/redis/users"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
  });

  it("shows Disable on a non-protected inspector when the ACL list is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-ok".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(details.className).not.toContain("danger");
    expect(within(details).getByRole("button", { name: "Disable" }).className).not.toMatch(/danger/);
  });

  it("shows Enable on a disabled non-protected inspector", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-off".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByRole("button", { name: "Enable" })).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
  });

  it("hides Enable and Disable for a protected ACL user", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-prot".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ protected: true, enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ protected: true, enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByText("Protected")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("hides Enable and Disable while ACL user details are loading", async () => {
    let releaseDetail: () => void = () => {};
    const blockedDetail = new Promise<void>((resolve) => {
      releaseDetail = resolve;
    });
    stubFetch(async (url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-load".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        await blockedDetail;
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(within(details).getByText("Loading details.")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    releaseDetail();
    expect(await within(details).findByRole("button", { name: "Disable" })).toBeInTheDocument();
  });

  it.each(["not_configured", "unavailable"] as const)(
    "hides Enable and Disable when the ACL list is %s",
    async (state) => {
      stubFetch((url) => {
        if (url.includes("/api/v1/session")) {
          return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-toggle-${state}`.padEnd(64, "0") });
        }
        if (isRedisUsersListUrl(url)) {
          return jsonResponse(200, {
            state,
            ...(state === "unavailable" ? { reason: "unreachable" } : { users: [] }),
            request_id: "77777777777777777777777777777777",
          });
        }
        return unknownApi(url);
      });
      render(<App />);
      expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
      goToAclUsers();
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    },
  );

  it("does not show Enable or Disable on the login route", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(
      fetch.mock.calls.every(
        (call) => !isRedisUserEnable(String(call[0]), "project_a", call[1]) && !isRedisUserDisable(String(call[0]), "project_a", call[1]),
      ),
    ).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/enable") && !String(call[0]).includes("/disable"))).toBe(
      true,
    );
  });

  it("POSTs disable with CSRF, an empty body, and no password", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-disable".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        return redisAclToggleOk({ enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Disable" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserDisable(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const disableCall = fetch.mock.calls.find((call) => isRedisUserDisable(String(call[0]), "project_a", call[1]));
    expect(disableCall?.[0]).toBe("/api/v1/redis/users/project_a/disable");
    expect(disableCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}/disable`);
    expect(new Headers(disableCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-disable".padEnd(64, "0"));
    expect(disableCall?.[1]?.body == null || disableCall?.[1]?.body === "").toBe(true);
    expect(String(disableCall?.[1]?.body ?? "")).not.toContain("password");
  });

  it("POSTs enable with CSRF, an empty body, and no password", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-enable".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ enabled: false });
      }
      if (isRedisUserEnable(url, "project_a", init)) {
        return redisAclToggleOk({ enabled: true });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Enable" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserEnable(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const enableCall = fetch.mock.calls.find((call) => isRedisUserEnable(String(call[0]), "project_a", call[1]));
    expect(enableCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}/enable`);
    expect(new Headers(enableCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-enable".padEnd(64, "0"));
    expect(enableCall?.[1]?.body == null || enableCall?.[1]?.body === "").toBe(true);
    expect(String(enableCall?.[1]?.body ?? "")).not.toContain("password");
  });

  it("applies a 200 disable payload to inspector status and the matching ledger row without a ticket", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-200".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        return redisAclToggleOk({ enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Disable" }));
    await waitFor(() => {
      expect(within(details).getByText("Disabled")).toBeInTheDocument();
    });
    expect(within(details).queryByText("Enabled")).not.toBeInTheDocument();
    expect(within(details).getByText("echo")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Enable" })).toBeInTheDocument();
    const row = screen.getByRole("button", { name: /project_a/ });
    expect(row).toHaveTextContent("Disabled");
    expect(row).not.toHaveTextContent("Enabled");
    expect(screen.getByRole("button", { name: /project_b/ })).toHaveTextContent("Enabled");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-one-time-password-32chars!!")).not.toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("shows session-expired copy on enable 401 without a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-401".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ enabled: false });
      }
      if (isRedisUserEnable(url, "project_a", init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Enable" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("shows not-found copy on disable 404", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-404".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Disable" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
  });

  it("shows Redis unavailable copy on disable 503", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-503".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Disable" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Redis is unavailable.");
  });

  it("disables the inspector action while enable or disable is in flight", async () => {
    let releaseDisable: () => void = () => {};
    const blockedDisable = new Promise<void>((resolve) => {
      releaseDisable = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-wait".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        await blockedDisable;
        return redisAclToggleOk({ enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    const action = await within(details).findByRole("button", { name: "Disable" });
    fireEvent.click(action);
    await waitFor(() => {
      expect(action).toBeDisabled();
    });
    releaseDisable();
    expect(await within(details).findByRole("button", { name: "Enable" })).toBeEnabled();
  });

  it("does not persist enable or disable action state across logout or section change", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    let releaseDisable: () => void = () => {};
    const blockedDisable = new Promise<void>((resolve) => {
      releaseDisable = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-toggle-clear".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        await blockedDisable;
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Disable" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    });
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(screen.queryByText("Redis is unavailable.")).not.toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const again = await screen.findByRole("button", { name: "Disable" });
    expect(again).toBeEnabled();
    expect(screen.queryByText("Redis is unavailable.")).not.toBeInTheDocument();
    fireEvent.click(again);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    });
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
    expect(screen.queryByText("Redis is unavailable.")).not.toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    releaseDisable();
    setItem.mockRestore();
  });

  it("never POSTs enable or disable from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-toggle-session".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-toggle".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/enable"))).toBe(true);
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/disable"))).toBe(true);
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
  });

  it("shows Rotate next to Enable or Disable for a non-protected inspector when the ACL list is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-ok".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    const rotate = within(details).getByRole("button", { name: "Rotate" });
    expect(rotate).toBeInTheDocument();
    expect(rotate).toBeEnabled();
    expect(rotate.className).not.toMatch(/danger/);
    expect(rotate.className).toMatch(/text-button/);
  });

  it("hides Rotate for a protected ACL user", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-prot".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ protected: true, enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ protected: true, enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByText("Protected")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("hides Rotate while ACL user details are loading", async () => {
    let releaseDetail: () => void = () => {};
    const blockedDetail = new Promise<void>((resolve) => {
      releaseDetail = resolve;
    });
    stubFetch(async (url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-load".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        await blockedDetail;
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(within(details).getByText("Loading details.")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
    releaseDetail();
    expect(await within(details).findByRole("button", { name: "Rotate" })).toBeInTheDocument();
  });

  it.each(["not_configured", "unavailable"] as const)(
    "hides Rotate when the ACL list is %s",
    async (state) => {
      stubFetch((url) => {
        if (url.includes("/api/v1/session")) {
          return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-rotate-${state}`.padEnd(64, "0") });
        }
        if (isRedisUsersListUrl(url)) {
          return jsonResponse(200, {
            state,
            ...(state === "unavailable" ? { reason: "unreachable" } : { users: [] }),
            request_id: "77777777777777777777777777777777",
          });
        }
        return unknownApi(url);
      });
      render(<App />);
      expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
      goToAclUsers();
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
    },
  );

  it("shows rotate confirm copy and does not POST until Rotate now", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-copy".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    expect(dialog).toHaveTextContent(/issues a new password/i);
    expect(dialog).toHaveTextContent(/previous credential stops working immediately/i);
    expect(dialog).toHaveTextContent(/cannot be recovered/i);
    const rotateNow = within(dialog).getByRole("button", { name: "Rotate now" });
    expect(rotateNow.className).not.toMatch(/danger/);
    expect(rotateNow.className).toMatch(/primary-button/);
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisUserRotate(String(call[0]), "project_a", call[1]))).toBe(true);
  });

  it("POSTs rotate with CSRF, an empty body, and no password", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserRotate(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const rotateCall = fetch.mock.calls.find((call) => isRedisUserRotate(String(call[0]), "project_a", call[1]));
    expect(rotateCall?.[0]).toBe("/api/v1/redis/users/project_a/credentials/rotate");
    expect(rotateCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}/credentials/rotate`);
    expect(new Headers(rotateCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-rotate".padEnd(64, "0"));
    expect(rotateCall?.[1]?.body == null || rotateCall?.[1]?.body === "").toBe(true);
    expect(String(rotateCall?.[1]?.body ?? "")).not.toContain("password");
  });

  it("opens the one-time ticket after rotate 200 and ignores extra secret fields", async () => {
    const writeText = vi.fn();
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-ticket".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200({
          credential: {
            username: "project_a",
            password: "canary-rotated-password-32chars!!",
            one_time: true,
            extra_secret: "should-not-render",
            private_key: "-----BEGIN PRIVATE KEY-----",
          },
          extra_secret: "top-level-should-not-render",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(screen.queryByRole("dialog", { name: "Rotate password?" })).not.toBeInTheDocument();
    expect(ticket).toHaveTextContent("canary-rotated-password-32chars!!");
    expect(ticket).toHaveTextContent("project_a");
    expect(within(ticket).getByRole("button", { name: "Copy username" })).toBeInTheDocument();
    expect(within(ticket).getByRole("button", { name: "Copy password" })).toBeInTheDocument();
    expect(within(ticket).queryByRole("button", { name: "Copy URL" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("should-not-render");
    expect(document.body.textContent).not.toContain("-----BEGIN PRIVATE KEY-----");
    expect(document.body.textContent).not.toContain("top-level-should-not-render");
    expect(writeText).not.toHaveBeenCalled();
    const details = screen.getByRole("region", { name: "ACL user details" });
    expect(within(details).getByRole("button", { name: "Rotate" })).toBeDisabled();
    expect(within(details).queryByText("canary-rotated-password-32chars!!")).not.toBeInTheDocument();
  });

  it("shows URL copy on rotate only when credential.urls.primary is present", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-url".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200({
          credential: {
            username: "project_a",
            password: "canary-rotated-password-32chars!!",
            one_time: true,
            urls: { primary: "rediss://project_a:canary-rotated-password-32chars!!@127.0.0.1:6380/0" },
          },
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    const ticket = await screen.findByRole("alertdialog", { name: /shown now/i });
    expect(within(ticket).getByRole("button", { name: "Copy URL" })).toBeInTheDocument();
    expect(ticket).toHaveTextContent("rediss://project_a:canary-rotated-password-32chars!!@127.0.0.1:6380/0");
  });

  it("clears the rotate ticket password on dismiss and refreshes inspect without storage", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    let detailLoads = 0;
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-dismiss".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        detailLoads += 1;
        return redisAclDetailOk({
          commands: detailLoads === 1 ? ["get"] : ["rotated-command"],
          categories: [],
        });
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByText("get")).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByText("canary-rotated-password-32chars!!")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(screen.queryByText("canary-rotated-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-rotated-password-32chars!!");
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(await screen.findByText("rotated-command")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /project_a/ })).toBeInTheDocument();
    const listGets = fetch.mock.calls.filter(
      (call) => isRedisUsersListUrl(String(call[0])) && !isRedisUsersCreate(String(call[0]), call[1]),
    );
    expect(listGets.length).toBeGreaterThan(1);
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("shows session-expired copy on rotate 401 without a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-401".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-rotated-password-32chars!!")).not.toBeInTheDocument();
  });

  it("shows not-found copy on rotate 404", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-404".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
  });

  it("shows protected copy on rotate 403 without a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-403".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return jsonResponse(403, { error: { code: "protected_resource", message: "This Redis user is protected" } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("This Redis user is protected");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("canary-rotated-password-32chars!!")).not.toBeInTheDocument();
  });

  it("shows Redis unavailable copy on rotate 503", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-503".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate now" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Redis is unavailable.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("disables Rotate while rotate is in flight", async () => {
    let releaseRotate: () => void = () => {};
    const blockedRotate = new Promise<void>((resolve) => {
      releaseRotate = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-rotate-wait".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        await blockedRotate;
        return redisAclRotate200();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Rotate" }));
    const dialog = await screen.findByRole("dialog", { name: "Rotate password?" });
    const rotateNow = within(dialog).getByRole("button", { name: "Rotate now" });
    fireEvent.click(rotateNow);
    await waitFor(() => {
      expect(rotateNow).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Rotate" })).toBeDisabled();
    });
    releaseRotate();
    expect(await screen.findByRole("alertdialog", { name: /shown now/i })).toBeInTheDocument();
  });

  it("never POSTs rotate from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-rotate-session".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-rotate".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/credentials/rotate"))).toBe(true);
    expect(screen.queryByRole("button", { name: "Rotate" })).not.toBeInTheDocument();
  });

  it("shows Edit permissions next to Enable or Disable for a non-protected inspector when the ACL list is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-ok".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Rotate" })).toBeInTheDocument();
    const edit = within(details).getByRole("button", { name: "Edit permissions" });
    expect(edit).toBeEnabled();
    expect(edit.className).not.toMatch(/danger/);
  });

  it("hides Edit permissions for a protected ACL user", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-prot".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ protected: true, enabled: false })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ protected: true, enabled: false });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByText("Protected")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("hides Edit permissions while ACL user details are loading", async () => {
    let releaseDetail: () => void = () => {};
    const blockedDetail = new Promise<void>((resolve) => {
      releaseDetail = resolve;
    });
    stubFetch(async (url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-load".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        await blockedDetail;
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(within(details).getByText("Loading details.")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
    releaseDetail();
    expect(await within(details).findByRole("button", { name: "Edit permissions" })).toBeInTheDocument();
  });

  it.each(["not_configured", "unavailable"] as const)(
    "hides Edit permissions when the ACL list is %s",
    async (state) => {
      stubFetch((url) => {
        if (url.includes("/api/v1/session")) {
          return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-patch-${state}`.padEnd(64, "0") });
        }
        if (isRedisUsersListUrl(url)) {
          return jsonResponse(200, {
            state,
            ...(state === "unavailable" ? { reason: "unreachable" } : { users: [] }),
            request_id: "77777777777777777777777777777777",
          });
        }
        return unknownApi(url);
      });
      render(<App />);
      expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
      goToAclUsers();
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
    },
  );

  it("prefills named presets without Custom, commands, username, or GET /presets", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-form".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ preset: "read-only", key_pattern: "project_a:*" })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ preset: "read-only", key_pattern: "project_a:*" });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(within(dialog).getByText("project_a")).toBeInTheDocument();
    expect(within(dialog).queryByRole("textbox", { name: "Username" })).not.toBeInTheDocument();
    expect(within(dialog).getByLabelText("Key prefix")).toHaveValue("project_a:*");
    expect(within(dialog).getByLabelText("Permission preset")).toHaveDisplayValue("Read only");
    expect(within(dialog).getByLabelText("Permission preset")).toHaveValue("read-only");
    expect(within(dialog).queryByLabelText("Queue type")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("option", { name: "Custom" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("group", { name: "Commands" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
  });

  it("prefills Custom from inspect commands intersected with the catalog and drops unknown names", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-custom".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ preset: "custom", rule_fidelity: "limited" })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({
          preset: "custom",
          rule_fidelity: "limited",
          commands: ["get", "unknown-cmd", "ping"],
        });
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping", "set"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(within(dialog).getByLabelText("Permission preset")).toHaveDisplayValue("Custom");
    expect(within(dialog).getByLabelText("Permission preset")).toHaveValue("custom");
    expect(within(dialog).queryByLabelText("Queue type")).not.toBeInTheDocument();
    expect(await within(dialog).findByRole("checkbox", { name: "get" })).toBeChecked();
    expect(within(dialog).getByRole("group", { name: "Commands" })).toHaveClass("command-checklist");
    expect(within(dialog).getByRole("checkbox", { name: "ping" })).toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "echo" })).not.toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "set" })).not.toBeChecked();
    expect(within(dialog).queryByRole("checkbox", { name: "unknown-cmd" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("textbox", { name: "Commands" })).not.toBeInTheDocument();
    const commandsCall = fetch.mock.calls.find((call) => isRedisCommandsUrl(String(call[0])));
    expect(commandsCall?.[0]).toBe("/api/v1/redis/commands");
    expect(String(commandsCall?.[1]?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(new Headers(commandsCall?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
  });

  it("disables Save when every catalog command is unchecked", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-empty".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ preset: "custom" })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ preset: "custom", commands: ["get", "ping"] });
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.click(await within(dialog).findByRole("checkbox", { name: "get" }));
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "ping" }));
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(fetch.mock.calls.every((call) => String(call[1]?.method ?? "").toUpperCase() !== "PATCH")).toBe(true);
  });

  it("includes queue_kind only for queue-worker and never sends password or commands", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-queue".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem({ preset: "queue-worker", key_pattern: "project_a:*" }),
        ]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return redisAclPatch200({ key_pattern: "project_a:*" });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({
          preset: "queue-worker",
          queue_kind: "streams",
          key_pattern: "project_a:*",
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const first = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(within(first).getByLabelText("Queue type")).toHaveDisplayValue("Streams");
    fireEvent.change(within(first).getByLabelText("Permission preset"), { target: { value: "read-only" } });
    expect(within(first).queryByLabelText("Queue type")).not.toBeInTheDocument();
    fireEvent.click(within(first).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const readOnlyBody = JSON.parse(
      String(fetch.mock.calls.find((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]))?.[1]?.body),
    );
    expect(readOnlyBody).toEqual({ key_pattern: "project_a:*", preset: "read-only" });
    expect(readOnlyBody).not.toHaveProperty("queue_kind");
    expect(readOnlyBody).not.toHaveProperty("username");
    expect(readOnlyBody).not.toHaveProperty("password");
    expect(readOnlyBody).not.toHaveProperty("commands");
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const second = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.change(within(second).getByLabelText("Permission preset"), { target: { value: "queue-worker" } });
    expect(within(second).getByLabelText("Queue type")).toBeInTheDocument();
    fireEvent.change(within(second).getByLabelText("Queue type"), { target: { value: "sorted-sets" } });
    fireEvent.click(within(second).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(fetch.mock.calls.filter((call) => isRedisUserPatch(String(call[0]), "project_a", call[1])).length).toBe(2);
    });
    const patchBodies = fetch.mock.calls
      .filter((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]))
      .map((call) => JSON.parse(String(call[1]?.body)));
    expect(patchBodies[1]).toEqual({
      key_pattern: "project_a:*",
      preset: "queue-worker",
      queue_kind: "sorted-sets",
    });
    expect(patchBodies[1]).not.toHaveProperty("password");
    expect(patchBodies[1]).not.toHaveProperty("commands");
    expect(patchBodies[1]).not.toHaveProperty("username");
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
  });

  it("PATCHes permissions with CSRF and encodeURIComponent and no GET /presets", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-csrf".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return redisAclPatch200();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.change(within(dialog).getByLabelText("Key prefix"), { target: { value: "other:*" } });
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "read-only" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const patchCall = fetch.mock.calls.find((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]));
    expect(patchCall?.[0]).toBe("/api/v1/redis/users/project_a");
    expect(patchCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}`);
    expect(new Headers(patchCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-patch-csrf".padEnd(64, "0"));
    const body = JSON.parse(String(patchCall?.[1]?.body));
    expect(body).toEqual({ key_pattern: "other:*", preset: "read-only" });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("commands");
    expect(body).not.toHaveProperty("categories");
    expect(body).not.toHaveProperty("enabled");
    expect(body).not.toHaveProperty("username");
    expect(body).not.toHaveProperty("queue_kind");
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
  });

  it("applies a 200 PATCH payload to inspector and the matching ledger row without a ticket", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-200".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return redisAclPatch200();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Edit permissions" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Edit permissions" })).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(within(details).getByText("read-only")).toBeInTheDocument();
    });
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(within(details).getByText("other:*")).toBeInTheDocument();
    expect(within(details).getByText("echo")).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("canary-rotated-password-32chars!!");
    expect(document.body.textContent).not.toContain("canary-one-time-password-32chars!!");
    const row = screen.getByRole("button", { name: /project_a/ });
    expect(row).toHaveTextContent("read-only");
    expect(row).toHaveTextContent("other:*");
    expect(screen.getByRole("button", { name: /project_b/ })).toHaveTextContent("cache-read-write");
    expect(screen.getByRole("button", { name: /project_b/ })).toHaveTextContent("project_b:*");
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("shows session-expired copy on PATCH 401 without a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-401".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Edit permissions" })).getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("shows not-found copy on PATCH 404", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-404".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Edit permissions" })).getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
  });

  it("shows protected copy on PATCH 403 without a ticket", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-403".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return jsonResponse(403, { error: { code: "protected_resource", message: "This Redis user is protected" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("This Redis user is protected");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("shows Redis unavailable copy on PATCH 503", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-503".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Redis is unavailable.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("disables Edit permissions while PATCH is in flight", async () => {
    let releasePatch: () => void = () => {};
    const blockedPatch = new Promise<void>((resolve) => {
      releasePatch = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-wait".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        await blockedPatch;
        return redisAclPatch200();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    const save = within(dialog).getByRole("button", { name: "Save" });
    fireEvent.click(save);
    await waitFor(() => {
      expect(save).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Edit permissions" })).toBeDisabled();
    });
    releasePatch();
    expect(await within(details).findByText("read-only")).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Edit permissions" })).toBeEnabled();
  });

  it("disables Edit permissions while enable, rotate, or a ticket is in flight", async () => {
    let releaseDisable: () => void = () => {};
    const blockedDisable = new Promise<void>((resolve) => {
      releaseDisable = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-busy".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDisable(url, "project_a", init)) {
        await blockedDisable;
        return redisAclToggleOk({ enabled: false });
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    const edit = await within(details).findByRole("button", { name: "Edit permissions" });
    fireEvent.click(await within(details).findByRole("button", { name: "Disable" }));
    await waitFor(() => {
      expect(edit).toBeDisabled();
    });
    releaseDisable();
    expect(await within(details).findByRole("button", { name: "Enable" })).toBeEnabled();
    expect(within(details).getByRole("button", { name: "Edit permissions" })).toBeEnabled();
    fireEvent.click(within(details).getByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alertdialog", { name: /shown now/i })).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Edit permissions" })).toBeDisabled();
  });

  it("does not persist PATCH dialog state across logout, section change, or inspect of another user", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-clear".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDetailUrl(url, "project_b")) {
        return redisAclDetailOk({
          username: "project_b",
          key_pattern: "project_b:*",
          commands: ["fresh-command"],
          categories: [],
        });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const first = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.click(within(first).getByRole("button", { name: "Save" }));
    expect(await within(first).findByRole("alert")).toHaveTextContent("Redis is unavailable.");
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(screen.queryByText("Redis is unavailable.")).not.toBeInTheDocument();
    expect(await screen.findByText("fresh-command")).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const again = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(within(again).queryByText("Redis is unavailable.")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    expect(await screen.findByRole("dialog", { name: "Edit permissions" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("never PATCHes Redis ACL users from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-patch-session".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-patch".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(
      fetch.mock.calls.every((call) => String(call[1]?.method ?? "").toUpperCase() !== "PATCH"),
    ).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
    expect(screen.queryByRole("button", { name: "Edit permissions" })).not.toBeInTheDocument();
  });

  it("prefills the named inspect command set when the operator switches to Custom", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-switch".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ commands: ["get", "set"] });
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping", "set"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(within(dialog).getByLabelText("Permission preset")).toHaveValue("cache-read-write");
    expect(fetch.mock.calls.every((call) => !isRedisCommandsUrl(String(call[0])))).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Permission preset"), { target: { value: "custom" } });
    expect(await within(dialog).findByRole("checkbox", { name: "get" })).toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "set" })).toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "echo" })).not.toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "ping" })).not.toBeChecked();
    expect(fetch.mock.calls.some((call) => isRedisCommandsUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
  });

  it("PATCHes Custom with CSRF, catalog commands, and no password or queue_kind", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-patch-cmds".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ preset: "custom" })]);
      }
      if (isRedisUserPatch(url, "project_a", init)) {
        return redisAclPatch200({ preset: "custom", commands: ["echo", "get", "ping"] });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({
          preset: "custom",
          commands: ["get", "unknown-cmd", "ping"],
        });
      }
      if (isRedisCommandsUrl(url)) {
        return redisAclCommandsOk(["echo", "get", "ping", "set"]);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    fireEvent.click(await within(dialog).findByRole("checkbox", { name: "echo" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const patchCall = fetch.mock.calls.find((call) => isRedisUserPatch(String(call[0]), "project_a", call[1]));
    expect(patchCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}`);
    expect(new Headers(patchCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-patch-cmds".padEnd(64, "0"));
    const body = JSON.parse(String(patchCall?.[1]?.body));
    expect(body).toEqual({ key_pattern: "project_a:*", preset: "custom", commands: ["echo", "get", "ping"] });
    expect(body).not.toHaveProperty("password");
    expect(body).not.toHaveProperty("queue_kind");
    expect(body).not.toHaveProperty("username");
    expect(body).not.toHaveProperty("categories");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisPresetsUrl(String(call[0])))).toBe(true);
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it.each([
    {
      status: 401,
      body: { error: { code: "unauthorized", message: "Authentication required" } },
      copy: "Your session has expired. Sign in again to continue.",
    },
    {
      status: 503,
      body: { error: { code: "dependency_unavailable", message: "Redis is unavailable." } },
      copy: "Redis is unavailable.",
    },
  ] as const)("disables Save and invents no commands when GET /commands is $status", async ({ status, body, copy }) => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-cmds-${status}`.padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem({ preset: "custom" })]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk({ preset: "custom", commands: ["get", "set"] });
      }
      if (isRedisCommandsUrl(url)) {
        return jsonResponse(status, body);
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit permissions" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit permissions" });
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(copy);
    expect(within(dialog).getByLabelText("Key prefix")).not.toHaveAttribute("aria-invalid");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox", { name: "get" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox", { name: "set" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.some((call) => isRedisCommandsUrl(String(call[0])))).toBe(true);
    expect(fetch.mock.calls.every((call) => String(call[1]?.method ?? "").toUpperCase() !== "PATCH")).toBe(true);
  });

  it("shows Delete next to inspector actions for a non-protected user when the ACL list is ok", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-ok".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    const header = screen.getByRole("heading", { name: "ACL users" }).closest("header");
    expect(header).not.toBeNull();
    expect(header).toHaveTextContent(/delete a non-protected ACL user/i);
    expect(header).not.toHaveTextContent(/Delete is not available in this slice/);
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(await within(details).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Rotate" })).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Edit permissions" })).toBeInTheDocument();
    const remove = within(details).getByRole("button", { name: "Delete" });
    expect(remove).toBeEnabled();
    expect(remove).toHaveClass("danger-button");
    expect(remove.className).not.toMatch(/redis/);
  });

  it("hides Delete while ACL user details are loading", async () => {
    let releaseDetail: () => void = () => {};
    const blockedDetail = new Promise<void>((resolve) => {
      releaseDetail = resolve;
    });
    stubFetch(async (url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-load".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        await blockedDetail;
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    expect(within(details).getByText("Loading details.")).toBeInTheDocument();
    expect(within(details).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    releaseDetail();
    expect(await within(details).findByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  it.each(["not_configured", "unavailable"] as const)(
    "hides Delete when the ACL list is %s",
    async (state) => {
      stubFetch((url) => {
        if (url.includes("/api/v1/session")) {
          return jsonResponse(200, { owner: { username: "admin" }, csrf_token: `acl-delete-${state}`.padEnd(64, "0") });
        }
        if (isRedisUsersListUrl(url)) {
          return jsonResponse(200, {
            state,
            ...(state === "unavailable" ? { reason: "unreachable" } : { users: [] }),
            request_id: "77777777777777777777777777777777",
          });
        }
        return unknownApi(url);
      });
      render(<App />);
      expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
      goToAclUsers();
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    },
  );

  it("shows delete confirm copy and does not DELETE until fields are valid and Confirm is used", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-copy".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    expect(dialog).toHaveTextContent(/type the exact username and owner password/i);
    expect(dialog).toHaveTextContent(/removes the ACL user/i);
    expect(dialog).toHaveTextContent(/existing Redis connections for that user are terminated/i);
    expect(dialog).toHaveTextContent(/keys are not deleted/i);
    expect(dialog).toHaveTextContent(/cannot be undone/i);
    const confirmUsername = within(dialog).getByLabelText("Confirm username");
    const ownerPassword = within(dialog).getByLabelText("Owner password");
    expect(confirmUsername).toHaveAttribute("autocomplete", "off");
    expect(ownerPassword).toHaveAttribute("type", "password");
    expect(ownerPassword).toHaveAttribute("autocomplete", "current-password");
    const confirm = within(dialog).getByRole("button", { name: "Delete" });
    expect(confirm).toHaveClass("danger-button");
    expect(confirm).toBeDisabled();
    fireEvent.click(confirm);
    fireEvent.change(confirmUsername, { target: { value: "project_b" } });
    fireEvent.change(ownerPassword, { target: { value: "owner-secret-15" } });
    expect(confirm).toBeDisabled();
    fireEvent.change(confirmUsername, { target: { value: "project_a" } });
    fireEvent.change(ownerPassword, { target: { value: "" } });
    expect(confirm).toBeDisabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !isRedisUserDelete(String(call[0]), "project_a", call[1]))).toBe(true);
  });

  it("DELETEs with CSRF, encodeURIComponent, and only username_confirmation plus owner_password", async () => {
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(200, { request_id: "66666666666666666666666666666666" });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => {
      expect(fetch.mock.calls.some((call) => isRedisUserDelete(String(call[0]), "project_a", call[1]))).toBe(true);
    });
    const deleteCall = fetch.mock.calls.find((call) => isRedisUserDelete(String(call[0]), "project_a", call[1]));
    expect(deleteCall?.[0]).toBe("/api/v1/redis/users/project_a");
    expect(deleteCall?.[0]).toBe(`/api/v1/redis/users/${encodeURIComponent("project_a")}`);
    expect(new Headers(deleteCall?.[1]?.headers).get("X-CSRF-Token")).toBe("acl-delete".padEnd(64, "0"));
    const body = JSON.parse(String(deleteCall?.[1]?.body));
    expect(Object.keys(body).sort()).toEqual(["owner_password", "username_confirmation"]);
    expect(body).toEqual({ username_confirmation: "project_a", owner_password: "owner-secret-15" });
  });

  it("clears secrets and inspector selection after delete 200 and refreshes the list without storage", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    let deleted = false;
    const fetch = stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-200".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk(
          deleted
            ? [redisAclListItem({ username: "project_b", key_pattern: "project_b:*" })]
            : [redisAclListItem(), redisAclListItem({ username: "project_b", key_pattern: "project_b:*" })],
        );
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        deleted = true;
        return jsonResponse(200, { request_id: "66666666666666666666666666666666" });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    });
    expect(screen.queryByRole("region", { name: "ACL user details" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Owner password")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    expect(await screen.findByRole("button", { name: /project_b/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /project_a/ })).not.toBeInTheDocument();
    const listGets = fetch.mock.calls.filter(
      (call) => isRedisUsersListUrl(String(call[0])) && !isRedisUsersCreate(String(call[0]), call[1]),
    );
    expect(listGets.length).toBeGreaterThan(1);
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("shows session-expired copy on delete 401 and leaves no leftover password", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-401".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your session has expired. Sign in again to continue.");
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Owner password")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
  });

  it("stays on the delete dialog for reauth_required, announces the error, and clears only the password", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-reauth".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(403, {
          error: { code: "reauth_required", message: "Owner password is incorrect" },
        });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Owner password is incorrect");
    expect(screen.getByRole("dialog", { name: "Delete Redis user" })).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Confirm username")).toHaveValue("project_a");
    expect(within(dialog).getByLabelText("Owner password")).toHaveValue("");
    expect(within(dialog).getByRole("button", { name: "Delete" })).toBeDisabled();
  });

  it("shows protected copy on delete 403 without closing the dialog", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-403".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(403, { error: { code: "protected_resource", message: "This Redis user is protected" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("This Redis user is protected");
    expect(screen.getByRole("dialog", { name: "Delete Redis user" })).toBeInTheDocument();
  });

  it("shows not-found copy on delete 404", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-404".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(404, { error: { code: "not_found", message: "Not found" } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Owner password")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    expect(screen.queryByText("No commands.")).not.toBeInTheDocument();
  });

  it("shows Redis unavailable copy on delete 503", async () => {
    stubFetch((url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-503".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Redis is unavailable." } });
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Redis is unavailable.");
    expect(screen.getByRole("dialog", { name: "Delete Redis user" })).toBeInTheDocument();
  });

  it("traps focus in the delete dialog", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-trap".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    expect(dialog.contains(document.activeElement)).toBe(true);
    fillDeleteDialog(dialog);
    const confirm = within(dialog).getByRole("button", { name: "Delete" });
    confirm.focus();
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toHaveAccessibleName("Confirm username");
    fireEvent.keyDown(dialog, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(confirm);
  });

  it("styles Delete with the danger token, not Redis identity red", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-danger".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    const remove = await within(details).findByRole("button", { name: "Delete" });
    expect(remove).toHaveClass("danger-button");
    expect(remove.className).not.toMatch(/redis/);
    expect(globalsCss).toMatch(/\.danger-button\s*\{[^}]*background:\s*var\(--danger\)/s);
    expect(globalsCss).not.toMatch(/\.danger-button\s*\{[^}]*var\(--redis\)/s);
    fireEvent.click(remove);
    const confirm = within(await screen.findByRole("dialog", { name: "Delete Redis user" })).getByRole("button", {
      name: "Delete",
    });
    expect(confirm).toHaveClass("danger-button");
    expect(confirm.className).not.toMatch(/redis/);
  });

  it("clears delete secrets on dismiss, inspect of another user, and logout", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-clear".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/logout")) {
        return jsonResponse(200, { ok: true });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([
          redisAclListItem(),
          redisAclListItem({ username: "project_b", key_pattern: "project_b:*" }),
        ]);
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      if (isRedisUserDetailUrl(url, "project_b")) {
        return redisAclDetailOk({ username: "project_b", key_pattern: "project_b:*" });
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    fillDeleteDialog(await screen.findByRole("dialog", { name: "Delete Redis user" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Delete Redis user" })).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    fillDeleteDialog(await screen.findByRole("dialog", { name: "Delete Redis user" }));
    fireEvent.click(screen.getByRole("button", { name: /project_b/ }));
    expect(await screen.findByText("project_b:*")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    fillDeleteDialog(await screen.findByRole("dialog", { name: "Delete Redis user" }), "project_b");
    fireEvent.click(screen.getByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("owner-secret-15");
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });

  it("disables Delete while delete is in flight or a credential ticket is open", async () => {
    let releaseDelete: () => void = () => {};
    const blockedDelete = new Promise<void>((resolve) => {
      releaseDelete = resolve;
    });
    stubFetch(async (url, init) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-delete-wait".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isRedisUserDelete(url, "project_a", init)) {
        await blockedDelete;
        return jsonResponse(200, { request_id: "66666666666666666666666666666666" });
      }
      if (isRedisUserRotate(url, "project_a", init)) {
        return redisAclRotate200();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    const details = await screen.findByRole("region", { name: "ACL user details" });
    fireEvent.click(await within(details).findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete Redis user" });
    fillDeleteDialog(dialog);
    const confirm = within(dialog).getByRole("button", { name: "Delete" });
    fireEvent.click(confirm);
    await waitFor(() => {
      expect(confirm).toBeDisabled();
      expect(within(details).getByRole("button", { name: "Delete" })).toBeDisabled();
    });
    releaseDelete();
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Delete Redis user" })).not.toBeInTheDocument();
    });
    fireEvent.click(await screen.findByRole("button", { name: /project_a/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));
    fireEvent.click(within(await screen.findByRole("dialog", { name: "Rotate password?" })).getByRole("button", { name: "Rotate now" }));
    expect(await screen.findByRole("alertdialog", { name: /shown now/i })).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "ACL user details" })).getByRole("button", { name: "Delete" })).toBeDisabled();
  });

  it("never DELETEs Redis ACL users from the login route", async () => {
    let authed = false;
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        if (!authed) {
          return jsonResponse(401, { error: { code: "unauthorized", message: "Authentication required" } });
        }
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-delete-session".padEnd(64, "0") });
      }
      if (url.includes("/api/v1/auth/login")) {
        authed = true;
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-login-delete".padEnd(64, "0") });
      }
      return unknownApi(url);
    });
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "owner-secret-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByRole("button", { name: "admin" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => String(call[1]?.method ?? "").toUpperCase() !== "DELETE")).toBe(true);
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("never DELETEs from search results", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "acl-search-delete".padEnd(64, "0") });
      }
      if (isRedisUsersListUrl(url)) {
        return redisAclListOk([redisAclListItem()]);
      }
      if (isSearchUrl(url)) {
        return redisHitSearch();
      }
      if (isRedisUserDetailUrl(url, "project_a")) {
        return redisAclDetailOk();
      }
      return unknownApi(url);
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    goToAclUsers();
    expect(await screen.findByRole("button", { name: /project_a/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages, databases, and ACL users"), { target: { value: "project" } });
    const dialog = await screen.findByRole("dialog", { name: "Search" });
    fireEvent.click(await within(dialog).findByRole("button", { name: /project_a/ }));
    expect(await screen.findByRole("region", { name: "ACL user details" })).toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => String(call[1]?.method ?? "").toUpperCase() !== "DELETE")).toBe(true);
  });
});
