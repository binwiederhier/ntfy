import { test, expect } from "./fixtures.js";

test.describe("notification permission alert", () => {
  // Pretend the browser hasn't been asked yet; Playwright's default context grants notifications
  test.use({ permissions: [] });
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => Object.defineProperty(Notification, "permission", { get: () => "default" }));
  });

  test("can be dismissed for good", async ({ page }) => {
    // #1220
    await page.goto("/app");
    const alert = page.getByRole("alert").filter({ hasText: "Notifications are disabled" });
    await expect(alert).toBeVisible();

    const close = alert.getByRole("button", { name: "Close" });
    await expect(close).toBeVisible();
    await close.click();
    await expect(alert).toBeHidden();
    await page.reload();
    await expect(page.getByRole("button", { name: "Subscribe to topic" })).toBeVisible();
    await expect(alert).toBeHidden();
  });
});
