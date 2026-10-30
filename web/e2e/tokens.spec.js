import { test, expect, account, apiToken, login } from "./fixtures.js";
import { unique } from "./env.js";

test.beforeEach(({ serverConfig }) => {
  test.skip(!serverConfig.enable_login, "login disabled");
});

test("create, edit and delete an access token", async ({ page, request, users }) => {
  const username = await users.create();
  const sessionToken = await apiToken(request, username);
  await login(page, username);
  await page.goto("/account");
  const table = page.getByRole("table", { name: "Access tokens" });
  await expect(table.getByText("Current browser session")).toBeVisible();

  // Create
  await page.getByRole("button", { name: "Create access token" }).click();
  let dialog = page.getByRole("dialog", { name: "Create access token" });
  await dialog.getByRole("textbox", { name: "Label, e.g. Radarr notifications", exact: true }).fill("Radarr");
  await dialog.getByRole("combobox").click();
  await page.getByRole("option", { name: "Token expires in 3 days" }).click();
  await dialog.getByRole("button", { name: "Create token" }).click();
  await expect(dialog).toBeHidden();
  const row = table.getByRole("row").filter({ hasText: "Radarr" });
  await expect(row).toBeVisible();

  // The new token is real: it can publish as the user
  const created = (await account(request, sessionToken)).tokens.find((t) => t.label === "Radarr");
  const inThreeDays = Date.now() / 1000 + 3 * 24 * 3600;
  expect(Math.abs(created.expires - inThreeDays)).toBeLessThan(60);
  const topic = unique("token");
  const publish = () => request.post(`/${topic}`, { headers: { Authorization: `Bearer ${created.token}` }, data: "via token" });
  expect((await publish()).status()).toBe(200);

  // Edit the label and make it never expire
  await row.getByRole("button", { name: "Edit access token" }).click();
  dialog = page.getByRole("dialog", { name: "Edit access token" });
  await dialog.getByRole("textbox", { name: "Label, e.g. Radarr notifications", exact: true }).fill("Sonarr");
  await dialog.getByRole("combobox").click();
  await page.getByRole("option", { name: "Token never expires" }).click();
  await dialog.getByRole("button", { name: "Update token" }).click();
  await expect(table.getByRole("row").filter({ hasText: "Sonarr" }).getByText("Never expires")).toBeVisible();

  // Delete; the token stops working
  await table.getByRole("row").filter({ hasText: "Sonarr" }).getByRole("button", { name: "Delete access token" }).click();
  dialog = page.getByRole("dialog", { name: "Delete access token" });
  await dialog.getByRole("button", { name: "Permanently delete token" }).click();
  await expect(table.getByRole("row").filter({ hasText: "Sonarr" })).toHaveCount(0);
  expect((await publish()).status()).toBe(401);
});

test("current session token cannot be deleted", async ({ page, users }) => {
  await login(page, await users.create());
  await page.goto("/account");
  // Its edit and delete buttons are unnamed, but sit under an explanatory tooltip
  const row = page.getByRole("table", { name: "Access tokens" }).getByRole("row").filter({ hasText: "Current browser session" });
  const buttons = row.getByLabel("Cannot edit or delete current session token").getByRole("button");
  await expect(buttons).toHaveCount(2);
  for (const button of await buttons.all()) {
    await expect(button).toBeDisabled();
  }
});
