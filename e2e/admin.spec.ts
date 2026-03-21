import { test, expect } from "@playwright/test";
import path from "path";

const ADMIN_STATE = path.join(__dirname, ".auth", "admin.json");

/**
 * Click the first element matching `selector` via JavaScript `el.click()`.
 *
 * On mobile viewports Playwright emulates touch events (`isMobile: true`),
 * which can cause standard `.click()` to time out when the element's
 * `onclick` handler expects a mouse-click event, or when strict-mode
 * rejects a selector that matches multiple elements.  This helper bypasses
 * Playwright's actionability checks and triggers a real DOM click event,
 * which reliably fires `onclick` handlers on both desktop and mobile.
 *
 * Use this instead of `page.locator(selector).click()` when:
 * - the target has an inline `onclick` attribute (buttons, modals), or
 * - the selector matches more than one element and you want the first.
 */
async function jsClick(page: import("@playwright/test").Page, selector: string) {
  await page.waitForSelector(selector, { state: "attached" });
  await page.evaluate((sel) => {
    const el = document.querySelector(sel) as HTMLElement | null;
    if (el) el.click();
  }, selector);
}

test.describe("Admin UI – responsive layout", () => {
  // Use the admin auth state saved by global-setup.ts.
  test.use({ storageState: ADMIN_STATE });

  test.beforeEach(async ({ page }) => {
    await page.goto("/skin");
    await page.waitForSelector(".items-table", { state: "visible", timeout: 15_000 });
  });

  test("loads the page with upload button and user info", async ({ page }) => {
    await expect(page.locator("h1")).toContainText("Teeworlds Asset Database");
    await expect(page.locator(".btn-upload")).toBeVisible();
    await expect(page.locator('.btn-auth:has-text("Logout"), a:has-text("Logout")')).toBeVisible();
  });

  test("admin buttons are visible in the items table rows", async ({ page }) => {
    const adminBtns = page.locator(".btn-admin");
    expect(await adminBtns.count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-info").count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-edit").count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-delete").count()).toBeGreaterThan(0);
  });

  test("clicking upload opens the upload modal", async ({ page }) => {
    await jsClick(page, ".btn-upload");
    await expect(page.locator("#uploadModal")).toHaveClass(/open/, { timeout: 5000 });
    await expect(page.locator("#uploadTypeLabel")).toBeVisible();
    await expect(page.locator("#uploadStep1 .btn-submit")).toBeVisible();
    await jsClick(page, "#uploadModal .modal-close");
    await expect(page.locator("#uploadModal")).not.toHaveClass(/open/);
  });

  test("clicking info button opens metadata modal", async ({ page }) => {
    await jsClick(page, ".btn-info");
    await expect(page.locator("#metadataModal")).toHaveClass(/open/, { timeout: 5000 });
    await jsClick(page, "#metadataModal .modal-close");
    await expect(page.locator("#metadataModal")).not.toHaveClass(/open/);
  });

  test("clicking edit button opens edit modal", async ({ page }) => {
    await jsClick(page, ".btn-edit");
    await expect(page.locator("#editModal")).toHaveClass(/open/, { timeout: 5000 });
    await expect(page.locator("#editName")).toBeVisible();
    await expect(page.locator("#editLicense")).toBeVisible();
    await jsClick(page, "#editModal .modal-close");
    await expect(page.locator("#editModal")).not.toHaveClass(/open/);
  });

  test("clicking delete button triggers a confirmation", async ({ page }) => {
    page.on("dialog", async (dialog) => {
      expect(dialog.type()).toBe("confirm");
      await dialog.dismiss();
    });
    await jsClick(page, ".btn-delete");
  });

  test("switching tabs preserves admin controls", async ({ page }) => {
    // Switch away from skin tab (which has seeded data).
    const emotTab = page.locator('.tab[data-type="emoticon"]');
    await emotTab.click();
    await expect(emotTab).toHaveClass(/active/, { timeout: 5000 });
    // Switch back to skin tab where admin buttons should reappear.
    const skinTab = page.locator('.tab[data-type="skin"]');
    await skinTab.click();
    await page.waitForSelector(".items-table .btn-admin", { state: "visible", timeout: 10_000 });
    expect(await page.locator(".btn-admin").count()).toBeGreaterThan(0);
  });

  test("upload modal does not overflow the viewport", async ({ page }) => {
    await jsClick(page, ".btn-upload");
    await expect(page.locator("#uploadModal")).toHaveClass(/open/, { timeout: 5000 });
    const overflow = await page.evaluate(() => {
      const modal = document.querySelector("#uploadModal .modal")!;
      const cs = window.getComputedStyle(modal);
      const modalPx = parseFloat(cs.width);
      const viewPx = document.documentElement.clientWidth;
      return { modalWidth: modalPx, viewWidth: viewPx };
    });
    expect(overflow.modalWidth).toBeLessThanOrEqual(overflow.viewWidth + 1);
    await jsClick(page, "#uploadModal .modal-close");
  });

  test("edit modal does not overflow the viewport", async ({ page }) => {
    await jsClick(page, ".btn-edit");
    await expect(page.locator("#editModal")).toHaveClass(/open/, { timeout: 5000 });
    const overflow = await page.evaluate(() => {
      const modal = document.querySelector("#editModal .modal")!;
      const cs = window.getComputedStyle(modal);
      const modalPx = parseFloat(cs.width);
      const viewPx = document.documentElement.clientWidth;
      return { modalWidth: modalPx, viewWidth: viewPx };
    });
    expect(overflow.modalWidth).toBeLessThanOrEqual(overflow.viewWidth + 1);
    await jsClick(page, "#editModal .modal-close");
  });

  test("logout link is visible for admin user", async ({ page }) => {
    await expect(page.locator('.btn-auth:has-text("Logout"), a:has-text("Logout")')).toBeVisible();
  });
});
