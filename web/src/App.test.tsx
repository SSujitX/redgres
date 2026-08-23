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
      return jsonResponse(500, {});
    });
    render(<App />);
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Redgres" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Primary" })).not.toBeInTheDocument();
    expect(screen.queryByText(/reachable/i)).not.toBeInTheDocument();
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/healthz"))).toBe(true);
  });

  it("shows the shell when the session is valid", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "a".repeat(64) });
      }
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "admin" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "admin" })).not.toBeInTheDocument();
    expect(fetch.mock.calls.some((call) => String(call[0]).includes("/api/v1/auth/logout"))).toBe(true);
  });

  it("filters navigation locally without calling search", async () => {
    const fetch = stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "d".repeat(64) });
      }
      return jsonResponse(500, {});
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search pages"), { target: { value: "audit" } });
    expect(screen.getByRole("dialog", { name: "Search navigation" }).querySelector(".nav-result")).toHaveTextContent("Audit");
    expect(fetch.mock.calls.every((call) => !String(call[0]).includes("/api/v1/search"))).toBe(true);
  });

  it("hides nested PostgreSQL items until Databases is current", async () => {
    stubFetch((url) => {
      if (url.includes("/api/v1/session")) {
        return jsonResponse(200, { owner: { username: "admin" }, csrf_token: "f".repeat(64) });
      }
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
    });
    render(<App />);
    const search = await screen.findByRole("button", { name: "Search" });
    fireEvent.click(search);
    fireEvent.change(screen.getByLabelText("Search pages"), { target: { value: "audit" } });
    expect(screen.getByRole("status")).toHaveTextContent("1 matching page.");
    fireEvent.click(screen.getByRole("button", { name: "Close search" }));
    await waitFor(() => {
      expect(search).toHaveFocus();
    });
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
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
      return jsonResponse(500, {});
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Open menu" }));
    expect(screen.getByRole("dialog", { name: "Navigation" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close menu" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Navigation" })).not.toBeInTheDocument();
    });
  });
});
