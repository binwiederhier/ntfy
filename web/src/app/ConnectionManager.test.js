import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Connection from "./Connection";

// Keep refresh() and its connection identity/hash real; only stub the WebSocket-owning class.
vi.mock("./Connection", () => ({
  default: vi.fn(function () {
    this.start = vi.fn();
    this.close = vi.fn();
  }),
}));

const baseUrl = "https://server-a.example.com";
const subscription = { id: `${baseUrl}/mytopic`, baseUrl, topic: "mytopic", last: "previous-message" };
const user = { baseUrl, username: "test-user", password: "old-test-password" };
let connectionManager;

beforeEach(async () => {
  // The module exports a singleton, so import a fresh one for each test.
  vi.resetModules();
  vi.clearAllMocks();
  vi.spyOn(console, "log").mockImplementation(() => {});
  ({ default: connectionManager } = await import("./ConnectionManager"));
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ConnectionManager.refresh", () => {
  it("replaces the connection when the password changes", async () => {
    await connectionManager.refresh([subscription], [user]);

    expect(Connection).toHaveBeenCalledTimes(1);
    expect(Connection).toHaveBeenLastCalledWith(
      expect.any(Number),
      subscription.id,
      baseUrl,
      subscription.topic,
      user,
      subscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [oldConnection] = Connection.mock.instances;
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(oldConnection.close).not.toHaveBeenCalled();

    const updatedUser = { ...user, password: "new-test-password" };
    await connectionManager.refresh([subscription], [updatedUser]);

    expect(Connection).toHaveBeenCalledTimes(2);
    expect(Connection).toHaveBeenLastCalledWith(
      expect.any(Number),
      subscription.id,
      baseUrl,
      subscription.topic,
      updatedUser,
      subscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [, replacement] = Connection.mock.instances;
    expect(oldConnection.close).toHaveBeenCalledTimes(1);
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(replacement.start).toHaveBeenCalledTimes(1);
    expect(replacement.close).not.toHaveBeenCalled();
    expect([...connectionManager.connections.values()]).toEqual([replacement]);
  });

  it("replaces the connection when the username changes", async () => {
    await connectionManager.refresh([subscription], [user]);
    const [oldConnection] = Connection.mock.instances;

    const updatedUser = { ...user, username: "other-test-user" };
    await connectionManager.refresh([subscription], [updatedUser]);

    expect(Connection).toHaveBeenCalledTimes(2);
    expect(Connection).toHaveBeenLastCalledWith(
      expect.any(Number),
      subscription.id,
      baseUrl,
      subscription.topic,
      updatedUser,
      subscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [, replacement] = Connection.mock.instances;
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(oldConnection.close).toHaveBeenCalledTimes(1);
    expect(replacement.start).toHaveBeenCalledTimes(1);
    expect(replacement.close).not.toHaveBeenCalled();
    expect([...connectionManager.connections.values()]).toEqual([replacement]);
  });

  it("replaces the connection when the token changes", async () => {
    const tokenUser = { baseUrl, token: "tk_test_old" };
    await connectionManager.refresh([subscription], [tokenUser]);
    const [oldConnection] = Connection.mock.instances;

    const updatedUser = { ...tokenUser, token: "tk_test_new" };
    await connectionManager.refresh([subscription], [updatedUser]);

    expect(Connection).toHaveBeenCalledTimes(2);
    expect(Connection).toHaveBeenLastCalledWith(
      expect.any(Number),
      subscription.id,
      baseUrl,
      subscription.topic,
      updatedUser,
      subscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [, replacement] = Connection.mock.instances;
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(oldConnection.close).toHaveBeenCalledTimes(1);
    expect(replacement.start).toHaveBeenCalledTimes(1);
    expect(replacement.close).not.toHaveBeenCalled();
    expect([...connectionManager.connections.values()]).toEqual([replacement]);
  });

  it("reuses the connection when username and password values are unchanged", async () => {
    await connectionManager.refresh([subscription], [user]);
    const [connection] = Connection.mock.instances;

    await connectionManager.refresh([subscription], [user]);
    await connectionManager.refresh([subscription], [{ ...user }]);

    expect(Connection).toHaveBeenCalledTimes(1);
    expect(connection.start).toHaveBeenCalledTimes(1);
    expect(connection.close).not.toHaveBeenCalled();
    expect([...connectionManager.connections.values()]).toEqual([connection]);
  });

  it("reuses the connection when the token value is unchanged", async () => {
    const tokenUser = { baseUrl, token: "tk_test_unchanged" };
    await connectionManager.refresh([subscription], [tokenUser]);
    const [connection] = Connection.mock.instances;

    await connectionManager.refresh([subscription], [tokenUser]);
    await connectionManager.refresh([subscription], [{ ...tokenUser }]);

    expect(Connection).toHaveBeenCalledTimes(1);
    expect(connection.start).toHaveBeenCalledTimes(1);
    expect(connection.close).not.toHaveBeenCalled();
    expect([...connectionManager.connections.values()]).toEqual([connection]);
  });

  it("preserves server B's connection when server A's credentials change", async () => {
    const otherBaseUrl = "https://server-b.example.com";
    const otherSubscription = { id: `${otherBaseUrl}/other-topic`, baseUrl: otherBaseUrl, topic: "other-topic", last: "other-message" };
    const otherUser = { baseUrl: otherBaseUrl, username: "other-test-user", password: "other-test-password" };
    const subscriptions = [subscription, otherSubscription];
    await connectionManager.refresh(subscriptions, [user, otherUser]);

    expect(Connection).toHaveBeenCalledTimes(2);
    expect(Connection).toHaveBeenNthCalledWith(
      2,
      expect.any(Number),
      otherSubscription.id,
      otherBaseUrl,
      otherSubscription.topic,
      otherUser,
      otherSubscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [oldConnection, otherConnection] = Connection.mock.instances;
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(otherConnection.start).toHaveBeenCalledTimes(1);

    const updatedUser = { ...user, password: "new-test-password" };
    await connectionManager.refresh(subscriptions, [updatedUser, { ...otherUser }]);

    expect(Connection).toHaveBeenCalledTimes(3);
    expect(Connection).toHaveBeenLastCalledWith(
      expect.any(Number),
      subscription.id,
      baseUrl,
      subscription.topic,
      updatedUser,
      subscription.last,
      expect.any(Function),
      expect.any(Function),
    );
    const [, , replacement] = Connection.mock.instances;
    expect(oldConnection.start).toHaveBeenCalledTimes(1);
    expect(oldConnection.close).toHaveBeenCalledTimes(1);
    expect(replacement.start).toHaveBeenCalledTimes(1);
    expect(replacement.close).not.toHaveBeenCalled();
    expect(otherConnection.start).toHaveBeenCalledTimes(1);
    expect(otherConnection.close).not.toHaveBeenCalled();
    expect(connectionManager.connections.size).toBe(2);
    expect([...connectionManager.connections.values()]).toEqual(expect.arrayContaining([replacement, otherConnection]));
  });
});
