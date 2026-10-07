import { test, expect, account, apiToken, login, selectPref, subscribe } from "./fixtures.js";
import { baseURL, unique } from "./env.js";

test("settings page renders", async ({ page }) => {
  await page.goto("/settings");
  for (const heading of ["Notifications", "Manage users", "Appearance"]) {
    await expect(page.getByRole("heading", { name: heading })).toBeVisible();
  }
});

test("notification prefs persist across reloads", async ({ page }) => {
  await page.goto("/settings");
  await selectPref(page, "Notification sound", "Pop");
  await selectPref(page, "Minimum priority", "High priority and higher");
  await selectPref(page, "Delete notifications", "After one day");
  await page.reload();
  await expect(page.getByText("Notifications play the Pop sound when they arrive")).toBeVisible();
  await expect(page.getByText("Show notifications if priority is 4 (high) or above")).toBeVisible();
  await expect(page.getByText("Notifications are auto-deleted after one day")).toBeVisible();
});

test("notification prefs sync to the account", async ({ page, request, users }) => {
  const username = await users.create();
  await login(page, username);
  await page.goto("/settings");
  await selectPref(page, "Notification sound", "No sound");
  await selectPref(page, "Minimum priority", "Only max priority");
  await selectPref(page, "Delete notifications", "Never");
  const token = await apiToken(request, username);
  await expect.poll(async () => (await account(request, token)).notification).toEqual({ sound: "none", min_priority: 5, delete_after: 0 });
});

test("theme", async ({ page }) => {
  await page.goto("/settings");
  await selectPref(page, "Theme", "Dark mode");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.reload();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await selectPref(page, "Theme", "Light mode");
  await expect(page.locator("html")).not.toHaveClass(/dark/);
});

test("date format applies to notifications", async ({ page, request }) => {
  const topic = unique("date");
  await page.goto("/app");
  await subscribe(page, topic);
  await request.post(`/${topic}`, { data: "dated message" });
  await page.getByRole("button", { name: "Settings" }).click();
  await selectPref(page, "Date format", /^ISO 8601/);
  await page.getByRole("button", { name: topic, exact: true }).click();
  await expect(page.getByRole("listitem", { name: "Notification" })).toContainText(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
});

test.describe("language", () => {
  test("switch to German and back", async ({ page }) => {
    await page.goto("/settings");
    const langFile = page.waitForResponse((r) => r.url().endsWith("/static/langs/de.json"));
    await selectPref(page, /^Language/, "Deutsch");
    expect((await langFile).ok()).toBeTruthy();
    await expect(page.getByRole("heading", { name: "Darstellung" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Thema abonnieren" })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "de");

    await page.reload();
    await expect(page.getByRole("heading", { name: "Darstellung" })).toBeVisible();

    await selectPref(page, /^Sprache/, "English");
    await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
  });

  test("right-to-left language", async ({ page }) => {
    await page.goto("/settings");
    await selectPref(page, /^Language/, "العربية");
    await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  });

  test("language syncs to the account", async ({ page, browser, request, users }) => {
    const username = await users.create();
    await login(page, username);
    await page.goto("/settings");
    await selectPref(page, /^Language/, "Français");
    await expect(page.getByRole("heading", { name: "Apparence" })).toBeVisible();
    const token = await apiToken(request, username);
    await expect.poll(async () => (await account(request, token)).language).toBe("fr");

    // Another browser picks it up after login
    const context = await browser.newContext({ baseURL });
    const other = await context.newPage();
    await login(other, username);
    await other.goto("/settings");
    await expect(other.getByRole("heading", { name: "Apparence" })).toBeVisible();
    await context.close();
  });
});

test.describe("manage users", () => {
  test("add, edit and delete a user", async ({ page }) => {
    await page.goto("/settings");
    await page.getByRole("button", { name: "Add user" }).click();
    let dialog = page.getByRole("dialog", { name: "Add user" });
    await dialog.getByRole("textbox", { name: "Service URL, e.g. https://ntfy.sh", exact: true }).fill("not a url");
    await expect(dialog.getByText("Invalid URL format. Must start with http:// or https://")).toBeVisible();
    await dialog.getByRole("textbox", { name: "Service URL, e.g. https://ntfy.sh", exact: true }).fill("https://ntfy.example.com");
    await dialog.getByRole("textbox", { name: "Username, e.g. phil", exact: true }).fill("alice");
    await dialog.getByRole("textbox", { name: "Password", exact: true }).fill("alice-password");
    await dialog.getByRole("button", { name: "Add" }).click();
    const table = page.getByRole("table", { name: "Users table" });
    const row = table.getByRole("row").filter({ hasText: "https://ntfy.example.com" });
    await expect(row).toContainText("alice");

    await page.reload();
    await row.getByRole("button", { name: "Edit user" }).click();
    dialog = page.getByRole("dialog", { name: "Edit user" });
    await dialog.getByRole("textbox", { name: "Username, e.g. phil", exact: true }).fill("bob");
    await dialog.getByRole("textbox", { name: "Password", exact: true }).fill("bob-password");
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(row).toContainText("bob");

    await row.getByRole("button", { name: "Delete user" }).click();
    await expect(row).toHaveCount(0);
  });

  test("duplicate service URL is rejected", async ({ page }) => {
    await page.goto("/settings");
    for (const attempt of [1, 2]) {
      await page.getByRole("button", { name: "Add user" }).click();
      const dialog = page.getByRole("dialog", { name: "Add user" });
      await dialog.getByRole("textbox", { name: "Service URL, e.g. https://ntfy.sh", exact: true }).fill("https://dup.example.com");
      if (attempt === 2) {
        await expect(dialog.getByText("A user for this service URL already exists")).toBeVisible();
        break;
      }
      await dialog.getByRole("textbox", { name: "Username, e.g. phil", exact: true }).fill("alice");
      await dialog.getByRole("textbox", { name: "Password", exact: true }).fill("alice-password");
      await dialog.getByRole("button", { name: "Add" }).click();
      await expect(dialog).toBeHidden();
    }
  });
});
