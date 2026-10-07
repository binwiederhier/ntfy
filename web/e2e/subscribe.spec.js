import { test, expect, account, apiToken, connected, login, openTopicMenu, subscribe } from "./fixtures.js";
import { baseURL, password, port, remote, unique } from "./env.js";

test("subscribe through the dialog", async ({ page }) => {
  const topic = unique("sub");
  await page.goto("/app");
  await subscribe(page, topic);
  await expect(page.getByText("You haven't received any notifications for this topic yet.")).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/${topic}$`));
});

test("generate a topic name", async ({ page }) => {
  await page.goto("/app");
  await page.getByRole("button", { name: "Subscribe to topic" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Generate name" }).click();
  const topic = await dialog.getByLabel(/topic name/i).inputValue();
  expect(topic).toMatch(/^\w{16}$/);
  await dialog.getByRole("button", { name: "Subscribe", exact: true }).click();
  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
});

test("visiting a topic URL subscribes to it", async ({ page }) => {
  const topic = unique("visit");
  await page.goto(`/${topic}`);
  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
});

test("subscribe to a topic on another server", async ({ page, request }) => {
  // localhost is a different base URL for the same local server, so it counts as "another server"
  test.skip(remote, "needs a second hostname for the server");
  const topic = unique("other");
  const otherServer = `http://localhost:${port}`;
  await page.goto("/app");
  await page.getByRole("button", { name: "Subscribe to topic" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel(/topic name/i).fill(topic);
  await dialog.getByRole("switch", { name: "Use another server" }).check();
  await dialog.getByRole("combobox").fill(otherServer);
  const live = connected(page, topic);
  await dialog.getByRole("button", { name: "Subscribe", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/localhost:${port}/${topic}$`));
  await live;

  await request.post(`/${topic}`, { data: "via the other server" });
  await expect(page.getByText("via the other server")).toBeVisible();
});

test("protected topic asks for a login", async ({ page, request, users, serverConfig }) => {
  test.skip(!serverConfig.enable_reservations, "reservations disabled");
  const owner = await users.create({ paid: true });
  const token = await apiToken(request, owner);
  const topic = unique("protected");
  const auth = { Authorization: `Bearer ${token}` };
  expect((await request.post("/v1/account/reservation", { headers: auth, data: { topic, everyone: "deny-all" } })).ok()).toBeTruthy();

  // Anonymous subscribe hits the login-required page; the owner's credentials get through
  await page.goto("/app");
  await page.getByRole("button", { name: "Subscribe to topic" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel(/topic name/i).fill(topic);
  await dialog.getByRole("button", { name: "Subscribe", exact: true }).click();
  await expect(dialog.getByText("Login required")).toBeVisible();
  await dialog.getByRole("textbox", { name: "Username, e.g. phil", exact: true }).fill(owner);
  await dialog.getByLabel("Password", { exact: true }).fill(password);
  const live = connected(page, topic);
  await dialog.getByRole("button", { name: "Login" }).click();
  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
  await live;

  // Stored credentials are used for the live connection, and listed under Settings
  await request.post(`/${topic}`, { headers: auth, data: "for the owner only" });
  await expect(page.getByText("for the owner only")).toBeVisible();
  await page.getByRole("button", { name: "Settings" }).click();
  await expect(page.getByRole("table", { name: "Users table" }).getByText(owner)).toBeVisible();
});

test.describe("subscription menu", () => {
  let topic;

  test.beforeEach(async ({ page }) => {
    topic = unique("menu");
    await page.goto("/app");
    await subscribe(page, topic);
  });

  test("change display name", async ({ page }) => {
    await openTopicMenu(page, topic);
    await page.getByRole("menuitem", { name: "Change display name" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("textbox", { name: "Display name" }).fill("My alerts");
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(page.getByRole("button", { name: "My alerts", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: topic, exact: true })).toHaveCount(0);
  });

  test("mute and unmute", async ({ page }) => {
    await openTopicMenu(page, topic);
    await page.getByRole("menuitem", { name: "Mute notifications" }).click();
    const muted = page.getByRole("button", { name: topic, exact: true }).getByLabel("Notifications muted").first();
    await expect(muted).toBeVisible();
    await openTopicMenu(page, topic);
    await page.getByRole("menuitem", { name: "Unmute notifications" }).click();
    await expect(muted).toHaveCount(0);
  });

  test("send test notification", async ({ page }) => {
    await openTopicMenu(page, topic);
    await page.getByRole("menuitem", { name: "Send test notification" }).click();
    await expect(page.getByRole("listitem", { name: "Notification" })).toHaveCount(1);
  });

  test("unsubscribe", async ({ page }) => {
    await openTopicMenu(page, topic);
    await page.getByRole("menuitem", { name: "Unsubscribe" }).click();
    await expect(page.getByRole("button", { name: topic, exact: true })).toHaveCount(0);
    await page.reload();
    await expect(page.getByText("It looks like you don't have any subscriptions yet.")).toBeVisible();
  });
});

test("subscriptions and display names sync to the account", async ({ page, browser, request, users }) => {
  const username = await users.create();
  const topic = unique("sync");
  await login(page, username);
  await subscribe(page, topic);
  await openTopicMenu(page, topic);
  await page.getByRole("menuitem", { name: "Change display name" }).click();
  await page.getByRole("dialog").getByRole("textbox", { name: "Display name" }).fill("Synced name");
  await page.getByRole("dialog").getByRole("button", { name: "Save" }).click();

  const token = await apiToken(request, username);
  await expect
    .poll(async () => (await account(request, token)).subscriptions?.map((s) => [s.topic, s.display_name]))
    .toEqual([[topic, "Synced name"]]);

  // A second browser picks the subscription up on login
  const context = await browser.newContext({ baseURL });
  const other = await context.newPage();
  await login(other, username);
  await expect(other.getByRole("button", { name: "Synced name", exact: true })).toBeVisible();

  // Unsubscribing removes it from the account
  await openTopicMenu(page, "Synced name");
  await page.getByRole("menuitem", { name: "Unsubscribe" }).click();
  await expect.poll(async () => (await account(request, token)).subscriptions ?? []).toEqual([]);
  await context.close();
});
