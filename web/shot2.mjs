import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";
mkdirSync("screenshots", { recursive: true });
const browser = await chromium.launch();
async function unauth(page) {
  const body = { error: { code: "unauthorized", message: "Authentication required" } };
  await page.route("**/api/v1/session", (r) => r.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify(body) }));
}
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
await unauth(page);
await page.goto("http://127.0.0.1:8791");
await page.getByRole("heading", { name: "Redgres" }).waitFor();
await page.waitForTimeout(200);
const gap = await page.evaluate(() => {
  const img = document.querySelector(".brand-logo-login img").getBoundingClientRect();
  const tag = document.querySelector(".login-tagline").getBoundingClientRect();
  const band = document.querySelector(".login-engines-bar").getBoundingClientRect();
  return { imgBottom: Math.round(img.bottom), tagTop: Math.round(tag.top), visualGap: Math.round(tag.top - img.bottom), bandHeight: Math.round(band.height) };
});
console.log("GAP:", JSON.stringify(gap));
await page.screenshot({ path: "screenshots/login-desktop-light.png" });
await page.getByRole("button", { name: "Switch to dark theme" }).click();
await page.waitForTimeout(200);
await page.screenshot({ path: "screenshots/login-desktop-dark.png" });
await ctx.close();
const mctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const mp = await mctx.newPage();
await unauth(mp);
await mp.goto("http://127.0.0.1:8791");
await mp.getByRole("heading", { name: "Redgres" }).waitFor();
await mp.waitForTimeout(200);
await mp.screenshot({ path: "screenshots/login-mobile-light.png" });
await mp.getByRole("button", { name: "Switch to dark theme" }).click();
await mp.waitForTimeout(200);
await mp.screenshot({ path: "screenshots/login-mobile-dark.png" });
await mctx.close();
await browser.close();
console.log("done");
