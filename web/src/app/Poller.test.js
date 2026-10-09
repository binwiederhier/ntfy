import { beforeEach, describe, expect, it, vi } from "vitest";
import { UnauthorizedError } from "./errors";

// Poller.js pulls in a handful of Dexie-backed singletons (Prefs, SubscriptionManager, Session)
// at import time; mock them wholesale so the module imports cleanly under the node test
// environment, same approach as SubscriptionManager.test.js and Prefs.test.js. config.js is left
// real: test/setup.js stubs window.config with base_url "https://ntfy.sh", same as every other
// test file.
vi.mock("./Api", () => ({ default: { poll: vi.fn() } }));
vi.mock("./Prefs", () => ({ default: { deleteAfter: vi.fn() } }));
vi.mock("./SubscriptionManager", () => ({ default: { all: vi.fn(), deleteNotificationBySequenceId: vi.fn(), addNotifications: vi.fn() } }));
vi.mock("./Session", () => ({ default: { resetAndRedirect: vi.fn(), exists: vi.fn() } }));
vi.mock("../components/routes", () => ({ default: { login: "/login" } }));

const api = (await import("./Api")).default;
const subscriptionManager = (await import("./SubscriptionManager")).default;
const session = (await import("./Session")).default;
const routes = (await import("../components/routes")).default;
const poller = (await import("./Poller")).default;

// Matches the config.base_url stub in test/setup.js, i.e. the user's own account/home server.
const homeSubscription = { id: "sub1", baseUrl: "https://ntfy.sh", topic: "mytopic", last: 0 };
const otherServerSubscription = { id: "sub2", baseUrl: "https://example.com", topic: "othertopic", last: 0 };

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, "log").mockImplementation(() => {});
  session.exists.mockReturnValue(true);
});

describe("Poller.pollAll", () => {
  it("ends the session and redirects to login on a 401 (bad credentials) from the home server", async () => {
    subscriptionManager.all.mockResolvedValue([homeSubscription]);
    api.poll.mockRejectedValue(new UnauthorizedError(401));

    await poller.pollAll();

    expect(session.resetAndRedirect).toHaveBeenCalledWith(routes.login);
  });

  it("does not touch the session for a non-auth polling error", async () => {
    subscriptionManager.all.mockResolvedValue([homeSubscription]);
    api.poll.mockRejectedValue(new Error("network down"));

    await poller.pollAll();

    expect(session.resetAndRedirect).not.toHaveBeenCalled();
  });

  it("does not touch the session for a home-server 403 (valid session, topic not authorized)", async () => {
    // 403 from authorizeTopic (server/server_middleware.go) means the credentials were fine but
    // this particular topic isn't -- the account session itself is not dead, so it must not be
    // wiped. Only a 401 (bad/expired credentials, from maybeAuthenticate) means that.
    subscriptionManager.all.mockResolvedValue([homeSubscription]);
    api.poll.mockRejectedValue(new UnauthorizedError(403));

    await poller.pollAll();

    expect(session.resetAndRedirect).not.toHaveBeenCalled();
  });

  it("does not touch the account session for a 401 from another server", async () => {
    // That server's own stored credentials are bad, not the account session (see UserManager.get):
    // wiping the session and bouncing everyone to /login over one other-server subscription would
    // be wrong.
    subscriptionManager.all.mockResolvedValue([otherServerSubscription]);
    api.poll.mockRejectedValue(new UnauthorizedError(401));

    await poller.pollAll();

    expect(session.resetAndRedirect).not.toHaveBeenCalled();
  });

  it("does not touch the session for a home-server 401 when there is no session", async () => {
    // No session means the poll wasn't authenticated with the account token in the first place
    // (see UserManager.get), so there's no session to invalidate.
    session.exists.mockReturnValue(false);
    subscriptionManager.all.mockResolvedValue([homeSubscription]);
    api.poll.mockRejectedValue(new UnauthorizedError(401));

    await poller.pollAll();

    expect(session.resetAndRedirect).not.toHaveBeenCalled();
  });
});
