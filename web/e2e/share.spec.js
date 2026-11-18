import { test, expect } from "./fixtures.js";
import { unique } from "./env.js";

// The installed PWA's manifest tells the operating system to submit this form.
const share = async (page, fields) => {
  await page.goto("/app");
  await page.evaluate((fields) => {
    const form = document.createElement("form");
    form.action = "/v1/web/share";
    form.method = "POST";
    for (const [name, value] of Object.entries(fields)) {
      const input = document.createElement("input");
      input.type = "hidden";
      input.name = name;
      input.value = value;
      form.append(input);
    }
    document.body.append(form);
    form.submit();
  }, fields);
  return page.getByRole("dialog");
};

test("shared text and a link are an editable draft until sent", async ({ page, request }) => {
  const title = "Café + notes";
  const url = "https://example.com/docs?a=1&b=2#details";
  const dialog = await share(page, { title, text: "Read this", url });
  await expect(dialog.getByRole("textbox", { name: "Title" })).toHaveValue(title);
  const message = dialog.getByRole("textbox", { name: "Message", exact: true });
  await expect(message).toHaveValue(`Read this\n\n${url}`);
  await expect(page).not.toHaveURL(/#share/);
  const send = dialog.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();

  const topic = unique("share");
  await dialog.getByRole("textbox", { name: "Topic name" }).fill(topic);
  await message.fill(`Read this later\n\n${url}`);
  await send.click();
  await expect(dialog).toBeHidden();

  const response = await request.get(`/${topic}/json?poll=1&since=all`);
  expect(await response.json()).toMatchObject({ title, message: `Read this later\n\n${url}` });
});

test("cancelling a share clears the draft", async ({ page }) => {
  const url = "https://example.com/docs";
  const dialog = await share(page, { text: url, url });
  await expect(dialog.getByRole("textbox", { name: "Message", exact: true })).toHaveValue(url);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toBeHidden();

  await page.getByRole("button", { name: "Publish notification" }).click();
  await expect(dialog.getByRole("textbox", { name: "Message", exact: true })).toHaveValue("");
  await page.reload();
  await expect(dialog).toBeHidden();
});
