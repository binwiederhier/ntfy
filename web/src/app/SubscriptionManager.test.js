import { beforeEach, describe, expect, it, vi } from "vitest";

// SubscriptionManager pulls in a handful of browser/Dexie-heavy singletons at import time. Mock
// them so the module imports cleanly under the node test environment; the tests construct their
// own SubscriptionManager with an in-memory fake db, so the real db singleton is never used.
vi.mock("./Api", () => ({ default: {} }));
vi.mock("./Notifier", () => ({ default: {} }));
vi.mock("./Prefs", () => ({ default: {} }));
vi.mock("./db", () => ({ default: () => ({}) }));

const { SubscriptionManager } = await import("./SubscriptionManager");

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

describe("SubscriptionManager.notificationStateFromBackend", () => {
  it("reads notification state through native IndexedDB requests", async () => {
    const request = (value) => {
      const indexedDbRequest = {};
      queueMicrotask(() => {
        indexedDbRequest.result = value;
        indexedDbRequest.onsuccess();
      });
      return indexedDbRequest;
    };
    const notifications = {
      count: () => request(4),
      index: (name) => {
        if (name === "new") {
          return { count: () => request(2) };
        }
        return { openCursor: () => request({ value: { id: "newest" } }) };
      },
    };
    const db = {
      open: vi.fn(),
      backendDB: () => ({
        transaction: () => ({ objectStore: () => notifications }),
      }),
    };
    const manager = new SubscriptionManager(db);

    await expect(manager.notificationStateFromBackend()).resolves.toEqual({ count: 4, unreadCount: 2, latestId: "newest" });
    expect(db.open).toHaveBeenCalledOnce();
  });
});
