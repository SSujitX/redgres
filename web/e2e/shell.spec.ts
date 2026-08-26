/**
 * Authenticated shell viewport and responsive behavior tests.
 *
 * These tests verify the shell chrome (sidebar, icon rail, mobile drawer,
 * topbar, search, owner menu) at all documented viewports from
 * UI_DESIGN_SYSTEM.md §4/§8 without requiring a real Go backend.
 */
import { CSRF_CANARY, test, expect } from "./e2e-fixtures";

// ---------------------------------------------------------------------------
// Documented review viewports (UI_DESIGN_SYSTEM.md §8)
// ---------------------------------------------------------------------------

const viewports = [
  { name: "320x800", width: 320, height: 800 },
  { name: "360x800", width: 360, height: 800 },
  { name: "768x1024", width: 768, height: 1024 },
  { name: "1280x800", width: 1280, height: 800 },
  { name: "1600x1000", width: 1600, height: 1000 },
] as const;

// ---------------------------------------------------------------------------
// No horizontal overflow at any documented viewport
// ---------------------------------------------------------------------------

test.describe("authenticated shell viewports", () => {
  for (const vp of viewports) {
    test(`shell fits ${vp.name} without horizontal overflow`, async ({ shellPage }) => {
      await shellPage.setViewportSize({ width: vp.width, height: vp.height });
      const overflow = await shellPage.evaluate(() => {
        return document.documentElement.scrollWidth - window.innerWidth;
      });
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }

  test("shell fits a 640x400 CSS viewport proxy without horizontal overflow", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 640, height: 400 });
    const overflow = await shellPage.evaluate(() => {
      return document.documentElement.scrollWidth - window.innerWidth;
    });
    expect(overflow).toBeLessThanOrEqual(0);
  });
});

// ---------------------------------------------------------------------------
// Responsive navigation modes
// ---------------------------------------------------------------------------

test.describe("responsive navigation", () => {
  test("mobile (360x800): sidebar hidden, drawer opens from hamburger menu", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 360, height: 800 });

    // Sidebar should be hidden at mobile widths
    const sidebar = shellPage.locator(".app-sidebar");
    await expect(sidebar).not.toBeVisible();

    // Hamburger menu button should be visible
    const menuButton = shellPage.getByRole("button", { name: "Open menu" });
    await expect(menuButton).toBeVisible();

    // Open the drawer
    await menuButton.click();

    // Drawer dialog should appear with navigation items
    const drawer = shellPage.getByRole("dialog", { name: "Navigation" });
    await expect(drawer).toBeVisible();

    // Navigation inside drawer should have items
    const nav = drawer.getByRole("navigation", { name: "Primary" });
    await expect(nav).toBeVisible();
    const firstItem = nav.getByRole("button", { name: "Overview" });
    await expect(firstItem).toBeVisible();
    await expect(firstItem).toBeFocused();
    await expect(nav.getByRole("button", { name: "Databases" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "ACL users" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "Audit" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "System" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "Documentation" })).toBeVisible();

    // Close menu button should be present
    const closeButton = drawer.getByRole("button", { name: "Close menu" });
    await expect(closeButton).toBeVisible();

    await firstItem.press("Shift+Tab");
    await expect(closeButton).toBeFocused();
    await closeButton.press("Tab");
    await expect(firstItem).toBeFocused();

    // Close the drawer
    await closeButton.click();
    await expect(drawer).not.toBeVisible();
    await expect(menuButton).toBeFocused();
  });

  test("mobile (360x800): drawer closes on Escape", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 360, height: 800 });

    const menuButton = shellPage.getByRole("button", { name: "Open menu" });
    await menuButton.click();

    const drawer = shellPage.getByRole("dialog", { name: "Navigation" });
    await expect(drawer).toBeVisible();

    // Press Escape to close
    await shellPage.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
    await expect(menuButton).toBeFocused();
  });

  test("tablet (768x1024): icon rail visible, no full sidebar labels", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 768, height: 1024 });

    // Sidebar element should be visible (as icon rail)
    const sidebar = shellPage.locator(".app-sidebar");
    await expect(sidebar).toBeVisible();

    // The sidebar should have the narrow rail width, not the full sidebar
    const width = await sidebar.evaluate((el) => el.getBoundingClientRect().width);
    // Rail width is 72px (var(--rail-width))
    expect(width).toBeLessThanOrEqual(80);
    expect(width).toBeGreaterThanOrEqual(60);

    // Brand name should be visually hidden (sr-only) at icon rail width
    const brandName = shellPage.locator(".brand-name");
    const brandClip = await brandName.evaluate((el) => getComputedStyle(el).clip);
    expect(brandClip).toBe("rect(0px, 0px, 0px, 0px)");

    // Navigation items should exist and be accessible
    const nav = shellPage.getByRole("navigation", { name: "Primary" });
    await expect(nav).toBeVisible();

    // Hamburger menu should be hidden (CSS display: none for drawer-toggle at >= 768px)
    const menuButton = shellPage.locator(".drawer-toggle");
    await expect(menuButton).not.toBeVisible();
  });

  test("desktop (1280x800): full sidebar with labels visible", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    // Sidebar should be visible
    const sidebar = shellPage.locator(".app-sidebar");
    await expect(sidebar).toBeVisible();

    // The sidebar should have the desktop width (248px)
    const width = await sidebar.evaluate((el) => el.getBoundingClientRect().width);
    expect(width).toBeGreaterThanOrEqual(240);
    expect(width).toBeLessThanOrEqual(280);

    // Navigation labels should be visible (not sr-only)
    const nav = shellPage.getByRole("navigation", { name: "Primary" });
    await expect(nav).toBeVisible();

    // Nav item labels should be visible at desktop
    await expect(nav.getByRole("button", { name: "Overview" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "Databases" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "ACL users" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "Audit" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "System" })).toBeVisible();
    await expect(nav.getByRole("button", { name: "Documentation" })).toBeVisible();

    // Brand name should be visible
    const brandName = shellPage.locator(".brand-name");
    await expect(brandName).toBeVisible();

    // Hamburger menu should be hidden
    const menuButton = shellPage.locator(".drawer-toggle");
    await expect(menuButton).not.toBeVisible();
  });

  test("wide desktop (1600x1000): wider sidebar at >= 1440px", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1600, height: 1000 });

    const sidebar = shellPage.locator(".app-sidebar");
    await expect(sidebar).toBeVisible();

    // At >= 1440px, sidebar width is var(--sidebar-wide) = 264px
    const width = await sidebar.evaluate((el) => el.getBoundingClientRect().width);
    expect(width).toBeGreaterThanOrEqual(256);
    expect(width).toBeLessThanOrEqual(280);
  });
});

