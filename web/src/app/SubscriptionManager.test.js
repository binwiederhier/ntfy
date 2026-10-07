import { beforeEach, describe, expect, it, vi } from "vitest";

// SubscriptionManager pulls in a handful of browser/Dexie-heavy singletons at import time. Mock
// them so the module imports cleanly under the node test environment; the tests construct their
// own SubscriptionManager with an in-memory fake db, so the real db singleton is never used.
vi.mock("./Api", () => ({ default: {} }));
vi.mock("./Notifier", () => ({ default: {} }));
vi.mock("./Prefs", () => ({ default: {} }));
vi.mock("./db", () => ({ default: () => ({}) }));

const { SubscriptionManager } = await import("./SubscriptionManager");
const { default: createDb } = await vi.importActual("./db");

// Minimal in-memory stand-in for the Dexie "subscriptions" table, implementing just the surface
// that syncFromRemote() (and the upsert/remove/update helpers it calls) touches.
const fakeDb = () => {
  const rows = new Map();
  return {
    rows,
    subscriptions: {
      get: async (id) => rows.get(id),
      put: async (sub) => {
        rows.set(sub.id, sub);
      },
      update: async (id, changes) => {
        const existing = rows.get(id);
        if (existing) {
          rows.set(id, { ...existing, ...changes });
        }
      },
      delete: async (id) => {
        rows.delete(id);
      },
      toArray: async () => Array.from(rows.values()),
    },
  };
};

const baseUrl = "https://ntfy.sh";

beforeEach(() => {
  vi.spyOn(console, "log").mockImplementation(() => {});
});

describe("SubscriptionManager.upsert", () => {
  it("merges fields into an existing subscription without clobbering local-only state", async () => {
    const db = fakeDb();
    const manager = new SubscriptionManager(db);

    await manager.upsert(baseUrl, "mytopic");
    await manager.setMutedUntil("https://ntfy.sh/mytopic", 123);

    const reservation = { topic: "mytopic", everyone: "deny-all" };
    await manager.upsert(baseUrl, "mytopic", { displayName: "My Topic", reservation });

    const stored = db.rows.get("https://ntfy.sh/mytopic");
    expect(stored.reservation).toEqual(reservation);
    expect(stored.displayName).toBe("My Topic");
    expect(stored.mutedUntil).toBe(123); // local-only state preserved
  });

  it("does not write when an existing subscription would not change", async () => {
    const db = fakeDb();
    const manager = new SubscriptionManager(db);

    await manager.upsert(baseUrl, "mytopic", { internal: true });
    const putSpy = vi.spyOn(db.subscriptions, "put");

    const result = await manager.upsert(baseUrl, "mytopic", { internal: true });

    expect(putSpy).not.toHaveBeenCalled();
    expect(result.topic).toBe("mytopic");
  });
});

describe("SubscriptionManager.syncFromRemote", () => {
  it("persists a reservation onto a subscription that already exists locally", async () => {
    const db = fakeDb();
    const manager = new SubscriptionManager(db);

    // Topic was subscribed to before it was reserved, so it already exists locally without a
    // reservation -- exactly the state when a user clicks "Reserve topic" in the navbar.
    await manager.upsert(baseUrl, "mytopic");
    expect(db.rows.get("https://ntfy.sh/mytopic").reservation).toBeFalsy();

    const reservation = { topic: "mytopic", everyone: "deny-all" };
    await manager.syncFromRemote([{ base_url: baseUrl, topic: "mytopic" }], [reservation]);

    expect(db.rows.get("https://ntfy.sh/mytopic").reservation).toEqual(reservation);
  });

  it("clears the reservation when the remote no longer reports one", async () => {
    const db = fakeDb();
    const manager = new SubscriptionManager(db);

    await manager.upsert(baseUrl, "mytopic", { reservation: { topic: "mytopic", everyone: "deny-all" } });

    await manager.syncFromRemote([{ base_url: baseUrl, topic: "mytopic" }], []);

    expect(db.rows.get("https://ntfy.sh/mytopic").reservation).toBeNull();
  });

  it("updates the display name on an existing subscription", async () => {
    const db = fakeDb();
    const manager = new SubscriptionManager(db);

    await manager.upsert(baseUrl, "mytopic");
    await manager.syncFromRemote([{ base_url: baseUrl, topic: "mytopic", display_name: "My Topic" }], []);

    expect(db.rows.get("https://ntfy.sh/mytopic").displayName).toBe("My Topic");
  });
});

// The notification paths depend on Dexie transactions and indexes, so they run against the real
// schema on an in-memory IndexedDB (fake-indexeddb), wiped before each test
const realDb = async () => {
  await createDb().delete();
  return createDb();
};

const message = (id, fields = {}) => ({ id, time: 1700000000, event: "message", topic: "mytopic", message: `message ${id}`, ...fields });

const subscriptionId = "https://ntfy.sh/mytopic";

const setup = async () => {
  const db = await realDb();
  const manager = new SubscriptionManager(db);
  await manager.upsert(baseUrl, "mytopic");
  return { db, manager };
};

