import { test, expect, subscribe } from "./fixtures.js";
import { unique } from "./env.js";

let topic;

// The last message on the topic, as stored by the server
const lastMessage = async (request, t = topic) => {
  const lines = (await (await request.get(`/${t}/json?poll=1`)).text()).trim().split("\n");
  return JSON.parse(lines.pop());
};

const openDialog = async (page) => {
  await page.getByRole("button", { name: "Show publish dialog" }).click();
  return page.getByRole("dialog");
};

test.beforeEach(async ({ page }) => {
  topic = unique("pub");
  await page.goto("/app");
  await subscribe(page, topic);
});

test("publish from the message bar", async ({ page, request }) => {
  const input = page.getByPlaceholder("Type a message here");
  await input.fill("hello from the message bar");
  await input.press("Enter");
  await expect(page.getByText("hello from the message bar")).toBeVisible();
  await expect(input).toHaveValue("");

  await input.fill("sent with the button");
  await page.getByRole("button", { name: "Publish message" }).click();
  await expect(page.getByText("sent with the button")).toBeVisible();
  expect((await lastMessage(request)).message).toBe("sent with the button");
});

test("publish dialog with title, tags and priority", async ({ page, request }) => {
  const dialog = await openDialog(page);
  await dialog.getByRole("textbox", { name: "Title" }).fill("Dialog title");
  await dialog.getByRole("textbox", { name: "Message" }).fill("from the publish dialog");
  await dialog.getByRole("textbox", { name: "Tags" }).fill("warning, srv1");
  await dialog.getByRole("combobox", { name: "Priority" }).click();
  await page.getByRole("option", { name: "Priority 5" }).click();
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText("from the publish dialog")).toBeVisible();
  expect(await lastMessage(request)).toMatchObject({
    title: "Dialog title",
    message: "from the publish dialog",
    priority: 5,
    tags: ["warning", "srv1"],
  });
});

test("emoji picker adds a tag", async ({ page, request }) => {
  const dialog = await openDialog(page);
  await dialog.getByRole("textbox", { name: "Message" }).fill("with an emoji");
  await dialog.getByRole("button", { name: "Pick emoji" }).click();
  await page.getByRole("searchbox", { name: "Search emoji" }).fill("tada");
  await page
    .getByLabel(/\(tada\)$/)
    .first()
    .click();
  await expect(dialog.getByRole("textbox", { name: "Tags" })).toHaveValue("tada");
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog).toBeHidden();
  expect((await lastMessage(request)).tags).toEqual(["tada"]);
});

test("markdown, click URL and attachment URL", async ({ page, request }) => {
  const dialog = await openDialog(page);
  await dialog.getByRole("textbox", { name: "Message" }).fill("**bold** text");
  await dialog.getByRole("checkbox", { name: "Format as Markdown" }).check();
  await dialog.getByRole("button", { name: "Click URL" }).click();
  await dialog.getByRole("textbox", { name: "Click URL" }).fill("https://example.com/click");
  await dialog.getByRole("button", { name: "Attach file by URL" }).click();
  await dialog.getByRole("textbox", { name: "Attachment URL" }).fill("https://example.com/files/app.apk");
  await expect(dialog.getByRole("textbox", { name: "Filename" })).toHaveValue("app.apk");
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.locator("strong", { hasText: "bold" })).toBeVisible();
  expect(await lastMessage(request)).toMatchObject({
    content_type: "text/markdown",
    click: "https://example.com/click",
    attachment: { name: "app.apk", url: "https://example.com/files/app.apk" },
  });
});

test("attach a local file", async ({ page, request }) => {
  const dialog = await openDialog(page);
  const chooser = page.waitForEvent("filechooser");
  await dialog.getByRole("button", { name: "Attach local file" }).click();
  await (await chooser).setFiles({ name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("some notes") });
  await expect(dialog.getByRole("textbox", { name: "Attachment filename" })).toHaveValue("notes.txt");
  await dialog.getByRole("textbox", { name: "Message" }).fill("file attached");
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("listitem", { name: "Notification" }).getByText("notes.txt")).toBeVisible();
  const message = await lastMessage(request);
  expect(message).toMatchObject({ message: "file attached", attachment: { name: "notes.txt", size: 10 } });
  expect(await (await request.get(message.attachment.url)).text()).toBe("some notes");
});

test("publish another keeps the dialog open", async ({ page, request }) => {
  const dialog = await openDialog(page);
  await dialog.getByRole("checkbox", { name: "Publish another" }).check();
  await dialog.getByRole("textbox", { name: "Message" }).fill("first of many");
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog.getByText("Notification published")).toBeVisible();
  await expect(dialog).toBeVisible();
  expect((await lastMessage(request)).message).toBe("first of many");
});

test("publish to another topic from the sidebar", async ({ page, request }) => {
  // The dialog is preset to the selected topic until "Change topic" is clicked
  const other = unique("pubother");
  await page.getByRole("button", { name: "Publish notification" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Change topic" }).click();
  await dialog.getByRole("textbox", { name: "Topic name" }).fill(other);
  await dialog.getByRole("textbox", { name: "Message" }).fill("to an unsubscribed topic");
  await dialog.getByRole("button", { name: "Send" }).click();
  await expect(dialog).toBeHidden();
  expect((await lastMessage(request, other)).message).toBe("to an unsubscribed topic");
});

test.describe("delayed delivery", () => {
  // Natural-language delays ("tomorrow 10am") must resolve in the browser's timezone, not the server's
  test.use({ timezoneId: "America/New_York" });

  test("uses the browser timezone", async ({ page, request }) => {
    const dialog = await openDialog(page);
    await dialog.getByRole("textbox", { name: "Message", exact: true }).fill("scheduled message");
    await dialog.getByRole("button", { name: "Delay delivery", exact: true }).click();
    await dialog.getByLabel("Delay", { exact: true }).fill("tomorrow 10am");

    const expected = await page.evaluate(() => {
      const d = new Date();
      d.setDate(d.getDate() + 1);
      d.setHours(10, 0, 0, 0);
      return Math.floor(d.getTime() / 1000);
    });
    const published = page.waitForResponse((r) => r.request().method() === "PUT" && r.url().includes(`/${topic}?`));
    await dialog.getByRole("button", { name: "Send", exact: true }).click();
    const response = await published;
    expect(response.status()).toBe(200);
    expect(new URL(response.url()).searchParams.get("timezone")).toBe("America/New_York");
    const message = await response.json();
    expect(message.time).toBe(expected);
    await expect(dialog).toBeHidden();

    // Scheduled, so only visible when asking for scheduled messages
    const scheduled = await (await request.get(`/${topic}/json?poll=1&scheduled=1`)).text();
    expect(scheduled).toContain(message.id);
    const delivered = await (await request.get(`/${topic}/json?poll=1`)).text();
    expect(delivered).not.toContain(message.id);
  });
});
