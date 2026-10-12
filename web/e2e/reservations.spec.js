import { test, expect, account, apiToken, login, openTopicMenu, subscribe } from "./fixtures.js";
import { unique } from "./env.js";

test.beforeEach(({ serverConfig }) => {
  test.skip(!serverConfig.enable_reservations, "reservations disabled");
});

// Anonymous publish/subscribe status codes for a topic; default access is read-write, so only
// the reservation can deny anything
const anonymous = async (request, topic) => ({
  publish: (await request.post(`/${topic}`, { data: "anonymous" })).status(),
  subscribe: (await request.get(`/${topic}/json?poll=1`)).status(),
});

test("reserve a topic while subscribing", async ({ page, request, users }) => {
  await login(page, await users.create({ paid: true }));
  const topic = unique("reserved");
  await subscribe(page, topic, { reserve: true });
  await expect.poll(() => anonymous(request, topic)).toEqual({ publish: 403, subscribe: 403 });
});

test("reserve switch is disabled without a tier", async ({ page, users }) => {
  await login(page, await users.create());
  await page.getByRole("button", { name: "Subscribe to topic" }).click();
  await expect(page.getByRole("dialog").getByRole("switch", { name: /reserve topic/i })).toBeDisabled();
});

test("add, edit and delete a reservation in settings", async ({ page, request, users }) => {
  const username = await users.create({ paid: true });
  const topic = unique("reserved");
  await login(page, username);
  await page.goto("/settings");

  // Add as "everyone can subscribe"
  await page.getByRole("button", { name: "Add reserved topic" }).click();
  let dialog = page.getByRole("dialog", { name: "Reserve topic" });
  await dialog.getByRole("textbox", { name: "Topic", exact: true }).fill(topic);
  await dialog.getByRole("combobox").click();
  await page.getByRole("option", { name: "I can publish and subscribe, everyone can subscribe" }).click();
  await dialog.getByRole("button", { name: "Add" }).click();
  const table = page.getByRole("table", { name: "Reserved topics table" });
  const row = table.getByRole("row").filter({ hasText: topic });
  await expect(row).toContainText("I can publish and subscribe, everyone can subscribe");
  await expect.poll(() => anonymous(request, topic)).toEqual({ publish: 403, subscribe: 200 });

  // Not subscribed yet; the chip subscribes
  await row.getByText("Not subscribed").click();
  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();

  // Edit to "everyone can publish and subscribe"
  await page.goto("/settings");
  await row.getByRole("button", { name: "Edit topic access" }).click();
  dialog = page.getByRole("dialog", { name: "Edit reserved topic" });
  await dialog.getByRole("combobox").click();
  await page.getByRole("option", { name: "Everyone can publish and subscribe" }).click();
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(row).toContainText("Everyone can publish and subscribe");
  await expect.poll(() => anonymous(request, topic)).toEqual({ publish: 200, subscribe: 200 });

  // Delete, along with the cached messages
  await row.getByRole("button", { name: "Reset topic access" }).click();
  dialog = page.getByRole("dialog", { name: "Delete topic reservation" });
  await dialog.getByRole("combobox").click();
  await page.getByRole("option", { name: "Delete cached messages and attachments" }).click();
  await dialog.getByRole("button", { name: "Delete reservation" }).click();
  await expect(row).toHaveCount(0);
  const token = await apiToken(request, username);
  expect((await account(request, token)).reservations ?? []).toEqual([]);
  await expect.poll(async () => (await (await request.get(`/${topic}/json?poll=1`)).text()).trim()).toBe("");
});

test("reserve and unreserve from the subscription menu", async ({ page, request, users }) => {
  await login(page, await users.create({ paid: true }));
  const topic = unique("reserved");
  await subscribe(page, topic);

  await openTopicMenu(page, topic);
  await page.getByRole("menuitem", { name: "Reserve topic" }).click();
  const dialog = page.getByRole("dialog", { name: "Reserve topic" });
  await dialog.getByRole("button", { name: "Add" }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => anonymous(request, topic)).toEqual({ publish: 403, subscribe: 403 });

  await openTopicMenu(page, topic);
  await page.getByRole("menuitem", { name: "Remove reservation" }).click();
  await page.getByRole("dialog", { name: "Delete topic reservation" }).getByRole("button", { name: "Delete reservation" }).click();
  await expect.poll(() => anonymous(request, topic)).toEqual({ publish: 200, subscribe: 200 });
});

test("an already reserved topic cannot be taken", async ({ page, request, users }) => {
  const owner = await users.create({ paid: true });
  const topic = unique("taken");
  const token = await apiToken(request, owner);
  await request.post("/v1/account/reservation", { headers: { Authorization: `Bearer ${token}` }, data: { topic, everyone: "read-write" } });

  await login(page, await users.create({ paid: true }));
  await page.goto("/settings");
  await page.getByRole("button", { name: "Add reserved topic" }).click();
  const dialog = page.getByRole("dialog", { name: "Reserve topic" });
  await dialog.getByRole("textbox", { name: "Topic", exact: true }).fill(topic);
  await dialog.getByRole("button", { name: "Add" }).click();
  await expect(dialog.getByText("Topic already reserved")).toBeVisible();
});
