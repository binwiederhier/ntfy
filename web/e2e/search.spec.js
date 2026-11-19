import { test, expect, subscribe } from "./fixtures.js";
import { unique } from "./env.js";

let topic;

test.beforeEach(async ({ page }) => {
  topic = unique("search");
  await page.goto("/app");
  await subscribe(page, topic);
});

test("search matches titles, bodies and tags, and can be cleared", async ({ page, request }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await request.post(`/${topic}`, { headers: { Title: "Nightly Backup", Tags: "archive" }, data: "Job finished" });
  await request.post(`/${topic}`, { data: "Backup failed on host two" });
  await request.post(`/${topic}`, { data: "Unrelated notification" });
  const items = page.getByRole("listitem", { name: "Notification" });
  await expect(items).toHaveCount(3);
  const search = page.getByRole("searchbox", { name: "Search notifications" });
  await search.fill("  BACKUP  ");
  await expect(items).toHaveCount(2);
  await expect(items.filter({ hasText: "Job finished" })).toBeVisible();
  await expect(items.filter({ hasText: "Backup failed on host two" })).toBeVisible();

  await search.fill("archive");
  await expect(items).toHaveCount(1);
  await expect(items).toContainText("Job finished");
  await search.fill("no match");
  await expect(items).toHaveCount(0);
  await expect(page.getByRole("status")).toHaveText("No matching notifications.");
  await page.getByRole("button", { name: "Clear search" }).click();
  await expect(search).toBeFocused();
  await expect(items).toHaveCount(3);
  await search.fill("no match");
  await search.press("Escape");
  await expect(search).toHaveValue("");
  await expect(items).toHaveCount(3);
});

test("search includes notifications beyond the first page", async ({ page, request }) => {
  await request.post(`/${topic}`, { data: "Old searchable message" });
  // Make this message older than every page of recent messages.
  await new Promise((resolve) => setTimeout(resolve, 1100));
  for (let i = 0; i < 24; i += 1) {
    await request.post(`/${topic}`, { data: `Recent message ${i}` });
  }
  const items = page.getByRole("listitem", { name: "Notification" });
  await expect(items).toHaveCount(20);
  await expect(items.filter({ hasText: "Old searchable message" })).toHaveCount(0);
  const search = page.getByRole("searchbox", { name: "Search notifications" });
  await search.fill("old searchable");
  await expect(items).toHaveCount(1);
  await expect(items).toContainText("Old searchable message");
  await search.fill("recent message");
  await expect(items).toHaveCount(20);
  await page.locator("#main").evaluate((main) => main.scrollTo(0, main.scrollHeight));
  await expect(items).toHaveCount(24);
  await search.fill("recent message 23");
  await expect(items).toHaveCount(1);
  await page.getByRole("button", { name: "Clear search" }).click();
  await expect(items).toHaveCount(20);
});

test("All notifications searches across topics and updates for live messages", async ({ page, request }) => {
  await request.post(`/${topic}`, { data: "Search match from topic one" });
  await expect(page.getByText("Search match from topic one")).toBeVisible();
  const other = unique("searchother");
  await subscribe(page, other);
  await request.post(`/${other}`, { data: "Search match from topic two" });
  await expect(page.getByText("Search match from topic two")).toBeVisible();
  const search = page.getByRole("searchbox", { name: "Search notifications" });
  const items = page.getByRole("listitem", { name: "Notification" });
  await search.fill("search match");
  await expect(items).toHaveCount(1);
  await expect(items).toContainText("topic two");
  await page.getByRole("button", { name: "All notifications", exact: true }).click();
  await search.fill("search match");
  await expect(items).toHaveCount(2);
  await request.post(`/${topic}`, { data: "Search match arriving live" });
  await expect(items).toHaveCount(3);
  await items.filter({ hasText: "arriving live" }).getByRole("button", { name: "Delete", exact: true }).click();
  await expect(items).toHaveCount(2);
  await page.getByRole("button", { name: topic, exact: true }).click();
  await expect(search).toHaveValue("");
  await expect(items).toHaveCount(1);
});
