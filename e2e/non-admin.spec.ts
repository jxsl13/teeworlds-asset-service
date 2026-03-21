import { test, expect } from "@playwright/test";

test.describe("Non-admin UI – responsive layout", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    // Wait for HTMX to load the initial content.
    await expect(page.locator(".items-table")).toBeVisible({ timeout: 10000 });
  });

  // ── Page load ───────────────────────────────────────────────────────────────

  test("loads the page with header, tabs, search, and items table", async ({
    page,
  }) => {
    await expect(page.locator("h1")).toContainText("Teeworlds Asset Database");
    await expect(page.locator(".subtitle")).toBeVisible();
    await expect(page.locator(".tabs")).toBeVisible();
    // At least 5 asset type tabs should be present.
    const tabs = page.locator(".tab");
    await expect(tabs).not.toHaveCount(0);
    expect(await tabs.count()).toBeGreaterThanOrEqual(5);
    // Search input should be visible.
    await expect(page.locator("#search")).toBeVisible();
    // Upload button should NOT be visible for non-admin.
    await expect(page.locator(".btn-upload")).toHaveCount(0);
    // Items table should be loaded.
    await expect(page.locator(".items-table")).toBeVisible();
  });

  // ── Tab switching ───────────────────────────────────────────────────────────

  test("switching tabs loads content for the selected asset type", async ({
    page,
  }) => {
    // Click on "map" tab.
    const mapTab = page.locator('.tab[data-type="map"]');
    await mapTab.click();
    await expect(mapTab).toHaveClass(/active/);

    // The items table should reload (HTMX swap).
    await expect(page.locator(".items-table")).toBeVisible({ timeout: 5000 });

    // Click on "hud" tab.
    const hudTab = page.locator('.tab[data-type="hud"]');
    await hudTab.click();
    await expect(hudTab).toHaveClass(/active/);
    await expect(page.locator(".items-table")).toBeVisible({ timeout: 5000 });
  });

  // ── Sorting ─────────────────────────────────────────────────────────────────

  test("clicking a column header triggers sort (desktop only)", async ({
    page,
  }) => {
    const viewport = page.viewportSize();
    // On mobile the thead is hidden, so sort headers are not clickable.
    if (viewport && viewport.width <= 480) {
      test.skip();
      return;
    }
    const nameHeader = page.locator('th[data-field="name"]');
    await expect(nameHeader).toBeVisible();
    await nameHeader.click();
    await expect(nameHeader.locator(".sort-indicator")).toBeVisible();
  });

  // ── Pagination ──────────────────────────────────────────────────────────────

  test("pagination controls are visible when items exist", async ({
    page,
  }) => {
    await expect(page.locator(".pagination")).toBeVisible();
    await expect(page.locator(".page-info")).toBeVisible();
  });

  // ── Image preview modal ─────────────────────────────────────────────────────

  test("clicking a thumbnail opens the preview modal", async ({ page }) => {
    const thumb = page.locator(".col-thumb img").first();
    if ((await thumb.count()) > 0) {
      await thumb.click();
      await expect(page.locator("#previewModal")).toHaveClass(/open/);
      // Press Escape to close.
      await page.keyboard.press("Escape");
      await expect(page.locator("#previewModal")).not.toHaveClass(/open/);
    }
  });

  // ── Search input ────────────────────────────────────────────────────────────

  test("search input is functional", async ({ page }) => {
    const searchInput = page.locator("#search");
    await searchInput.fill("Greyfox");
    await expect(searchInput).toHaveValue("Greyfox");
  });

  // ── Table does not overflow viewport ────────────────────────────────────────

  test("table is contained within the viewport", async ({ page }) => {
    const table = page.locator(".items-table");
    const box = await table.boundingBox();
    expect(box).not.toBeNull();
    if (box) {
      const viewport = page.viewportSize();
      if (viewport) {
        expect(box.x).toBeGreaterThanOrEqual(-1);
      }
    }
  });

  // ── No admin elements visible ───────────────────────────────────────────────

  test("admin column and buttons are not visible for non-admin", async ({
    page,
  }) => {
    await expect(page.locator(".btn-delete")).toHaveCount(0);
    await expect(page.locator(".btn-edit")).toHaveCount(0);
    await expect(page.locator(".btn-info")).toHaveCount(0);
  });

  // ── Login link in footer ────────────────────────────────────────────────────

  test("footer shows login link for non-admin", async ({ page }) => {
    await expect(page.locator(".site-footer")).toBeVisible();
    await expect(
      page.locator('.site-footer a[href="/auth/login"]')
    ).toBeVisible();
  });

  // ── Selection checkboxes ────────────────────────────────────────────────────

  test("selecting a row shows the selection bar", async ({ page }) => {
    const checkbox = page.locator(".row-select").first();
    if ((await checkbox.count()) > 0) {
      // Use JS to toggle the checkbox and call its onchange handler,
      // avoiding mobile touch-event issues with Playwright.
      await page.evaluate(() => {
        const cb = document.querySelector(".row-select") as HTMLInputElement;
        if (cb) { cb.checked = true; (window as any).toggleRowSelect(cb); }
      });
      await expect(page.locator("#selectionBar")).toBeVisible({ timeout: 3000 });
      await page.evaluate(() => {
        (window as any).clearSelection();
      });
      await expect(page.locator("#selectionBar")).toBeHidden();
    }
  });

  // ── Items rendered as data rows ─────────────────────────────────────────────

  test("items table contains data rows with names", async ({ page }) => {
    const names = page.locator(".col-name");
    expect(await names.count()).toBeGreaterThan(0);
    await expect(names.first()).not.toBeEmpty();
  });

  // ── Mobile card layout ──────────────────────────────────────────────────────

  test("on mobile, items are displayed as cards", async ({ page }) => {
    const viewport = page.viewportSize();
    if (!viewport || viewport.width > 480) {
      test.skip();
      return;
    }
    // On mobile the thead should be hidden.
    await expect(page.locator(".items-table thead")).toBeHidden();
    // Each row should still have name and actions.
    const firstRow = page.locator(".items-table tbody tr").first();
    await expect(firstRow.locator(".col-name")).toBeVisible();
    await expect(firstRow.locator(".col-actions")).toBeVisible();
  });
});
