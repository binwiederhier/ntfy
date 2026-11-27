import { describe, expect, it } from "vitest";
import { matchPath } from "react-router-dom";
import routes from "./routes";

// Navigating to a subscription on a server whose base URL has a path (e.g. https://host/gitup)
// must produce a URL that still matches /:baseUrl/:topic, i.e. exactly two path segments.
describe("routes.forSubscription", () => {
  it("returns /:topic for the default server", () => {
    expect(routes.forSubscription({ baseUrl: "https://ntfy.sh", topic: "mytopic" })).toBe("/mytopic");
  });

  it("returns /:baseUrl/:topic for another server", () => {
    expect(routes.forSubscription({ baseUrl: "https://ntfy.example.com", topic: "mytopic" })).toBe("/ntfy.example.com/mytopic");
  });

  it("keeps a base URL with a path as a single route segment", () => {
    const path = routes.forSubscription({ baseUrl: "https://nebula.example.ts.net/gitup", topic: "status" });
    expect(path.split("/").filter(Boolean)).toHaveLength(2);
    const match = matchPath(routes.subscriptionExternal, path);
    expect(match).not.toBeNull();
    expect(match.params).toEqual({ baseUrl: "nebula.example.ts.net/gitup", topic: "status" });
  });
});
