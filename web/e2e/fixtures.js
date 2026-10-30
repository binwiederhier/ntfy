import { test as base, expect } from "@playwright/test";
import { adminToken, baseURL, password, tier, unique } from "./env.js";

// Errors that are not bugs: headless Chromium has no notification backend, so
// showNotification() throws, even with the permission granted or before the worker is active.
const ignoredErrors = [/Failed to execute 'showNotification'/];

// Every spec fails on an uncaught exception or a console error, without each one
// having to remember to check. A missing import only throws on the path that uses it.
export const test = base.extend({
  // A random client IP per test, so per-visitor rate limits start fresh (see server.mjs)
  extraHTTPHeaders: async ({}, use) => {
    const octet = () => Math.floor(Math.random() * 256);
    await use({ "X-Forwarded-For": `10.${octet()}.${octet()}.${octet()}` });
  },
  page: async ({ page }, use) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push(`uncaught: ${e.message}`));
    page.on("console", (m) => {
      // Non-2xx responses are logged as console errors; specs assert on statuses directly.
      // Known ones: en-US.json 404 (i18n tries the region first), the 401 session check.
      if (m.type() === "error" && !/^Failed to load resource/.test(m.text())) {
        errors.push(m.text());
      }
    });

    await use(page);

    const unexpected = errors.filter((e) => !ignoredErrors.some((re) => re.test(e)));
    expect(unexpected, `JavaScript errors on the page:\n  ${unexpected.join("\n  ")}`).toEqual([]);
  },

  // The server's /v1/config, so specs can skip features the server under test has disabled
  serverConfig: [
    async ({ playwright }, use) => {
      const request = await playwright.request.newContext({ baseURL });
      await use(await (await request.get("/v1/config")).json());
      await request.dispose();
    },
    { scope: "worker" },
  ],

  // Admin API client, used to create test users without the 6/day signup limit
  admin: [
    async ({ playwright }, use) => {
      const request = await playwright.request.newContext({ baseURL, extraHTTPHeaders: { Authorization: `Bearer ${adminToken()}` } });
      await use(request);
      await request.dispose();
    },
    { scope: "worker" },
  ],

  // Creates users via the admin API (optionally on the test tier) and deletes them after the
  // test, including users that a spec signed up through the UI and handed to track()
  users: async ({ admin }, use) => {
    const created = [];
    await use({
      create: async ({ paid = false } = {}) => {
        const username = unique("user");
        const response = await admin.post("/v1/users", { data: { username, password, tier: paid ? tier() : "" } });
        expect(response.ok(), `creating user ${username}: ${await response.text()}`).toBeTruthy();
        created.push(username);
        return username;
      },
      track: (username) => created.push(username),
    });
    for (const username of created) {
      await admin.delete("/v1/users", { data: { username } });
    }
  },
});

// Resolves once the topic's WebSocket got the server's "open" event. The first connect has no
// "since", so anything published before that only shows up with the next poll, minutes later.
export const connected = (page, topic) =>
  new Promise((resolve) => {
    page.on("websocket", (ws) => {
      if (new URL(ws.url()).pathname.endsWith(`/${topic}/ws`)) {
        ws.on("framereceived", (frame) => {
          if (JSON.parse(frame.payload).event === "open") resolve();
        });
      }
    });
  });

// Subscribes to a topic through the "Subscribe to topic" dialog and waits until it is live.
// It also waits for the poll that follows subscribing: a message arriving through both at once
// trips a check-then-add race in SubscriptionManager.addNotification (harmless, but logged).
export const subscribe = async (page, topic, { reserve = false } = {}) => {
  const live = connected(page, topic);
  const polled = page.waitForResponse((r) => new URL(r.url()).pathname.endsWith(`/${topic}/json`));
  await page.getByRole("button", { name: "Subscribe to topic" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel(/topic name/i).fill(topic);
  if (reserve) {
    await dialog.getByRole("switch", { name: /reserve topic/i }).check();
  }
  await dialog.getByRole("button", { name: "Subscribe", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("button", { name: topic, exact: true })).toBeVisible();
  await Promise.all([live, polled]);
};

// Signs up a new user through /signup; the app logs them in and lands on /app.
export const signup = async (page, username) => {
  await page.goto("/signup");
  await page.getByRole("textbox", { name: "Username" }).fill(username);
  await page.getByRole("textbox", { name: "Password", exact: true }).fill(password);
  await page.getByRole("textbox", { name: "Confirm password" }).fill(password);
  await page.getByRole("button", { name: "Sign up" }).click();
  await expect(page.getByRole("button", { name: "Subscribe to topic" })).toBeVisible();
};

// Logs in through /login and waits for the reload into the app root (the account's language
// may not be English, so don't wait for any text).
export const login = async (page, username, pass = password) => {
  await page.goto("/login");
  await page.getByRole("textbox", { name: "Username" }).fill(username);
  await page.getByRole("textbox", { name: "Password", exact: true }).fill(pass);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).not.toHaveURL(/\/login$/);
  await expect(page.getByRole("navigation").first()).toBeVisible();
};

// Logs a user in via the API and returns a bearer token, for setup and for checking server state.
export const apiToken = async (request, username, pass = password) => {
  const auth = Buffer.from(`${username}:${pass}`).toString("base64");
  const response = await request.post("/v1/account/token", { headers: { Authorization: `Basic ${auth}` } });
  expect(response.ok(), `login of ${username}: ${await response.text()}`).toBeTruthy();
  return (await response.json()).token;
};

// GET /v1/account as the given token's user.
export const account = async (request, token) =>
  (await request.get("/v1/account", { headers: { Authorization: `Bearer ${token}` } })).json();

// Picks an option in a settings select. These comboboxes have no accessible name (MUI only
// forwards labelId/inputProps to the combobox), so find them by their Pref row title.
export const selectPref = async (page, title, option) => {
  await page
    .getByRole("row")
    .filter({ has: page.getByRole("cell", { name: title }) })
    .getByRole("combobox")
    .click();
  await page.getByRole("option", { name: option, exact: true }).click();
};

// Opens the subscription menu from the sidebar topic row (its icon button has no name).
export const openTopicMenu = async (page, topic) => {
  const row = page.getByRole("button", { name: topic, exact: true });
  await row.hover();
  await row.getByRole("button").click();
};

export { expect };
