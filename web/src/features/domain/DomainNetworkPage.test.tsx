import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

  it("hides the wizard when status is already configured", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(200, {
          configured: true,
          zone: "example.com",
          hostname: "redgres.example.com",
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
    expect((screen.getByLabelText("Console hostname") as HTMLInputElement).value).toBe("redgres.example.com");

    fireEvent.change(screen.getByLabelText("Console hostname"), {
      target: { value: "console.example.com" },
    });
    fireEvent.change(screen.getByLabelText("Zone"), { target: { value: "other.com" } });
    expect((screen.getByLabelText("Console hostname") as HTMLInputElement).value).toBe("console.example.com");
  });

  it("suggests redgres.<zone> and applies with CSRF", async () => {
    let configured = false;
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/domain" && (!init || !init.method || init.method === "GET")) {
        if (configured) {
          return jsonResponse(200, {
            configured: true,
            zone: "example.com",
            hostname: "redgres.example.com",
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
          hostname: "redgres.example.com",
        });
        configured = true;
        return jsonResponse(200, {
          zone: "example.com",
          hostname: "redgres.example.com",
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
    expect((screen.getByLabelText("Console hostname") as HTMLInputElement).value).toBe("redgres.example.com");
    expect(screen.getByRole("button", { name: "Apply domain" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Apply domain" }));

    expect(await screen.findByRole("heading", { name: "Apply result" })).toBeInTheDocument();
    expect(screen.getByText("tun-1")).toBeInTheDocument();
    expect(screen.getByText(/not a secret token/i)).toBeInTheDocument();
    const applyResult = screen.getByRole("heading", { name: "Apply result" }).closest("section");
    expect(applyResult).not.toBeNull();
    expect(within(applyResult as HTMLElement).getByText(/Deny by default/)).toBeInTheDocument();
    expect(within(applyResult as HTMLElement).getByText(/Still open/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText("Yes")).toBeInTheDocument();
    });
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
            hostname: "redgres.example.com",
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
    expect(within(statusSection as HTMLElement).getAllByText("redgres.example.com").length).toBeGreaterThanOrEqual(1);
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
      target: { value: "redgres.example.com" },
    });
    fireEvent.click(within(dialogAgain).getByRole("button", { name: "Disconnect" }));

    await waitFor(() => {
      expect(screen.getByText("No")).toBeInTheDocument();
    });
    expect(screen.queryByRole("dialog", { name: "Disconnect domain" })).not.toBeInTheDocument();
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
          hostname: "redgres.example.com",
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

    fireEvent.change(screen.getByLabelText("Allowed email"), { target: { value: "owner@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Access allow policy" }));

    expect(await screen.findByText("Allow policy configured")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Console is reachable — close bootstrap" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Allowed email")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Console is reachable — close bootstrap" }));
    const dialog = await screen.findByRole("dialog", { name: "Close bootstrap listener" });
    fireEvent.change(within(dialog).getByLabelText("Confirm hostname"), {
      target: { value: "redgres.example.com" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Close bootstrap" }));
    await waitFor(() => {
      expect(screen.getByText("Closed or not configured")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeInTheDocument();
  });
});