// ---------------------------------------------------------------------------
// Topbar and owner menu
// ---------------------------------------------------------------------------

test.describe("topbar", () => {
  test("topbar uses sticky positioning and contains context, search, and owner", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    const topbar = shellPage.locator(".topbar");
    await expect(topbar).toBeVisible();
    await expect(topbar).toHaveCSS("position", "sticky");
    await expect(topbar).toHaveCSS("top", "0px");

    // Context shows current section
    const context = shellPage.locator(".topbar-context");
    await expect(context).toBeVisible();
    await expect(context).toHaveText("Overview");

    // Search button
    const searchButton = shellPage.getByRole("button", { name: "Search" });
    await expect(searchButton).toBeVisible();

    // Owner button
    const ownerButton = shellPage.getByRole("button", { name: "owner" });
    await expect(ownerButton).toBeVisible();
  });

  test("owner menu opens and shows Log out", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    const ownerButton = shellPage.getByRole("button", { name: "owner" });
    await ownerButton.click();

    // Owner dropdown menu should appear
    const menu = shellPage.getByRole("menu");
    await expect(menu).toBeVisible();

    // Log out menu item
    const logoutItem = menu.getByRole("menuitem", { name: "Log out" });
    await expect(logoutItem).toBeVisible();

    // Close by pressing Escape
    await shellPage.keyboard.press("Escape");
    await expect(menu).not.toBeVisible();
  });

  test("owner button visible at mobile (360x800)", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 360, height: 800 });

    const ownerButton = shellPage.getByRole("button", { name: "owner" });
    await expect(ownerButton).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Global search
// ---------------------------------------------------------------------------

