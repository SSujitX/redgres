/**
 * Domain & Network wizard e2e (OPS-009 G7).
 * Mocks /api/v1/domain* routes — no real Cloudflare or backend.
 */
import { CSRF_CANARY, interceptShellAPIs, test, expect, type Page } from "./e2e-fixtures";

const ZONE = "example.com";
const CONSOLE = "console.example.com";
const ORIGIN = "203.0.113.10";

type DomainState = {
  configured: boolean;
  access: string;
  bootstrapOpen: boolean;
  dnsProvider: string;
  credential: string;
  instructions: string[];
};

function defaultState(): DomainState {
  return {
    configured: false,
    access: "deny_by_default",
    bootstrapOpen: true,
    dnsProvider: "cloudflare",
    credential: "api_token",
    instructions: [],
  };
}

async function interceptDomainAPIs(page: Page, state: DomainState) {
  await page.route("**/api/v1/domain**", async (route) => {
    const req = route.request();
    const url = req.url();
    const method = req.method();

    if (url.endsWith("/api/v1/domain") && method === "GET") {
      if (!state.configured) {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ configured: false, request_id: "e2e-domain-1" }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          zone: ZONE,
          hostname: CONSOLE,
          hostnames: { console: CONSOLE, db: `db.${ZONE}`, redis: `redis.${ZONE}` },
          origin_ip: ORIGIN,
          dns_provider: state.dnsProvider,
          access: state.access,
          credential: state.credential,
          bootstrap_still_open: state.bootstrapOpen,
          instructions: state.instructions,
          request_id: "e2e-domain-2",
        }),
      });
    }

    if (url.endsWith("/api/v1/domain/token") && method === "POST") {
      const csrf = req.headers()["x-csrf-token"];
      if (csrf !== CSRF_CANARY) {
        return route.fulfill({ status: 403, body: "{}" });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, request_id: "e2e-token" }),
      });
    }

    if (url.endsWith("/api/v1/domain/apply") && method === "POST") {
      const body = req.postDataJSON() as { dns_provider?: string };
      state.configured = true;
      state.access = "deny_by_default";
      state.bootstrapOpen = true;
      if (body.dns_provider === "manual") {
        state.dnsProvider = "manual";
        state.instructions = [
          `Create a proxied CNAME for ${CONSOLE} pointing to your cloudflared tunnel hostname.`,
          `Create a DNS-only A record for db.${ZONE} with content ${ORIGIN}.`,
        ];
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            zone: ZONE,
            dns_provider: "manual",
            instructions: state.instructions,
            bootstrap_still_open: true,
            request_id: "e2e-manual-apply",
          }),
        });
      }
      state.dnsProvider = "cloudflare";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          zone: ZONE,
          hostname: CONSOLE,
          tunnel_id: "tun-e2e-1",
          bootstrap_still_open: true,
          access: "deny_by_default",
          request_id: "e2e-apply",
        }),
      });
    }

    if (url.endsWith("/api/v1/domain/access-policy") && method === "POST") {
      state.access = "allow";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, access: "allow", bootstrap_still_open: true, request_id: "e2e-access" }),
      });
    }

    if (url.endsWith("/api/v1/domain/manual/confirm-access") && method === "POST") {
      state.access = "allow";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, access: "allow", request_id: "e2e-manual-access" }),
      });
    }

    if (url.endsWith("/api/v1/domain/confirm-reachable") && method === "POST") {
      state.bootstrapOpen = false;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ok: true,
          bootstrap_still_open: false,
          bootstrap_closed: true,
          bootstrap_ufw_removed: true,
          bootstrap_ufw_attempted: true,
          request_id: "e2e-confirm",
        }),
      });
    }

    if (url.endsWith("/api/v1/domain") && method === "DELETE") {
      Object.assign(state, defaultState());
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, request_id: "e2e-disconnect" }),
      });
    }

    return route.fulfill({ status: 404, body: '{"error":{"code":"not_found"}}' });
  });
}

async function openDomainPage(page: Page, state: DomainState) {
  await interceptShellAPIs(page);
  await interceptDomainAPIs(page, state);
  await page.goto("/");
  await expect(page.getByRole("button", { name: "owner" })).toBeVisible({ timeout: 15_000 });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.getByRole("button", { name: "Domain & Network" }).click();
  await expect(page.getByRole("heading", { name: "Domain & Network" })).toBeVisible();
}

test.describe("Domain & Network wizard", () => {
  test("Cloudflare path: token, apply, access allow, close bootstrap", async ({ page }) => {
    const state = defaultState();
    await openDomainPage(page, state);

    await page.getByRole("textbox", { name: "API token" }).fill("e2e-test-token-not-real");
    await page.getByRole("button", { name: "Store token" }).click();
    await expect(page.getByText("Token stored on the server")).toBeVisible();

    await page.getByLabel("Zone").fill(ZONE);
    await page.getByLabel("Origin IP (grey-cloud A or AAAA)").fill(ORIGIN);
    await page.getByRole("button", { name: "Apply domain" }).click();

    await expect(page.getByRole("heading", { name: "Apply result" })).toBeVisible();
    await expect(page.getByText("tun-e2e-1")).toBeVisible();
    await expect(page.getByText("Allow policy configured")).not.toBeVisible();

    await page.getByLabel("Allowed email 1").fill("owner@example.com");
    await page.getByRole("button", { name: "Add Access allow policy" }).click();
    await expect(page.getByText("Allow policy configured")).toBeVisible();

    await page.getByRole("button", { name: "Console is reachable — close bootstrap" }).click();
    const dialog = page.getByRole("dialog", { name: "Close bootstrap listener" });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("Confirm hostname").fill(CONSOLE);
    await dialog.getByRole("button", { name: "Close bootstrap" }).click();

    await expect(page.getByText("Closed or not configured")).toBeVisible();
    await expect(page.getByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeVisible();
  });

  test("manual DNS path: save plan, confirm access, close bootstrap offered", async ({ page }) => {
    const state = defaultState();
    await openDomainPage(page, state);

    await page.getByLabel("Manual DNS (instructions only)").check();
    await expect(page.getByLabel("API token")).not.toBeVisible();

    await page.getByLabel("Zone").fill(ZONE);
    await page.getByLabel("Origin IP (grey-cloud A or AAAA)").fill(ORIGIN);
    await page.getByRole("button", { name: "Save manual plan" }).click();

    await expect(page.getByRole("heading", { name: "3. Manual DNS and Access" })).toBeVisible();
    await expect(page.getByText(`Create a proxied CNAME for ${CONSOLE}`)).toBeVisible();
    await expect(page.getByRole("button", { name: "Console is reachable — close bootstrap" })).not.toBeVisible();

    await page.getByRole("button", { name: "Access configured manually" }).click();
    await expect(page.getByText("Allow policy configured")).toBeVisible();
    await expect(page.getByRole("button", { name: "Console is reachable — close bootstrap" })).toBeVisible();
  });
});
