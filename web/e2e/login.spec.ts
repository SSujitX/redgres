import { expect, test, type Page } from "@playwright/test";

const viewports = [
  { name: "360x800", width: 360, height: 800 },
  { name: "768x1024", width: 768, height: 1024 },
  { name: "1280x800", width: 1280, height: 800 },
  { name: "1600x1000", width: 1600, height: 1000 },
] as const;

async function waitForLogin(page: Page) {
  await page.goto("/");
    await expect(page.getByRole("heading", { name: "Redgres" })).toBeAttached();
    await expect(page.locator(".brand-logo-login .brand-logo-light")).toBeVisible();
  await expect(page.getByLabel("Username")).toBeVisible();
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Log in" })).toBeVisible();
  await expect(page.getByRole("navigation")).toHaveCount(0);
}

test.describe("login viewports", () => {
  for (const vp of viewports) {
    test(`login fits ${vp.name}`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height });
      await waitForLogin(page);
      const overflow = await page.evaluate(() => {
        return document.documentElement.scrollWidth - window.innerWidth;
      });
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }

  test("login fits 1280x800 at 200% zoom", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitForLogin(page);
    await page.evaluate(() => {
      document.documentElement.style.zoom = "2";
    });
    const overflow = await page.evaluate(() => {
      return document.documentElement.scrollWidth - window.innerWidth;
    });
    expect(overflow).toBeLessThanOrEqual(0);
  });
});

test("login does not persist credentials", async ({ page }) => {
  await waitForLogin(page);
  await page.getByLabel("Username").fill("owner");
  await page.getByLabel("Password", { exact: true }).fill("not-a-real-password");
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 10_000 });
  expect(page.url()).not.toMatch(/password=/i);
  const stored = await page.evaluate(() => {
    const keys = [...Object.keys(localStorage), ...Object.keys(sessionStorage)];
    const values = [
      ...Object.values(localStorage),
      ...Object.values(sessionStorage),
    ];
    return { keys, values };
  });
  for (const key of stored.keys) {
    expect(key.toLowerCase()).not.toMatch(/password|session|csrf|token|secret/);
  }
  for (const value of stored.values) {
    expect(value.toLowerCase()).not.toContain("not-a-real-password");
    expect(value.toLowerCase()).not.toMatch(/csrf/);
  }
});
