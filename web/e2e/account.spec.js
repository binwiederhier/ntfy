import { test, expect, login, signup } from "./fixtures.js";
import { password, unique } from "./env.js";

test.beforeEach(({ serverConfig }) => {
  test.skip(!serverConfig.enable_login, "login disabled");
});

// Logout fades out and then navigates to /app; wait for that before moving on
const logout = async (page) => {
  await page.getByRole("button", { name: "Profile" }).click();
  await page.getByRole("menuitem", { name: "Logout" }).click();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
};

const submitLogin = async (page, username, pass) => {
  await page.goto("/login");
  await page.getByRole("textbox", { name: "Username" }).fill(username);
  await page.getByRole("textbox", { name: "Password", exact: true }).fill(pass);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
};

test.describe("signup", () => {
  test.beforeEach(({ serverConfig }) => {
    test.skip(!serverConfig.enable_signup, "signup disabled");
  });

  test("sign up, log out and log back in", async ({ page, users }) => {
    const username = unique("user");
    users.track(username);
    await signup(page, username);

    // Account page and profile menu show the new user
    await page.goto("/account");
    await expect(page.getByText(username).first()).toBeVisible();
    await page.getByRole("button", { name: "Profile" }).click();
    await expect(page.getByRole("menuitem", { name: username })).toBeVisible();
    await page.keyboard.press("Escape");

    await logout(page);
    await login(page, username);
    await page.goto("/account");
    await expect(page.getByText(username).first()).toBeVisible();
  });

  test("taken username is rejected", async ({ page, users }) => {
    const username = await users.create();
    await page.goto("/signup");
    await page.getByRole("textbox", { name: "Username" }).fill(username);
    await page.getByRole("textbox", { name: "Password", exact: true }).fill(password);
    await page.getByRole("textbox", { name: "Confirm password" }).fill(password);
    await page.getByRole("button", { name: "Sign up" }).click();
    await expect(page.getByText(`Username ${username} is already taken`)).toBeVisible();
  });

  test("sign up button needs matching passwords", async ({ page }) => {
    await page.goto("/signup");
    await page.getByRole("textbox", { name: "Username" }).fill(unique("user"));
    await page.getByRole("textbox", { name: "Password", exact: true }).fill(password);
    await page.getByRole("textbox", { name: "Confirm password" }).fill("something else");
    await expect(page.getByRole("button", { name: "Sign up" })).toBeDisabled();
  });
});

test("wrong password is rejected", async ({ page, users }) => {
  const username = await users.create();
  await submitLogin(page, username, "wrong-password");
  await expect(page.getByText("Login failed: Invalid username/email or password")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("logged out users are sent away from the account page", async ({ page }) => {
  await page.goto("/account");
  await expect(page).not.toHaveURL(/\/account$/);
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
});

test("change password", async ({ page, users }) => {
  const username = await users.create();
  await login(page, username);
  await page.goto("/account");
  await page.getByRole("button", { name: "Change your account password" }).click();
  const dialog = page.getByRole("dialog", { name: "Change password" });

  // Wrong current password first
  await dialog.getByRole("textbox", { name: "Current password", exact: true }).fill("wrong-password");
  await dialog.getByRole("textbox", { name: "New password", exact: true }).fill("new-secret-123");
  await dialog.getByRole("textbox", { name: "Confirm password", exact: true }).fill("new-secret-123");
  await dialog.getByRole("button", { name: "Change password" }).click();
  await expect(dialog.getByText("Password incorrect")).toBeVisible();

  await dialog.getByRole("textbox", { name: "Current password", exact: true }).fill(password);
  await dialog.getByRole("button", { name: "Change password" }).click();
  await expect(dialog).toBeHidden();

  await logout(page);
  await submitLogin(page, username, password);
  await expect(page.getByText("Login failed: Invalid username/email or password")).toBeVisible();
  await login(page, username, "new-secret-123");
});

test("delete account", async ({ page, request, users }) => {
  const username = await users.create();
  await login(page, username);
  await page.goto("/account");
  await page.getByRole("button", { name: "Delete account" }).click();
  const dialog = page.getByRole("dialog", { name: "Delete account" });

  await dialog.getByRole("textbox", { name: "Password", exact: true }).fill("wrong-password");
  await dialog.getByRole("button", { name: "Permanently delete account" }).click();
  await expect(dialog.getByText("Password incorrect")).toBeVisible();

  await dialog.getByRole("textbox", { name: "Password", exact: true }).fill(password);
  await dialog.getByRole("button", { name: "Permanently delete account" }).click();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();

  const auth = Buffer.from(`${username}:${password}`).toString("base64");
  expect((await request.post("/v1/account/token", { headers: { Authorization: `Basic ${auth}` } })).status()).toBe(401);
});

test("account page shows usage", async ({ page, users }) => {
  await login(page, await users.create());
  await page.goto("/account");
  const usage = page.getByLabel("Usage", { exact: true });
  await expect(usage.getByText("Published messages")).toBeVisible();
  await expect(usage.getByText("Attachment storage")).toBeVisible();
});