test.describe("global search", () => {
  test("search opens with Ctrl+K and closes with Escape", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    // Open search with Ctrl+K
    await shellPage.keyboard.press("Control+k");

    // Search dialog should appear
    const searchInput = shellPage.locator(".search-input");
    await expect(searchInput).toBeVisible({ timeout: 5_000 });
    await expect(searchInput).toBeFocused();
    await expect(shellPage.getByRole("status")).toBeAttached();

    // Close with Escape
    await shellPage.keyboard.press("Escape");
    await expect(searchInput).not.toBeVisible();
    await expect(shellPage.getByRole("button", { name: "Search" })).toBeFocused();
  });

  test("search opens with / key", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    await shellPage.keyboard.press("/");

    const searchInput = shellPage.locator(".search-input");
    await expect(searchInput).toBeVisible({ timeout: 5_000 });
    await expect(searchInput).toBeFocused();

    await searchInput.fill("docs");
    await shellPage.keyboard.press("/");
    await expect(searchInput).toHaveValue("docs/");

    await shellPage.keyboard.press("Escape");
    await expect(searchInput).not.toBeVisible();
  });

  test("search opens from search button", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    const searchButton = shellPage.getByRole("button", { name: "Search" });
    await searchButton.click();

    const searchInput = shellPage.locator(".search-input");
    await expect(searchInput).toBeVisible({ timeout: 5_000 });

    await shellPage.keyboard.press("Escape");
  });

  test("mobile search opens full-screen", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 360, height: 800 });

    const searchButton = shellPage.getByRole("button", { name: "Search" });
    await searchButton.click();

    const searchDialog = shellPage.locator(".search-dialog");
    await expect(searchDialog).toBeVisible({ timeout: 5_000 });

    const bounds = await searchDialog.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds?.x).toBe(0);
    expect(bounds?.y).toBe(0);
    expect(bounds?.width).toBe(360);
    expect(bounds?.height).toBe(800);
    await expect(searchDialog).toHaveCSS("border-radius", "0px");

    await shellPage.keyboard.press("Escape");
  });
});

// ---------------------------------------------------------------------------
// Page navigation via sidebar
// ---------------------------------------------------------------------------

test.describe("page navigation", () => {
  test("clicking nav items changes the page context", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 1280, height: 800 });

    const nav = shellPage.getByRole("navigation", { name: "Primary" });
    const context = shellPage.locator(".topbar-context");

    // Start on Overview
    await expect(context).toHaveText("Overview");

    // Navigate to Databases
    await nav.getByRole("button", { name: "Databases" }).click();
    await expect(context).toHaveText("PostgreSQL · Databases");

    // Navigate to ACL users
    await nav.getByRole("button", { name: "ACL users" }).click();
    await expect(context).toHaveText("Redis ACL · ACL users");

    // Navigate to Audit
    await nav.getByRole("button", { name: "Audit" }).click();
    await expect(context).toHaveText("Audit");

    // Navigate to System
    await nav.getByRole("button", { name: "System" }).click();
    await expect(context).toHaveText("System");

    // Navigate to Documentation
    await nav.getByRole("button", { name: "Documentation" }).click();
    await expect(context).toHaveText("Documentation");

    // Navigate back to Overview
    await nav.getByRole("button", { name: "Overview" }).click();
    await expect(context).toHaveText("Overview");
  });

  test("mobile drawer navigation changes page and closes drawer", async ({ shellPage }) => {
    await shellPage.setViewportSize({ width: 360, height: 800 });

    // Open drawer
    await shellPage.getByRole("button", { name: "Open menu" }).click();
    const drawer = shellPage.getByRole("dialog", { name: "Navigation" });
    await expect(drawer).toBeVisible();

    // Navigate to Databases via drawer
    await drawer.getByRole("button", { name: "Databases" }).click();

    // Drawer should close after navigation
    await expect(drawer).not.toBeVisible();

    // Context should update
    const context = shellPage.locator(".topbar-context");
    await expect(context).toHaveText("PostgreSQL · Databases");
  });
});

// ---------------------------------------------------------------------------
// Secret persistence guard
// ---------------------------------------------------------------------------

test("shell does not persist its session CSRF value in browser storage", async ({ shellPage }) => {
  await shellPage.setViewportSize({ width: 1280, height: 800 });

  // Navigate through several pages to exercise the app
  const nav = shellPage.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("button", { name: "Databases" }).click();
  await nav.getByRole("button", { name: "ACL users" }).click();
  await nav.getByRole("button", { name: "Overview" }).click();

  // The session response supplies this exact canary to page JavaScript.
  const stored = await shellPage.evaluate(() => {
    const keys = [...Object.keys(localStorage), ...Object.keys(sessionStorage)];
    const values = [...Object.values(localStorage), ...Object.values(sessionStorage)];
    return { keys, values };
  });
  for (const key of stored.keys) {
    expect(key.toLowerCase()).not.toMatch(/password|session|csrf|token|secret/);
  }
  for (const value of stored.values) {
    expect(value).not.toContain(CSRF_CANARY);
  }
});
