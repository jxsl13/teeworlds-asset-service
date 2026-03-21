import { test, expect } from "@playwright/test";

// Helper: click the first element matching `selector` via JavaScript,
// bypassing Playwright's strict-mode and mobile touch/actionability checks.
async function jsClick(page: import("@playwright/test").Page, selector: string) {
  await page.waitForSelector(selector, { state: "attached" });
  await page.evaluate((sel) => {
    const el = document.querySelector(sel) as HTMLElement | null;
    if (el) el.click();
  }, selector);
}

test.describe("Admin UI – responsive layout", () => {
  test.beforeEach(async ({ page, context }) => {
    await context.addCookies([
      { name: "e2e_admin", value: "1", url: "http://localhost:3333" },
    ]);
    await page.goto("/?admin=1");
    await expect(page.locator(".items-table")).toBeVisible({ timeout: 10000 });
  });

  test("loads the page with upload button and user badge", async ({
    page,
  }) => {
    await expect(page.locator("h1")).toContainText("Teeworlds Asset Database");
    await expect(page.locator(".btn-upload")).toBeVisible();
    await expect(page.locator(".user-badge")).toContainText("TestAdmin");
  });

  test("admin buttons are visible in the items table rows", async ({
    page,
  }) => {
    const adminBtns = page.locator(".btn-admin");
    expect(await adminBtns.count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-info").count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-edit").count()).toBeGreaterThan(0);
    expect(await page.locator(".btn-delete").count()).toBeGreaterThan(0);
  });

  test("clicking upload opens the upload modal", async ({ page }) => {
    await jsClick(page, ".btn-upload");
    await expect(page.locator("#uploadModal")).toHaveClass(/open/, {
      timeout: 5000,
    });
    await expect(page.locator("#uploadTypeLabel")).toBeVisible();
    await expect(page.locator("#uploadStep1 .btn-submit")).toBeVisible();
    await jsClick(page, "#uploadModal .modal-close");
    await expect(page.locator("#uploadModal")).not.toHaveClass(/open/);
  });

  test("clicking info button opens metadata modal", async ({ page }) => {
    await jsClick(page, ".btn-info");
    await expect(page.locator("#metadataModal")).toHaveClass(/open/, {
      timeout: 5000,
    });
    await jsClick(page, "#metadataModal .modal-close");
    await expect(page.locator("#metadataModal")).not.toHaveClass(/open/);
  });

  test("clicking edit button opens edit modal", async ({ page }) => {
    await jsClick(page, ".btn-edit");
    await expect(page.locator("#editModal")).toHaveClass(/open/, {
      timeout: 5000,
    });
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
    const mapTab = page.locator('.tab[data-type="map"]');
    await mapTab.click();
    await expect(page.locator(".items-table")).toBeVisible({ timeout: 5000 });
    expect(await page.locator(".btn-admin").count()).toBeGreaterThan(0);
  });

  test("upload modal fits within viewport", async ({ page }) => {
    await jsClick(page, ".btn-upload");
    await expect(page.locator("#uploadModal")).toHaveClass(/open/, {
      timeout: 5000,
    });
    const modal = page.locator("#uploadModal .modal");
    const box = await modal.boundingBox();
    expect(box).not.toBeNull();
    if (box) {
      const viewport = page.viewportSize();
      if (viewport) {
        expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 2);
      }
    }
    await jsClick(page, "#uploadModal .modal-close");
  });

  test("edit modal fits within viewport", async ({ page }) => {
    await jsClick(page, ".btn-edit");
    await expect(page.locator("#editModal")).toHaveClass(/open/, {
      timeout: 5000,
    });
    const modal = page.locator("#editModal .modal");
    const box = await modal.boundingBox();
    expect(box).not.toBeNull();
    if (box) {
      const viewport = page.viewportSize();
      if (viewport) {
        expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 2);
      }
    }
    await jsClick(page, "#editModal .modal-close");
  });

  test("logout link is visible for admin user", async ({ page }) => {
    await expect(page.locator(".btn-auth")).toContainText("Logout");
  });
});
