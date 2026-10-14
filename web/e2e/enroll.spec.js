import { test, expect, account, apiToken, login } from "./fixtures.js";
import { unique } from "./env.js";

test.beforeEach(({ serverConfig }) => {
  test.skip(!serverConfig.enable_login, "login disabled");
});

// Mints a one-time enrollment code for the user, as an integration would via the admin API
const enrollCode = async (admin, username) => {
  const response = await admin.post("/v1/users/token/enroll", { data: { username } });
  expect(response.ok(), `enrolling ${username}: ${await response.text()}`).toBeTruthy();
  return (await response.json()).code;
};

const redeemLink = (code, username, topics) => `/account/token/redeem#code=${code}&user=${username}&subscribe=${topics.join(",")}`;

test("an enrollment link signs a signed-out browser in and subscribes it", async ({ page, request, admin, users }) => {
  const username = await users.create();
  const topics = [unique("alerts"), unique("inbox")];
  const code = await enrollCode(admin, username);

  await page.goto(redeemLink(code, username, topics));

  for (const topic of topics) {
    await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
  }
  expect(new URL(page.url()).hash).toBe(""); // The code does not linger in the address bar or history

  // The browser holds its own token, labelled for the web app, and the code is spent
  const state = await account(request, await apiToken(request, username));
  expect(state.username).toBe(username);
  expect(state.tokens.map((t) => t.label)).toContain("Web app");
  expect(state.subscriptions.map((s) => s.topic).sort()).toEqual([...topics].sort());
  expect((await request.post("/v1/account/token/redeem", { data: { code } })).status()).toBe(400);
});

test("an enrollment link only subscribes when already signed in as its user", async ({ page, request, admin, users }) => {
  const username = await users.create();
  await login(page, username);
  const topic = unique("alerts");
  const code = await enrollCode(admin, username);

  await page.goto(redeemLink(code, username, [topic]));

  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
  // The code was left unused, so it still works (and the browser kept its own session)
  expect((await request.post("/v1/account/token/redeem", { data: { code } })).status()).toBe(200);
});

test("an enrollment link never replaces another user's session", async ({ page, request, admin, users }) => {
  const signedIn = await users.create();
  const other = await users.create();
  await login(page, signedIn);
  const code = await enrollCode(admin, other);

  await page.goto(redeemLink(code, other, [unique("alerts")]));

  await expect(page.getByText(`You're already signed in as ${signedIn}`)).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("user"))).toBe(signedIn);
  expect((await request.post("/v1/account/token/redeem", { data: { code } })).status()).toBe(200);
});

test("a used enrollment link explains that it no longer works", async ({ page, request, admin, users }) => {
  const username = await users.create();
  const code = await enrollCode(admin, username);
  expect((await request.post("/v1/account/token/redeem", { data: { code } })).status()).toBe(200);

  await page.goto(redeemLink(code, username, []));

  await expect(page.getByText("This sign-in link is invalid, has expired, or was already used")).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("user"))).toBeNull();
});
