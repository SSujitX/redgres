import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import DomainNetworkPage, { CLOUDFLARE_TOKEN_PERMISSIONS } from "./DomainNetworkPage";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("DomainNetworkPage", () => {
  it("does not show token or apply forms until status says unconfigured", async () => {
    let resolveStatus: ((value: Response) => void) | undefined;
    const pending = new Promise<Response>((resolve) => {
      resolveStatus = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => pending),
    );

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(screen.getByText(/Loading domain status/)).toBeInTheDocument();
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Apply domain" })).not.toBeInTheDocument();

    resolveStatus?.(jsonResponse(200, { configured: false, request_id: "r1" }));
    expect(await screen.findByLabelText("API token")).toBeInTheDocument();
  });

  it("paints secret-safe activity steps and never shows tokens or emails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          activity: {
            operation: "apply",
            in_progress: false,
            steps: [
              { id: "discover_zone", label: "Looking up the zone", state: "done" },
              { id: "create_tunnel", label: "Creating tunnel for db.customer.example.com token=raw", state: "done" },
              { id: "leak", label: "owner@example.com token=secret", state: "done" },
            ],
          },
          request_id: "r1",
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByRole("heading", { name: "Adding the domain" })).toBeInTheDocument();
    expect(screen.getByText("Looking up the zone")).toBeInTheDocument();
    expect(screen.getByText("Creating the tunnel")).toBeInTheDocument();
    expect(screen.getAllByText("Done").length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByText("owner@example.com token=secret")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("db.customer.example.com");
    expect(document.body.textContent).not.toContain("token=raw");
    expect(screen.getByRole("heading", { name: "Remove this domain" })).toBeInTheDocument();
  });

  it("shows a retryable pending disconnect and hides other domain mutations", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          disconnect_pending: true,
          zone: "example.com",
          hostname: "console.example.com",
          access: "allow",
          bootstrap_still_open: true,
          tls: { db: "failed", rs: "failed" },
          request_id: "r1",
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByText(/Disconnect cleanup is pending on the server/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect domain" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Issue TLS certificates" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add Access allow policy" })).not.toBeInTheDocument();
  });

  it("labels prepared DB and legacy Redis certificate state without offering a no-op retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          hostnames: { db: "db.example.com", rs: "rs.example.com" },
          access: "allow",
          dns_provider: "cloudflare",
          tls: { db: "certificate_prepared", redis: "issued" },
          request_id: "r1",
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(
      await screen.findByText((_, element) =>
        element?.textContent === "TLS: Certificate prepared; PostgreSQL TLS not applied"),
    ).toBeInTheDocument();
    expect(
      screen.getByText((_, element) =>
        element?.textContent === "TLS: Certificate prepared; Redis TLS not applied"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Issue TLS certificates" })).not.toBeInTheDocument();
  });

  it("re-enables TLS issue automatically when the cooldown expires", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2030-01-01T00:00:00Z"));
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          access: "allow",
          tls: { db: "failed", rs: "failed" },
          activity: {
            operation: "tls",
            in_progress: false,
            steps: [
              {
                id: "issue_tls",
                state: "failed",
                failure_code: "rate_limited",
                retry_after: "2030-01-01T00:00:01Z",
              },
            ],
          },
          request_id: "r1",
        }),
      ),
    );

    await act(async () => {
      render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    });
    expect(screen.getByRole("button", { name: /retry after/i })).toBeDisabled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_100);
    });
    expect(screen.getByRole("button", { name: "Issue TLS certificates" })).toBeEnabled();
  });

  it("shows an allow-listed TLS failure reason and retry time without raw logs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          access: "allow",
          tls: { db: "failed", rs: "failed" },
          activity: {
            operation: "tls",
            in_progress: false,
            steps: [
              {
                id: "issue_tls",
                label: "Issuing certificates",
                state: "failed",
                failure_code: "rate_limited",
                retry_after: "2099-08-31T10:43:35Z",
                error: "canary raw Certbot error token=secret",
              },
            ],
          },
          request_id: "r1",
        }),
      ),
    );

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByRole("heading", { name: "Issuing certificates" })).toBeInTheDocument();
    expect(screen.getByText(/temporarily rate-limited certificate requests/i)).toBeInTheDocument();
    expect(screen.getAllByText(/31 Aug 2099/i)).toHaveLength(2);
    expect(screen.getByRole("button", { name: /retry after 31 Aug 2099/i })).toBeDisabled();
    expect(document.body.textContent).not.toContain("canary raw Certbot error");
    expect(document.body.textContent).not.toContain("token=secret");
  });

  it("hides the wizard when status is already configured", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          request_id: "r1",
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByText("Yes")).toBeInTheDocument();
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Apply domain" })).not.toBeInTheDocument();
  });

  it("loads unconfigured status and lists token permissions", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo) => {
        expect(String(input)).toBe("/api/v1/domain");
        return jsonResponse(200, { configured: false, request_id: "r1" });
      }),
    );

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByRole("heading", { name: "Domain & Network" })).toBeInTheDocument();
    expect(await screen.findByText("No")).toBeInTheDocument();
    for (const permission of CLOUDFLARE_TOKEN_PERMISSIONS) {
      expect(screen.getByText(permission)).toBeInTheDocument();
    }
  });

  it("stores token with CSRF and clears the input; never persists to storage", async () => {
    const canary = "canary-cloudflare-token-xyz";
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || init.method === undefined || init.method === "GET")) {
        return jsonResponse(200, { configured: false, request_id: "r1" });
      }
      if (url === "/api/v1/domain/token") {
        expect(init?.method).toBe("POST");
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf".padEnd(64, "0"));
        expect(JSON.parse(String(init?.body))).toEqual({ token: canary });
        return jsonResponse(200, { ok: true, request_id: "r2" });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");

    const input = screen.getByLabelText("API token");
    fireEvent.change(input, { target: { value: canary } });
    fireEvent.click(screen.getByRole("button", { name: "Store token" }));

    expect(await screen.findByText(/Token stored on the server/)).toBeInTheDocument();
    expect((input as HTMLInputElement).value).toBe("");
    expect(document.body.textContent).not.toContain(canary);
    for (const store of [window.localStorage, window.sessionStorage]) {
      const n = typeof store.length === "number" ? store.length : 0;
      for (let i = 0; i < n; i += 1) {
        const key = store.key(i) ?? "";
        const value = store.getItem(key) ?? "";
        expect(`${key}=${value}`).not.toContain(canary);
      }
    }
  });

  it("stores the token after a one-time CSRF hash refresh", async () => {
    const canary = "canary-csrf-retry-token";
    const stale = "csrf".padEnd(64, "0");
    const next = "next".padEnd(64, "1");
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        return jsonResponse(200, { configured: false, request_id: "r1" });
      }
      if (url === "/api/v1/domain/token") {
        const header = new Headers(init?.headers).get("X-CSRF-Token");
        if (header === stale) {
          return jsonResponse(403, {
            error: { code: "csrf_invalid", message: "CSRF token is invalid" },
          });
        }
        if (header === next) {
          expect(JSON.parse(String(init?.body))).toEqual({ token: canary });
          return jsonResponse(200, { ok: true, request_id: "r3" });
        }
        throw new Error(`unexpected csrf ${header ?? ""}`);
      }
      if (url === "/api/v1/session") {
        return jsonResponse(200, { csrf_token: next, owner: { username: "admin" } });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={stale} />);
    await screen.findByText("No");
    const input = screen.getByLabelText("API token");
    fireEvent.change(input, { target: { value: canary } });
    fireEvent.click(screen.getByRole("button", { name: "Store token" }));

    expect(await screen.findByText(/Token stored on the server/)).toBeInTheDocument();
    expect((input as HTMLInputElement).value).toBe("");
    expect(document.body.textContent).not.toContain(canary);
  });

  it("clears the token field when store fails", async () => {
    const canary = "canary-fail-token-abc";
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        return jsonResponse(200, { configured: false, request_id: "r1" });
      }
      if (url === "/api/v1/domain/token") {
        return jsonResponse(400, {
          error: { code: "validation_error", message: "Cloudflare token file is not configured" },
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");
    const input = screen.getByLabelText("API token");
    fireEvent.change(input, { target: { value: canary } });
    fireEvent.click(screen.getByRole("button", { name: "Store token" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/could not be stored|not configured/i);
    expect((input as HTMLInputElement).value).toBe("");
    expect(document.body.textContent).not.toContain(canary);
  });

  it("does not clobber a manually edited hostname when the zone changes", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(200, { configured: false, request_id: "r1" })),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");

    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "example.com" } });
    expect((screen.getByPlaceholderText("console.example.com") as HTMLInputElement).value).toBe("console.example.com");

    fireEvent.change(screen.getByPlaceholderText("console.example.com"), {
      target: { value: "app.example.com" },
    });
    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "other.com" } });
    expect((screen.getByPlaceholderText("console.example.com") as HTMLInputElement).value).toBe("app.example.com");
  });

  it("suggests console.<zone> and applies with CSRF", async () => {
    let configured = false;
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        if (configured) {
          return jsonResponse(200, {
            configured: true,
            zone: "example.com",
            hostname: "console.example.com",
            request_id: "r4",
          });
        }
        return jsonResponse(200, { configured: false, request_id: "r1" });
      }
      if (url === "/api/v1/domain/apply") {
        expect(init?.method).toBe("POST");
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf".padEnd(64, "0"));
        expect(JSON.parse(String(init?.body))).toEqual({
          zone: "example.com",
          origin_ip: "203.0.113.10",
          hostnames: {
            console: "console.example.com",
            db: "db.example.com",
            rs: "rs.example.com",
            pgadmin: "pgadmin.example.com",
            redis: "redis.example.com",
          },
        });
        configured = true;
        return jsonResponse(200, {
          zone: "example.com",
          hostname: "console.example.com",
          tunnel_id: "tun-1",
          bootstrap_still_open: true,
          access: "deny_by_default",
          request_id: "r3",
        });
      }
      throw new Error(`unexpected ${url} ${init?.method}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");

    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "example.com" } });
    fireEvent.change(screen.getByPlaceholderText("203.0.113.10"), {
      target: { value: "203.0.113.10" },
    });
    expect((screen.getByPlaceholderText("console.example.com") as HTMLInputElement).value).toBe("console.example.com");
    expect(screen.getByRole("button", { name: "Apply domain" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Apply domain" }));

    expect(await screen.findByRole("heading", { name: "Apply result" })).toBeInTheDocument();
    expect(screen.getAllByText("tun-1").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/not a secret token/i).length).toBeGreaterThanOrEqual(1);
    const applyResult = screen.getByRole("heading", { name: "Apply result" }).closest("section");
    expect(applyResult).not.toBeNull();
    expect(within(applyResult as HTMLElement).getByText(/Deny by default/)).toBeInTheDocument();
    expect(within(applyResult as HTMLElement).getByText(/Still open/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText("Yes")).toBeInTheDocument();
    });
    const statusAfterApply = screen.getByRole("heading", { name: "Status" }).closest("section");
    expect(statusAfterApply).not.toBeNull();
    expect(within(statusAfterApply as HTMLElement).getByText(/belongs on the Ubuntu server/)).toBeInTheDocument();
    expect(within(statusAfterApply as HTMLElement).getByText(/Add an Access allow policy first/)).toBeInTheDocument();
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
  });

  it("shows configured status and disconnects after hostname confirmation", async () => {
    let configured = true;
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        if (configured) {
          return jsonResponse(200, {
            configured: true,
            zone: "example.com",
            hostname: "console.example.com",
            request_id: "r1",
          });
        }
        return jsonResponse(200, { configured: false, request_id: "r4" });
      }
      if (url === "/api/v1/domain" && init?.method === "DELETE") {
        expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf".padEnd(64, "0"));
        configured = false;
        return jsonResponse(200, { ok: true, request_id: "r5" });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByText("Yes")).toBeInTheDocument();
    const statusSection = screen.getByRole("heading", { name: "Status" }).closest("section");
    expect(statusSection).not.toBeNull();
    // Hostname appears in the fact list and again in the Access-allow copy.
    expect(within(statusSection as HTMLElement).getAllByText("console.example.com").length).toBeGreaterThanOrEqual(1);
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Disconnect domain" }));
    const dialog = await screen.findByRole("dialog", { name: "Disconnect domain" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Disconnect domain" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Disconnect domain" })).toHaveFocus();

    fireEvent.click(screen.getByRole("button", { name: "Disconnect domain" }));
    const dialogAgain = await screen.findByRole("dialog", { name: "Disconnect domain" });
    fireEvent.change(within(dialogAgain).getByLabelText("Confirm hostname"), {
      target: { value: "console.example.com" },
    });
    fireEvent.click(within(dialogAgain).getByRole("button", { name: "Disconnect" }));

    await waitFor(() => {
      expect(screen.getByText("No")).toBeInTheDocument();
    });
    expect(screen.queryByRole("dialog", { name: "Disconnect domain" })).not.toBeInTheDocument();
  });

  it("refreshes into pending cleanup state after a failed disconnect", async () => {
    let pending = false;
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init?.method || init.method === "GET")) {
        return jsonResponse(200, {
          configured: true,
          disconnect_pending: pending,
          zone: "example.com",
          hostname: "console.example.com",
          access: "allow",
          bootstrap_still_open: true,
          tls: { db: "failed", rs: "failed" },
          request_id: pending ? "r3" : "r1",
        });
      }
      if (url === "/api/v1/domain" && init?.method === "DELETE") {
        pending = true;
        return jsonResponse(503, { error: { code: "dependency_unavailable", message: "Cleanup is still pending" } });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("Yes");
    fireEvent.click(screen.getByRole("button", { name: "Disconnect domain" }));
    const dialog = await screen.findByRole("dialog", { name: "Disconnect domain" });
    fireEvent.change(within(dialog).getByLabelText("Confirm hostname"), { target: { value: "console.example.com" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Disconnect" }));
    expect(await screen.findByText(/Disconnect cleanup is pending on the server/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Issue TLS certificates" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();
  });

  it("surfaces forbidden without exposing secrets", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(403, {
          error: { code: "forbidden", message: "You do not have permission to manage domain settings." },
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/do not have permission/i);
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
  });

  it("adds Access allow policy then confirms reachable to close bootstrap", async () => {
    let access = "deny_by_default";
    let bootstrapOpen = true;
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        return jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          access,
          bootstrap_still_open: bootstrapOpen,
          request_id: "r1",
        });
      }
      if (url === "/api/v1/domain/access-policy") {
        expect(init?.method).toBe("POST");
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf".padEnd(64, "0"));
        expect(JSON.parse(String(init?.body))).toEqual({ emails: ["owner@example.com"] });
        access = "allow";
        return jsonResponse(200, { ok: true, access: "allow", bootstrap_still_open: true, request_id: "r2" });
      }
      if (url === "/api/v1/domain/confirm-reachable") {
        expect(init?.method).toBe("POST");
        bootstrapOpen = false;
        return jsonResponse(200, {
          ok: true,
          bootstrap_still_open: false,
          bootstrap_closed: true,
          request_id: "r3",
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByRole("button", { name: "Add Access allow policy" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Allowed email 1"), { target: { value: "owner@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Access allow policy" }));

    expect(await screen.findByText("Allow policy configured")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Console is reachable — close bootstrap" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Allowed email 1")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Console is reachable — close bootstrap" }));
    const dialog = await screen.findByRole("dialog", { name: "Close bootstrap listener" });
    fireEvent.change(within(dialog).getByLabelText("Confirm hostname"), {
      target: { value: "console.example.com" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Close bootstrap" }));
    await waitFor(() => {
      expect(screen.getByText("Closed or not configured")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();
  });

  it("keeps Apply result access in sync after allow policy", async () => {
    let configured = false;
    let access = "deny_by_default";
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        if (!configured) {
          return jsonResponse(200, { configured: false, request_id: "r1" });
        }
        return jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          tunnel_id: "tun-1",
          access,
          bootstrap_still_open: true,
          request_id: "r4",
        });
      }
      if (url === "/api/v1/domain/apply") {
        configured = true;
        return jsonResponse(200, {
          zone: "example.com",
          hostname: "console.example.com",
          tunnel_id: "tun-1",
          bootstrap_still_open: true,
          access: "deny_by_default",
          request_id: "r2",
        });
      }
      if (url === "/api/v1/domain/access-policy") {
        access = "allow";
        return jsonResponse(200, { ok: true, access: "allow", bootstrap_still_open: true, request_id: "r3" });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");
    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "example.com" } });
    fireEvent.change(screen.getByPlaceholderText("203.0.113.10"), { target: { value: "203.0.113.10" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply domain" }));
    expect(await screen.findByRole("heading", { name: "Apply result" })).toBeInTheDocument();
    expect(within(screen.getByRole("heading", { name: "Apply result" }).closest("section") as HTMLElement).getByText(/Deny by default/)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Allowed email 1"), { target: { value: "owner@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Access allow policy" }));

    expect((await screen.findAllByText("Allow policy configured")).length).toBeGreaterThanOrEqual(1);
    const applyResult = screen.getByRole("heading", { name: "Apply result" }).closest("section");
    expect(applyResult).not.toBeNull();
    expect(within(applyResult as HTMLElement).getByText("Allow policy configured")).toBeInTheDocument();
    expect(within(applyResult as HTMLElement).queryByText(/Deny by default/)).not.toBeInTheDocument();
    expect(within(applyResult as HTMLElement).getByText("tun-1")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(screen.queryByText(/Deny by default/)).not.toBeInTheDocument();
    expect(screen.getAllByText("Allow policy configured").length).toBeGreaterThanOrEqual(1);
  });

  it("completes manual DNS wizard through confirm-access to close bootstrap", async () => {
    let configured = false;
    let access: string | undefined;
    const manualInstructions = [
      "Create a proxied CNAME for console.example.com pointing to your cloudflared tunnel hostname.",
      "Create a DNS-only A record for db.example.com with content 203.0.113.10.",
      "Configure Cloudflare Access on console.example.com, pgadmin.example.com, and redis.example.com (deny by default).",
    ];
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        if (!configured) {
          return jsonResponse(200, { configured: false, request_id: "r1" });
        }
        return jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          dns_provider: "manual",
          access,
          bootstrap_still_open: true,
          instructions: manualInstructions,
          request_id: "r2",
        });
      }
      if (url === "/api/v1/domain/apply") {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          zone: "example.com",
          origin_ip: "203.0.113.10",
          dns_provider: "manual",
          hostnames: {
            console: "console.example.com",
            db: "db.example.com",
            rs: "rs.example.com",
            pgadmin: "pgadmin.example.com",
            redis: "redis.example.com",
          },
        });
        configured = true;
        return jsonResponse(200, {
          zone: "example.com",
          dns_provider: "manual",
          instructions: manualInstructions,
          bootstrap_still_open: true,
          request_id: "r3",
        });
      }
      if (url === "/api/v1/domain/manual/confirm-access") {
        expect(init?.method).toBe("POST");
        access = "allow";
        return jsonResponse(200, { ok: true, access: "allow", request_id: "r4" });
      }
      throw new Error(`unexpected ${url} ${init?.method}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    await screen.findByText("No");

    fireEvent.click(screen.getByLabelText("Manual DNS (instructions only)"));
    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "example.com" } });
    fireEvent.change(screen.getByPlaceholderText("203.0.113.10"), {
      target: { value: "203.0.113.10" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save manual plan" }));

    expect(await screen.findByRole("heading", { name: "3. Manual DNS and Access" })).toBeInTheDocument();
    expect(screen.getByText(manualInstructions[0])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Access configured manually" }));

    expect((await screen.findAllByText("Allow policy configured")).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole("button", { name: "Console is reachable — close bootstrap" })).toBeInTheDocument();
  });

  it("does not ask for OAuth after an API-token apply", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "console.example.com",
          credential: "api_token",
          access: "allow",
          bootstrap_still_open: true,
          tls: { db: "not_issued", rs: "not_issued" },
          request_id: "r1",
        }),
      ),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    const statusSection = await screen.findByRole("heading", { name: "Status" });
    expect(within(statusSection.closest("section") as HTMLElement).getByText("API token")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Console is reachable — close bootstrap" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Connect Cloudflare OAuth/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("OAuth client ID")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("OAuth client secret")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect Cloudflare" })).not.toBeInTheDocument();
  });

  it("lets the owner choose API token once and hides OAuth fields", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(200, { configured: false, request_id: "r1" })),
    );
    render(<DomainNetworkPage csrf={"csrf".padEnd(64, "0")} />);
    expect(await screen.findByLabelText("API token")).toBeInTheDocument();
    expect(screen.getByLabelText(/API token — paste once/i)).toBeChecked();
    expect(screen.queryByLabelText("OAuth client ID")).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(/OAuth — after the console hostname is live/i));
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
    expect(screen.getAllByText(/cannot add the domain from this bootstrap URL/i).length).toBeGreaterThanOrEqual(1);
    fireEvent.click(screen.getByLabelText(/API token — paste once/i));
    expect(screen.getByLabelText("API token")).toBeInTheDocument();
  });
});
