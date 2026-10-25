import { test, expect, subscribe } from "./fixtures.js";
import { unique } from "./env.js";

test.describe("upgrade banner", () => {
  // The e2e server has no Stripe, so payments are switched on in the served config only, and
  // the tier list (fetched by the upgrade dialog) is stubbed with just the free tier. Specs must
  // stay on one page load: once the service worker controls the page, it serves config.js itself.
  test.beforeEach(async ({ page }) => {
    await page.route("**/config.js", async (route) => {
      const response = await route.fetch();
      const body = (await response.text()).replace(/"enable_payments": false/, '"enable_payments": true');
      await route.fulfill({ response, body });
    });
    const limits = { messages: 250, messages_expiry_duration: 43200, emails: 5, calls: 0, reservations: 0 };
    await page.route("**/v1/billing/tiers", (route) => route.fulfill({ json: [{ limits }] }));
  });

  test("does not cover the bottom of a long topic list", async ({ page }) => {
    // #1007: the banner was position: fixed on top of the list, hiding the last buttons
    // Tall while subscribing, so the setup itself doesn't run into the bug
    await page.setViewportSize({ width: 1280, height: 1600 });
    await page.goto("/app");
    for (let i = 0; i < 8; i++) {
      await subscribe(page, unique("nav"));
    }
    await page.setViewportSize({ width: 1280, height: 600 });
    const banner = page.getByRole("button", { name: /Upgrade to ntfy Pro/ });
    const subscribeButton = page.getByRole("button", { name: "Subscribe to topic" });
    await expect(banner).toBeVisible();

    await subscribeButton.scrollIntoViewIfNeeded();
    const subscribeBox = await subscribeButton.boundingBox();
    const bannerBox = await banner.boundingBox();
    expect(subscribeBox.y + subscribeBox.height).toBeLessThanOrEqual(bannerBox.y);
    await subscribeButton.click();
    await expect(page.getByRole("dialog")).toBeVisible();
  });
});

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
