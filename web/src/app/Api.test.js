import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import userManager from "./UserManager";
import api from "./Api";
import { UnauthorizedError } from "./errors";

// Api.js talks to the server through the global fetch and looks up credentials via userManager.
// (vi.mock is hoisted above the imports by vitest, so the import above receives the mock.)
vi.mock("./UserManager", () => ({ default: { get: vi.fn() } }));

let fetchMock;

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, "log").mockImplementation(() => {});
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  userManager.get.mockResolvedValue(undefined); // anonymous by default
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Api.poll", () => {
  it("parses newline-delimited JSON and skips lines without an id", async () => {
    const body = [
      JSON.stringify({ id: "1", message: "a" }),
      JSON.stringify({ event: "keepalive" }), // no id -> skipped
      JSON.stringify({ id: "2", message: "b" }),
    ].join("\n");
    fetchMock.mockResolvedValue(new Response(body));

    const messages = await api.poll("https://ntfy.sh", "mytopic");

    expect(messages.map((m) => m.id)).toEqual(["1", "2"]);
  });

  it("uses the plain poll URL without a since cursor", async () => {
    fetchMock.mockResolvedValue(new Response(""));
    await api.poll("https://ntfy.sh", "mytopic");
    expect(fetchMock).toHaveBeenCalledWith("https://ntfy.sh/mytopic/json?poll=1", expect.anything());
  });

  it("uses the since URL when a cursor is given", async () => {
    fetchMock.mockResolvedValue(new Response(""));
    await api.poll("https://ntfy.sh", "mytopic", 12345);
    expect(fetchMock).toHaveBeenCalledWith("https://ntfy.sh/mytopic/json?poll=1&since=12345", expect.anything());
  });

  it("throws UnauthorizedError instead of returning silently on a 401/403", async () => {
    fetchMock.mockResolvedValue(new Response("", { status: 403 }));
    await expect(api.poll("https://ntfy.sh", "mytopic")).rejects.toBeInstanceOf(UnauthorizedError);
  });
});

describe("Api.publish", () => {
  it("PUTs the message body to the base URL", async () => {
    fetchMock.mockResolvedValue({ status: 200 });
    await api.publish("https://ntfy.sh", "mytopic", "Hello", { priority: 5, tags: ["warning"] });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ntfy.sh",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ topic: "mytopic", message: "Hello", priority: 5, tags: ["warning"] }),
      }),
    );
  });

  it("attaches basic auth when the user has a password", async () => {
    userManager.get.mockResolvedValue({ username: "phil", password: "secret" });
    fetchMock.mockResolvedValue({ status: 200 });
    await api.publish("https://ntfy.sh", "mytopic", "Hi");
    const [, options] = fetchMock.mock.calls[0];
    expect(options.headers.Authorization).toBe(`Basic ${btoa("phil:secret")}`);
  });

  it("attaches bearer auth when the user has a token", async () => {
    userManager.get.mockResolvedValue({ token: "tk_abc" });
    fetchMock.mockResolvedValue({ status: 200 });
    await api.publish("https://ntfy.sh", "mytopic", "Hi");
    const [, options] = fetchMock.mock.calls[0];
    expect(options.headers.Authorization).toBe("Bearer tk_abc");
  });
});

describe("Api.topicAuth", () => {
  it("returns true for a 2xx response", async () => {
    fetchMock.mockResolvedValue({ status: 200 });
    expect(await api.topicAuth("https://ntfy.sh", "mytopic")).toBe(true);
  });

  it("returns false for 401/403", async () => {
    fetchMock.mockResolvedValue({ status: 403 });
    expect(await api.topicAuth("https://ntfy.sh", "mytopic")).toBe(false);
  });

  it("throws for any other status", async () => {
    fetchMock.mockResolvedValue({ status: 500 });
    await expect(api.topicAuth("https://ntfy.sh", "mytopic")).rejects.toThrow("Unexpected server response 500");
  });
});

describe("Api web push", () => {
  const pushSubscription = {
    endpoint: "https://push.example/abc",
    keys: { auth: "AUTH", p256dh: "P256" },
  };

  it("updateWebPush POSTs endpoint, keys and topics to the web push URL", async () => {
    fetchMock.mockResolvedValue({ status: 200 });
    await api.updateWebPush(pushSubscription, ["topicA", "topicB"]);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ntfy.sh/v1/webpush",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ endpoint: "https://push.example/abc", auth: "AUTH", p256dh: "P256", topics: ["topicA", "topicB"] }),
      }),
    );
  });

  it("deleteWebPush DELETEs just the endpoint", async () => {
    fetchMock.mockResolvedValue({ status: 200 });
    await api.deleteWebPush(pushSubscription);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ntfy.sh/v1/webpush",
      expect.objectContaining({
        method: "DELETE",
        body: JSON.stringify({ endpoint: "https://push.example/abc" }),
      }),
    );
  });
});
