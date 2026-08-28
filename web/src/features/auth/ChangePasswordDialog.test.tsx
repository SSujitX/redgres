import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ChangePasswordDialog from "./ChangePasswordDialog";

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

const csrf = "c".repeat(64);

function fill(current: string, next: string, confirm: string) {
  fireEvent.change(screen.getByLabelText("Current password"), { target: { value: current } });
  fireEvent.change(screen.getByLabelText("New password"), { target: { value: next } });
  fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: confirm } });
}

describe("ChangePasswordDialog", () => {
  it("submits with CSRF and reports success", async () => {
    const onSuccess = vi.fn();
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      expect(String(input)).toBe("/api/v1/auth/password");
      expect(init?.method).toBe("POST");
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe(csrf);
      expect(JSON.parse(String(init?.body))).toEqual({
        current_password: "old-secret-password",
        new_password: "new-secret-password",
      });
      return jsonResponse(200, { ok: true, request_id: "r1" });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<ChangePasswordDialog csrf={csrf} onClose={() => {}} onSuccess={onSuccess} />);
    fill("old-secret-password", "new-secret-password", "new-secret-password");
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));
  });

  it("shows current-password error on reauth_required", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(403, { error: { code: "reauth_required", message: "Current password is incorrect" }, request_id: "r1" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<ChangePasswordDialog csrf={csrf} onClose={() => {}} onSuccess={() => {}} />);
    fill("wrong", "new-secret-password", "new-secret-password");
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    expect(await screen.findByText("Current password is incorrect.")).toBeInTheDocument();
  });

  it("shows weak-new error on validation failure", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(422, { error: { code: "validation_error", fields: { new_password: "too_weak" } }, request_id: "r1" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<ChangePasswordDialog csrf={csrf} onClose={() => {}} onSuccess={() => {}} />);
    fill("old-secret-password", "short", "short");
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    expect(await screen.findByText(/too weak/)).toBeInTheDocument();
  });

  it("rejects mismatched confirmation without calling the API", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    render(<ChangePasswordDialog csrf={csrf} onClose={() => {}} onSuccess={() => {}} />);
    fill("old-secret-password", "new-secret-password", "different-secret");
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    expect(await screen.findByText("New passwords do not match.")).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