describe("SubscriptionManager.addNotification", () => {
  it("stores a new message as unread and remembers it as the last one", async () => {
    const { db, manager } = await setup();

    expect(await manager.addNotification(subscriptionId, message("m1", { sequence_id: "s1" }))).toBe(true);

    expect(await db.notifications.get("m1")).toMatchObject({ subscriptionId, sequenceId: "s1", new: 1, message: "message m1" });
    expect((await db.subscriptions.get(subscriptionId)).last).toBe("m1");
  });

  it("uses the message ID as sequence ID if there is none", async () => {
    const { db, manager } = await setup();
    await manager.addNotification(subscriptionId, message("m1"));
    expect((await db.notifications.get("m1")).sequenceId).toBe("m1");
  });

  it("leaves a stored message untouched and returns false", async () => {
    const { db, manager } = await setup();
    await manager.addNotification(subscriptionId, message("m1"));
    await manager.markNotificationRead("m1");
    await manager.addNotification(subscriptionId, message("m2"));

    // Delivered again, e.g. by a second tab or a reconnect
    expect(await manager.addNotification(subscriptionId, message("m1", { message: "changed" }))).toBe(false);

    expect(await db.notifications.get("m1")).toMatchObject({ new: 0, message: "message m1" });
    expect((await db.subscriptions.get(subscriptionId)).last).toBe("m2");
  });

  it("does not store delete and clear events", async () => {
    const { db, manager } = await setup();
    expect(await manager.addNotification(subscriptionId, message("d1", { event: "message_delete", sequence_id: "s1" }))).toBe(false);
    expect(await manager.addNotification(subscriptionId, message("c1", { event: "message_clear", sequence_id: "s1" }))).toBe(false);
    expect(await db.notifications.count()).toBe(0);
    expect((await db.subscriptions.get(subscriptionId)).last).toBeNull();
  });

  it("adds a message delivered twice at the same time only once", async () => {
    const { db, manager } = await setup();
    const results = await Promise.all([
      manager.addNotification(subscriptionId, message("m1")),
      manager.addNotification(subscriptionId, message("m1")),
    ]);
    expect(results.sort()).toEqual([false, true]);
    expect(await db.notifications.count()).toBe(1);
  });
});

describe("SubscriptionManager.addNotifications", () => {
  it("stores polled messages as read and remembers the last one", async () => {
    const { db, manager } = await setup();

    await manager.addNotifications(subscriptionId, [message("m1"), message("m2", { sequence_id: "s2" })]);

    const stored = await db.notifications.toArray();
    expect(stored.map((n) => [n.id, n.sequenceId, n.subscriptionId, n.new])).toEqual([
      ["m1", "m1", subscriptionId, undefined],
      ["m2", "s2", subscriptionId, undefined],
    ]);
    expect((await db.subscriptions.get(subscriptionId)).last).toBe("m2");
  });

  it("keeps the local state of messages that are already stored", async () => {
    const { db, manager } = await setup();
    await manager.addNotification(subscriptionId, message("unread"));
    await manager.addNotification(subscriptionId, message("read"));
    await manager.markNotificationRead("read");
    const pinged = message("pinged", { actions: [{ id: "a1", action: "http", label: "Ping" }] });
    await manager.addNotification(subscriptionId, pinged);
    await manager.updateNotification({ ...(await db.notifications.get("pinged")), actions: [{ ...pinged.actions[0], progress: 2 }] });

    // A poll returns all of them again, plus one that is new
    await manager.addNotifications(subscriptionId, [message("unread"), message("read"), pinged, message("polled")]);

    expect((await db.notifications.get("unread")).new).toBe(1);
    expect((await db.notifications.get("read")).new).toBe(0);
    expect((await db.notifications.get("pinged")).actions[0].progress).toBe(2);
    expect(await db.notifications.get("polled")).toMatchObject({ subscriptionId, sequenceId: "polled" });
    expect((await db.subscriptions.get(subscriptionId)).last).toBe("polled");
  });

  it("replaces an older version of the same sequence, keeping it unread", async () => {
    const { db, manager } = await setup();
    await manager.addNotification(subscriptionId, message("m1", { sequence_id: "s1", message: "Downloading" }));
    await manager.addNotification(subscriptionId, message("o1", { sequence_id: "other", message: "Other" }));

    // The WebSocket missed the update, so only the poll sees it
    await manager.addNotifications(subscriptionId, [message("m2", { sequence_id: "s1", message: "Download complete", time: 1700000001 })]);

    const versions = await db.notifications.where({ subscriptionId, sequenceId: "s1" }).toArray();
    expect(versions.map((n) => [n.message, n.new])).toEqual([["Download complete", 1]]);
    expect(await db.notifications.get("o1")).toBeDefined();
  });

  it("does not replace a newer version of the same sequence", async () => {
    const { db, manager } = await setup();
    await manager.addNotification(subscriptionId, message("m2", { sequence_id: "s1", message: "Download complete", time: 1700000001 }));

    // The poll fetched the old version just before the update arrived via WebSocket
    await manager.addNotifications(subscriptionId, [message("m1", { sequence_id: "s1", message: "Downloading" })]);

    expect((await db.notifications.toArray()).map((n) => n.message)).toEqual(["Download complete"]);
  });
});
