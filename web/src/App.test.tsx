import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("renders the product name", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ status: "ok" }),
      }),
    );
    render(<App />);
    expect(screen.getByRole("heading", { name: "Redgres" })).toBeInTheDocument();
    expect(await screen.findByText("Control-plane storage is reachable.")).toBeInTheDocument();
  });

  it("renders a healthy state from healthz", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ status: "ok", request_id: "abc" }),
      }),
    );
    render(<App />);
    expect(await screen.findByText("Control-plane storage is reachable.")).toBeInTheDocument();
  });

  it("renders an unavailable state without dumping the error object", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        json: async () => ({
          error: { code: "dependency_unavailable", message: "Control-plane storage is unavailable" },
        }),
      }),
    );
    const { container } = render(<App />);
    expect(await screen.findByText("Control-plane storage is unavailable.")).toBeInTheDocument();
    expect(container.textContent).not.toContain("dependency_unavailable");
    expect(container.textContent).not.toContain("{");
  });
});
