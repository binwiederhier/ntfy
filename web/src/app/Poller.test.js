import { beforeEach, describe, expect, it, vi } from "vitest";

// The poller stores through the real SubscriptionManager and database (an in-memory IndexedDB,
// see src/test/setup.js); only the server and the prefs are stubbed
vi.mock("./Api", () => ({ default: { poll: vi.fn() } }));
vi.mock("./Prefs", () => ({ default: { deleteAfter: vi.fn() } }));
vi.mock("./Notifier", () => ({ default: {} }));

const { default: api } = await import("./Api");
const { default: prefs } = await import("./Prefs");
const { default: subscriptionManager } = await import("./SubscriptionManager");
const { default: poller } = await import("./Poller");

const { db } = subscriptionManager;
const baseUrl = "https://ntfy.sh";
const subscriptionId = "https://ntfy.sh/mytopic";
const now = () => Math.round(Date.now() / 1000);

const message = (id, fields = {}) => ({ id, time: now(), event: "message", topic: "mytopic", message: `message ${id}`, ...fields });

// Polls once, with the server returning the given messages
const poll = async (messages) => {
  api.poll.mockResolvedValueOnce(messages);
  await poller.poll(await subscriptionManager.get(subscriptionId));
};

const stored = async () => (await db.notifications.toArray()).map((n) => n.message);

beforeEach(async () => {
  vi.spyOn(console, "log").mockImplementation(() => {});
  vi.clearAllMocks();
  prefs.deleteAfter.mockResolvedValue(0);
  await db.notifications.clear();
  await db.subscriptions.clear();
  await subscriptionManager.upsert(baseUrl, "mytopic");
});

describe("Poller.poll", () => {
  it("asks for messages since the last one and stores them", async () => {
    await poll([message("m1"), message("m2")]);
    expect(api.poll).toHaveBeenLastCalledWith(baseUrl, "mytopic", null);
    expect(await stored()).toEqual(["message m1", "message m2"]);

    await poll([]);
    expect(api.poll).toHaveBeenLastCalledWith(baseUrl, "mytopic", "m2");
  });

  it("stores only the latest version of a sequence", async () => {
    await poll([
      message("m1", { sequence_id: "s1", message: "Downloading", time: now() - 2 }),
      message("m2", { sequence_id: "s1", message: "Download complete", time: now() - 1 }),
    ]);
    expect(await stored()).toEqual(["Download complete"]);
  });

  it("replaces a stored message with its update", async () => {
    await subscriptionManager.addNotification(
      subscriptionId,
      message("m1", { sequence_id: "s1", message: "Downloading", time: now() - 1 }),
    );
    await poll([message("m2", { sequence_id: "s1", message: "Download complete" })]);
    expect(await stored()).toEqual(["Download complete"]);
  });

  it("keeps a stored message that is newer than the polled one", async () => {
    // The poll fetched the old version just before the update arrived via WebSocket
    await subscriptionManager.addNotification(subscriptionId, message("m2", { sequence_id: "s1", message: "Download complete" }));
    await poll([message("m1", { sequence_id: "s1", message: "Downloading", time: now() - 1 })]);
    expect(await stored()).toEqual(["Download complete"]);
  });

  it("removes a deleted sequence", async () => {
    await subscriptionManager.addNotification(subscriptionId, message("m1", { sequence_id: "s1" }));
    await subscriptionManager.addNotification(subscriptionId, message("m2", { sequence_id: "s2" }));
    await poll([message("d1", { sequence_id: "s1", event: "message_delete" })]);
    expect(await stored()).toEqual(["message m2"]);
  });

  it("does not store messages older than the auto-delete setting", async () => {
    prefs.deleteAfter.mockResolvedValue(3600);
    await poll([message("old", { time: now() - 7200 }), message("new")]);
    expect(await stored()).toEqual(["message new"]);
  });
});
