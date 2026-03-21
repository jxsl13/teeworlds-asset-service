import { test, expect } from "@playwright/test";

/**
 * Non-admin E2E tests — run against the live asset-service with
 * real PostgreSQL data seeded by global-setup.ts.
 *
 * These tests verify the public (anonymous) UI on both mobile and
 * desktop viewports.
 */
test.describe("Non-admin UI – responsive layout", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/skin");
    // Wait for HTMX to load the items fragment.
    await page.waitForSelector(".items-table", { state: "visible", timeout: 15_000 });
  });

  test("loads the page with header, tabs, search, and items table", async ({ page }) => {
    await expect(page.locator("h1")).toContainText("Teeworlds Asset Database");
    const tabs = page.locator(".tab");
    expect(await tabs.count()).toBeGreaterThanOrEqual(5);
    await expect(page.locator("#search")).toBeVisible();
    await expect(page.locator(".items-table")).toBeVisible();
  });

  test("switching tabs loads content for the selected asset type", async ({ page }) => {
    const mapTab = page.locator('.tab[data-type="map"]');
    await mapTab.click();
    // Wait for HTMX swap to complete by checking the tab becomes active.
    await expect(mapTab).toHaveClass(/active/, { timeout: 5000 });
    await expect(page.locator("#content")).toBeVisible();
    await expect(page.locator('.tab[data-type="skin"]')).not.toHaveClass(/active/);
  });

  test("clicking a column header triggers sort (desktop only)", async ({
    page,
    isMobile,
  }) => {
    test.skip(!!isMobile, "Sort headers are hidden on mobile (thead is display:none)");
    const nameHeader = page.locator("th.col-name");
    await expect(nameHeader).toBeVisible();
    await nameHeader.click();
    await expect(nameHeader.locator(".sort-indicator")).toBeVisible({ timeout: 3000 });
  });

  test("pagination controls are visible when items exist", async ({ page }) => {
    const pagination = page.locator(".pagination");
    await expect(pagination).toBeVisible();
    await expect(pagination.locator(".page-info")).toBeVisible();
  });

  test("clicking a thumbnail opens the preview modal", async ({ page }) => {
    const thumb = page.locator(".col-thumb img").first();
    if ((await thumb.count()) > 0) {
      await thumb.click();
      await expect(page.locator("#previewModal")).toHaveClass(/open/, { timeout: 5000 });
      await page.keyboard.press("Escape");
      await expect(page.locator("#previewModal")).not.toHaveClass(/open/);
    }
  });

  test("search input is functional", async ({ page }) => {
    const searchInput = page.locator("#search");
    await expect(searchInput).toBeVisible();
    await searchInput.fill("E2E");
    // Wait for HTMX search response (triggers on input changed delay:100ms).
    await page.waitForResponse((r) => r.url().includes("/skin") && r.status() === 200, { timeout: 5000 }).catch(() => {});
    await expect(page.locator("#content")).toBeVisible();
  });

  test("table is contained within the viewport", async ({ page }) => {
    const overflow = await page.evaluate(() => {
      const table = document.querySelector(".items-table");
      if (!table) return { ok: true };
      const cs = window.getComputedStyle(table);
      return {
        ok: parseFloat(cs.width) <= document.documentElement.clientWidth + 2,
        tableWidth: parseFloat(cs.width),
        viewWidth: document.documentElement.clientWidth,
      };
    });
    expect(overflow.ok).toBe(true);
  });

  test("admin column and buttons are not visible for non-admin", async ({ page }) => {
    expect(await page.locator(".btn-admin").count()).toBe(0);
    expect(await page.locator(".btn-delete").count()).toBe(0);
    expect(await page.locator(".btn-edit").count()).toBe(0);
    expect(await page.locator(".btn-info").count()).toBe(0);
  });

  test("footer shows login link for non-admin", async ({ page }) => {
    const footer = page.locator(".site-footer");
    await expect(footer).toBeVisible();
    await expect(footer.locator('a[href*="/auth/login"]')).toBeVisible();
  });

  test("selecting a row shows the selection bar", async ({ page }) => {
    const checkbox = page.locator(".row-select").first();
    if ((await checkbox.count()) > 0) {
      await page.evaluate(() => {
        const w = window as Window & { toggleRowSelect?: (cb: HTMLInputElement) => void; clearSelection?: () => void };
        const cb = document.querySelector(".row-select") as HTMLInputElement;
        if (cb && w.toggleRowSelect) { cb.checked = true; w.toggleRowSelect(cb); }
      });
      await expect(page.locator("#selectionBar")).toBeVisible({ timeout: 3000 });
      await page.evaluate(() => {
        const w = window as Window & { clearSelection?: () => void };
        if (w.clearSelection) w.clearSelection();
      });
      await expect(page.locator("#selectionBar")).toBeHidden();
    }
  });

  test("items table contains seeded data rows", async ({ page }) => {
    const rows = page.locator(".items-table tbody tr");
    expect(await rows.count()).toBeGreaterThanOrEqual(1);
  });

  test("on mobile, items are displayed as cards", async ({ page, isMobile }) => {
    test.skip(!isMobile, "Card layout only applies on mobile");
    const thead = page.locator(".items-table thead");
    await expect(thead).toBeHidden();
    const firstRow = page.locator(".items-table tbody tr").first();
    const display = await firstRow.evaluate((el) => getComputedStyle(el).display);
    expect(display).toBe("flex");
  });
});
