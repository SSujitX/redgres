import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ExpertToolsSection from "./ExpertToolsSection";

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(),
    json: async () => body,
  };
}

const csrf = "expert-tools-csrf".padEnd(64, "0");
const launchURL = "https://pgadmin.example.com/__redgres/launch?ticket=launch-ticket-32chars!!!!!!!!";

function auditOk(events: unknown[] = []) {
  return jsonResponse(200, { events, request_id: "dddddddddddddddddddddddddddddddd" });
}

function isAuditUrl(url: string): boolean {
  return url === "/api/v1/audit" || url.startsWith("/api/v1/audit?");
}

afterEach(() => {
  vi.useRealTimers();
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ExpertToolsSection", () => {
  it("POSTs launch with CSRF and navigates the pre-opened blank window", async () => {
    const popup = {
      closed: false,
      location: { replace: vi.fn() },
      close: vi.fn(),
    };
    const open = vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
    const fetch = vi.fn(async (url: string, init?: RequestInit) => {
      if (isAuditUrl(url)) {
        expect(url).toBe("/api/v1/audit?limit=8");
        expect(init?.method === undefined || String(init.method).toUpperCase() === "GET").toBe(true);
        expect(new Headers(init?.headers).get("X-CSRF-Token")).toBeNull();
        return auditOk([
          {
            id: 9,
            action: "tools.launch",
            target: "pgadmin",
            outcome: "success",
            created_at: "2026-08-30T11:00:00.000000000Z",
            request_id: "should-not-paint-request-id-32ch",
          },
        ]);
      }
      expect(url).toBe("/api/v1/tools/pgadmin/launch");
      expect(String(init?.method ?? "").toUpperCase()).toBe("POST");
      expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe(csrf);
      expect(init?.body == null || init?.body === "").toBe(true);
      return jsonResponse(200, { launch_url: launchURL, request_id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" });
    });
    vi.stubGlobal("fetch", fetch);

    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open pgAdmin" }));
    expect(open).toHaveBeenCalledWith("about:blank", "_blank");
    await waitFor(() => {
      expect(popup.location.replace).toHaveBeenCalledWith(launchURL);
    });
    expect(popup.close).not.toHaveBeenCalled();
    expect(await screen.findByText("tools.launch")).toBeInTheDocument();
    expect(screen.queryByText("should-not-paint-request-id-32ch")).not.toBeInTheDocument();
  });

  it("reveals pgAdmin email and password with copy controls and clears on dismiss", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    const fetch = vi.fn(async (url: string, init?: RequestInit) => {
      if (isAuditUrl(url)) {
        return auditOk();
      }
      expect(url).toBe("/api/v1/tools/pgadmin/credentials/reveal");
      expect(String(init?.method ?? "").toUpperCase()).toBe("POST");
      expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe(csrf);
      return jsonResponse(200, {
        email: "admin@redgres.com",
        password: "pgadmin-canary-password-32chars!!",
        request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      });
    });
    vi.stubGlobal("fetch", fetch);

    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Reveal pgAdmin login" }));
    expect(await screen.findByText("This pgAdmin password is still saved.")).toBeInTheDocument();
    expect(screen.getByText("admin@redgres.com")).toBeInTheDocument();
    expect(screen.getByText("pgadmin-canary-password-32chars!!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Copy password" }));
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith("pgadmin-canary-password-32chars!!");
    });
    fireEvent.click(screen.getByRole("button", { name: "I have copied it — dismiss" }));
    expect(screen.queryByText("pgadmin-canary-password-32chars!!")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("pgadmin-canary-password-32chars!!");
  });

  it("clears the ticket on unmount and does not persist secrets", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    const fetch = vi.fn(async (url: string) => {
      if (isAuditUrl(url)) {
        return auditOk();
      }
      return jsonResponse(200, {
        email: "admin@redgres.com",
        password: "pgadmin-canary-password-32chars!!",
        request_id: "cccccccccccccccccccccccccccccccc",
      });
    });
    vi.stubGlobal("fetch", fetch);
    const view = render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Reveal pgAdmin login" }));
    expect(await screen.findByText("pgadmin-canary-password-32chars!!")).toBeInTheDocument();
    view.unmount();
    expect(document.body.textContent).not.toContain("pgadmin-canary-password-32chars!!");
    expect(localSet).not.toHaveBeenCalled();
    localSet.mockRestore();
  });

  it("compact Overview controls omit reveal and raw hrefs", () => {
    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{
          pgadmin: "https://pgadmin.example.com",
          redisinsight: "https://redis.example.com",
        }}
        variant="compact"
      />,
    );
    expect(screen.getByRole("button", { name: "Open pgAdmin" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open RedisInsight" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reveal pgAdmin login" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "pgAdmin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Expert tools" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Recent tool activity" })).not.toBeInTheDocument();
  });

  it("filters compact audit to tool actions and never paints secrets or request IDs", async () => {
    const fetch = vi.fn(async (url: string) => {
      expect(isAuditUrl(url)).toBe(true);
      return auditOk([
        {
          id: 1,
          action: "owner.login",
          target: "admin",
          outcome: "success",
          created_at: "2026-08-30T10:00:00.000000000Z",
          request_id: "login-request-id-must-not-show",
        },
        {
          id: 2,
          action: "tools.pgadmin.reveal",
          target: "pgadmin",
          outcome: "success",
          created_at: "2026-08-30T10:01:00.000000000Z",
          request_id: "reveal-request-id-must-not-show",
        },
      ]);
    });
    vi.stubGlobal("fetch", fetch);
    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    expect(await screen.findByText("tools.pgadmin.reveal")).toBeInTheDocument();
    expect(screen.queryByText("owner.login")).not.toBeInTheDocument();
    expect(screen.queryByText("login-request-id-must-not-show")).not.toBeInTheDocument();
    expect(screen.queryByText("reveal-request-id-must-not-show")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("pgadmin-canary-password-32chars!!");
  });

  it("does not claim no tool events ever when the latest audit page has none", async () => {
    const fetch = vi.fn(async () =>
      auditOk([
        {
          id: 1,
          action: "owner.login",
          target: "admin",
          outcome: "success",
          created_at: "2026-08-30T10:00:00.000000000Z",
        },
      ]),
    );
    vi.stubGlobal("fetch", fetch);
    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    expect(await screen.findByText("No tool events in the latest audit page.")).toBeInTheDocument();
    expect(screen.queryByText("No tool launches or reveals yet.")).not.toBeInTheDocument();
  });

  it("does not poll tool activity while the tab is hidden", async () => {
    vi.useFakeTimers();
    const fetch = vi.fn(async (_url: string) => auditOk());
    vi.stubGlobal("fetch", fetch);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    render(
      <ExpertToolsSection
        csrf={csrf}
        toolLinks={{ pgadmin: "https://pgadmin.example.com" }}
        variant="full"
      />,
    );
    await vi.runOnlyPendingTimersAsync();
    const afterMount = fetch.mock.calls.filter((call) => isAuditUrl(String(call[0]))).length;
    expect(afterMount).toBe(1);
    await vi.advanceTimersByTimeAsync(16000);
    expect(fetch.mock.calls.filter((call) => isAuditUrl(String(call[0]))).length).toBe(afterMount);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    vi.useRealTimers();
  });
});
