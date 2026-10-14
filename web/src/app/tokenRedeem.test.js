import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import session from "./Session";
import { EnrollCodeInvalidError } from "./errors";
import { parseTokenRedeemFragment, redeemLink, RedeemResult, SignedInAsOtherUserError } from "./tokenRedeem";

vi.mock("i18next", () => ({ default: { changeLanguage: vi.fn() } }));

let fetchMock;

const respond = (status, body = {}) => ({ status, json: async () => body });

// Routes the fake server's answers by endpoint, so each test states only what differs
const serve = (handlers) => {
  fetchMock.mockImplementation(async (url) => {
    const path = new URL(url).pathname;
    const handler = handlers[path];
    if (!handler) {
      throw new Error(`unexpected request to ${path}`);
    }
    return handler();
  });
};

const requestsTo = (path) => fetchMock.mock.calls.filter(([url]) => new URL(url).pathname === path);

beforeEach(async () => {
  vi.spyOn(console, "log").mockImplementation(() => {});
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  localStorage.clear();
  await session.db.kv.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("parseTokenRedeemFragment", () => {
  it("reads the code, user, and topics, dropping topics the app could not subscribe to", () => {
    expect(parseTokenRedeemFragment("#code=abc123&user=phil&subscribe=alerts,phil-inbox,bad/topic,app,")).toEqual({
      code: "abc123",
      user: "phil",
      topics: ["alerts", "phil-inbox"],
    });
  });

  it("treats a missing fragment as an empty link", () => {
    expect(parseTokenRedeemFragment("")).toEqual({ code: "", user: "", topics: [] });
  });
});

describe("redeemLink", () => {
  it("signs in with the code, then subscribes the new account to each topic", async () => {
    serve({
      "/v1/account/token/redeem": () => respond(200, { token: "tk_new", username: "phil" }),
      "/v1/account/subscription": () => respond(200, {}),
    });

    const result = await redeemLink({ code: "abc123", user: "phil", topics: ["alerts", "phil-inbox"] });

    expect(result).toBe(RedeemResult.SIGNED_IN);
    expect(session.username()).toBe("phil");
    expect(session.token()).toBe("tk_new");
    expect(await session.usernameAsync()).toBe("phil"); // The service worker reads the IndexedDB replica

    const [[, redeem]] = requestsTo("/v1/account/token/redeem");
    expect(redeem.method).toBe("POST");
    expect(redeem.headers).toBeUndefined(); // The code is the credential
    expect(JSON.parse(redeem.body)).toEqual({ code: "abc123", label: "Web app" });

    const subscriptions = requestsTo("/v1/account/subscription");
    expect(subscriptions.map(([, options]) => JSON.parse(options.body).topic)).toEqual(["alerts", "phil-inbox"]);
    subscriptions.forEach(([, options]) => expect(options.headers.Authorization).toBe("Bearer tk_new"));
  });

  it("treats topics the account already has as subscribed", async () => {
    serve({
      "/v1/account/token/redeem": () => respond(200, { token: "tk_new", username: "phil" }),
      "/v1/account/subscription": () => respond(409, { code: 40903, error: "conflict: topic subscription already exists" }),
    });

    expect(await redeemLink({ code: "abc123", user: "", topics: ["alerts"] })).toBe(RedeemResult.SIGNED_IN);
  });

  it("does not sign in when the code is invalid or expired", async () => {
    serve({
      "/v1/account/token/redeem": () => respond(400, { code: 40060, error: "invalid request: enrollment code invalid or expired" }),
    });

    await expect(redeemLink({ code: "used", user: "phil", topics: ["alerts"] })).rejects.toBeInstanceOf(EnrollCodeInvalidError);
    expect(session.exists()).toBeFalsy();
    expect(requestsTo("/v1/account/subscription")).toHaveLength(0);
  });

  it("rejects a link without a code when signed out", async () => {
    await expect(redeemLink({ code: "", user: "phil", topics: ["alerts"] })).rejects.toBeInstanceOf(EnrollCodeInvalidError);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("leaves the code unused when already signed in as the link's user, and only subscribes", async () => {
    await session.store("phil", "tk_existing");
    serve({ "/v1/account/subscription": () => respond(200, {}) });

    const result = await redeemLink({ code: "abc123", user: "phil", topics: ["alerts"] });

    expect(result).toBe(RedeemResult.ALREADY_SIGNED_IN);
    expect(session.token()).toBe("tk_existing");
    expect(requestsTo("/v1/account/token/redeem")).toHaveLength(0);
    expect(requestsTo("/v1/account/subscription")[0][1].headers.Authorization).toBe("Bearer tk_existing");
  });

  it("never replaces a session that belongs to someone else", async () => {
    await session.store("ben", "tk_ben");

    await expect(redeemLink({ code: "abc123", user: "phil", topics: ["alerts"] })).rejects.toBeInstanceOf(SignedInAsOtherUserError);
    expect(session.username()).toBe("ben");
    expect(session.token()).toBe("tk_ben");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
