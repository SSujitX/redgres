import { chromium } from "@playwright/test";
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
await page.route("**/api/v1/session", (r) => r.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: { code: "unauthorized" } }) }));
await page.goto("http://127.0.0.1:8791");
await page.getByRole("heading", { name: "Redgres" }).waitFor();
const d = await page.evaluate(() => {
  const span = document.querySelector(".brand-logo-login");
  const img = document.querySelector(".brand-logo-login img");
  const tag = document.querySelector(".login-tagline");
  const brand = document.querySelector(".login-brand");
  const cs = (el) => { const s = getComputedStyle(el); return { display: s.display, position: s.position, marginBottom: s.marginBottom, gap: s.gap, width: s.width, height: s.height }; };
  return {
    span: cs(span),
    img: { ...img.getBoundingClientRect().toJSON() },
    tag: { top: Math.round(tag.getBoundingClientRect().top) },
    brand: cs(brand),
    brandChildren: [...brand.children].map((c) => ({ cls: c.className, tag: c.tagName, pos: getComputedStyle(c).position, mb: getComputedStyle(c).marginBottom })),
  };
});
console.log(JSON.stringify(d, null, 2));
await browser.close();
