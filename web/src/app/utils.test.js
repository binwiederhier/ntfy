import { describe, expect, it } from "vitest";
import { THEME, DATE_FORMAT, TIME_FORMAT } from "./Prefs";
import {
  topicUrl,
  topicUrlWs,
  topicUrlJsonPollWithSince,
  accountUrl,
  accountTokenUrl,
  shortUrl,
  expandUrl,
  expandSecureUrl,
  validUrl,
  validTopic,
  disallowedTopic,
  unsubscribedSharedTopics,
  encodeBase64,
  encodeBase64Url,
  bearerAuth,
  basicAuth,
  withBearerAuth,
  maybeWithAuth,
  splitNoEmpty,
  sanitizeUrl,
  hashCode,
  formatBytes,
  formatNumber,
  formatPrice,
  formatShortDuration,
  formatDate,
  formatDateTime,
  formatTime,
  getKebabCaseLangStr,
  darkModeEnabled,
  urlB64ToUint8Array,
} from "./utils";

describe("URL builders", () => {
  it("build topic URLs", () => {
    expect(topicUrl("https://ntfy.sh", "mytopic")).toBe("https://ntfy.sh/mytopic");
    expect(topicUrlJsonPollWithSince("https://ntfy.sh", "mytopic", 123)).toBe("https://ntfy.sh/mytopic/json?poll=1&since=123");
  });

  it("rewrite the scheme for websocket URLs", () => {
    expect(topicUrlWs("https://ntfy.sh", "mytopic")).toBe("wss://ntfy.sh/mytopic/ws");
    expect(topicUrlWs("http://localhost:8080", "mytopic")).toBe("ws://localhost:8080/mytopic/ws");
  });

  it("build account URLs", () => {
    expect(accountUrl("https://ntfy.sh")).toBe("https://ntfy.sh/v1/account");
    expect(accountTokenUrl("https://ntfy.sh")).toBe("https://ntfy.sh/v1/account/token");
  });
});

describe("url helpers", () => {
  it("strip the scheme with shortUrl", () => {
    expect(shortUrl("https://ntfy.sh/mytopic")).toBe("ntfy.sh/mytopic");
  });

  it("expand a bare host to both schemes", () => {
    expect(expandUrl("ntfy.sh")).toEqual(["https://ntfy.sh", "http://ntfy.sh"]);
    expect(expandSecureUrl("ntfy.sh")).toBe("https://ntfy.sh");
  });

  it("validate http(s) URLs", () => {
    expect(validUrl("https://ntfy.sh")).toBeTruthy();
    expect(validUrl("http://ntfy.sh")).toBeTruthy();
    expect(validUrl("ftp://ntfy.sh")).toBeFalsy();
  });
});

describe("topic validation", () => {
  it("rejects disallowed topics (from config.disallowed_topics)", () => {
    expect(disallowedTopic("app")).toBe(true);
    expect(disallowedTopic("mytopic")).toBe(false);
    expect(validTopic("app")).toBe(false);
  });

  it("accepts well-formed topic names and rejects malformed ones", () => {
    expect(validTopic("valid_Topic-123")).toBeTruthy();
    expect(validTopic("bad/slash")).toBeFalsy();
    expect(validTopic("with space")).toBeFalsy();
    expect(validTopic("")).toBeFalsy();
  });
});

describe("discover filtering", () => {
  const baseUrl = "https://ntfy.sh";
  const shared = [
    { topic: "announcements", owner: "phil" },
    { topic: "home-automation", owner: "ben" },
  ];

  it("keeps only topics the user is not subscribed to", () => {
    const subscriptions = [{ baseUrl, topic: "announcements" }];
    expect(unsubscribedSharedTopics(shared, subscriptions, baseUrl)).toEqual([{ topic: "home-automation", owner: "ben" }]);
  });

  it("ignores subscriptions on other servers", () => {
    const subscriptions = [{ baseUrl: "https://other.example", topic: "announcements" }];
    expect(unsubscribedSharedTopics(shared, subscriptions, baseUrl)).toEqual(shared);
  });

  it("returns an empty list when everything is subscribed (empty-state case)", () => {
    const subscriptions = [
      { baseUrl, topic: "announcements" },
      { baseUrl, topic: "home-automation" },
    ];
    expect(unsubscribedSharedTopics(shared, subscriptions, baseUrl)).toEqual([]);
  });

  it("handles missing topics/subscriptions gracefully", () => {
    expect(unsubscribedSharedTopics(undefined, undefined, baseUrl)).toEqual([]);
    expect(unsubscribedSharedTopics(shared, undefined, baseUrl)).toEqual(shared);
  });
});

describe("base64 + auth headers", () => {
  it("encodes base64 and base64url", () => {
    expect(encodeBase64("hello")).toBe("aGVsbG8=");
    expect(encodeBase64Url("hello")).toBe("aGVsbG8");
  });

  it("builds bearer and basic auth headers", () => {
    expect(bearerAuth("tok")).toBe("Bearer tok");
    expect(basicAuth("phil", "secret")).toBe(`Basic ${encodeBase64("phil:secret")}`);
    expect(withBearerAuth({ "Content-Type": "application/json" }, "tok")).toEqual({
      "Content-Type": "application/json",
      Authorization: "Bearer tok",
    });
  });

  it("picks the right scheme in maybeWithAuth", () => {
    expect(maybeWithAuth({}, { username: "u", password: "p" }).Authorization).toBe(basicAuth("u", "p"));
    expect(maybeWithAuth({}, { token: "tok" }).Authorization).toBe(bearerAuth("tok"));
    expect(maybeWithAuth({ a: 1 }, undefined)).toEqual({ a: 1 });
  });
});

describe("misc pure helpers", () => {
  it("splitNoEmpty trims and drops empty entries", () => {
    expect(splitNoEmpty("a, b, ,c ", ",")).toEqual(["a", "b", "c"]);
    expect(splitNoEmpty("", ",")).toEqual([]);
  });

  it("hashCode is deterministic", () => {
    expect(hashCode("")).toBe(0);
    expect(hashCode("a")).toBe(97);
    expect(hashCode("hello")).toBe(hashCode("hello"));
  });

  it("formatBytes is human readable", () => {
    expect(formatBytes(0)).toBe("0 bytes");
    expect(formatBytes(1024)).toBe("1 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
  });

  it("formatNumber abbreviates round thousands", () => {
    expect(formatNumber(0)).toBe(0);
    expect(formatNumber(1000)).toBe("1k");
    expect(formatNumber(1500)).toBe((1500).toLocaleString());
  });

  it("formatPrice renders cents as dollars", () => {
    expect(formatPrice(100)).toBe("$1");
    expect(formatPrice(150)).toBe("$1.5");
  });

  it("formatShortDuration picks the largest fitting unit", () => {
    expect(formatShortDuration(60000, "en")).toContain("minute");
    expect(formatShortDuration(3600000, "en")).toContain("hour");
  });

  it("getKebabCaseLangStr normalizes language tags", () => {
    expect(getKebabCaseLangStr("en_US")).toBe("en-US");
    expect(getKebabCaseLangStr(undefined)).toBe("en");
    expect(getKebabCaseLangStr("")).toBe("en");
  });

  it("darkModeEnabled honors the theme preference", () => {
    expect(darkModeEnabled(false, THEME.DARK)).toBe(true);
    expect(darkModeEnabled(true, THEME.LIGHT)).toBe(false);
    expect(darkModeEnabled(true, THEME.SYSTEM)).toBe(true);
    expect(darkModeEnabled(false, THEME.SYSTEM)).toBe(false);
  });

  it("urlB64ToUint8Array decodes web push keys", () => {
    expect(Array.from(urlB64ToUint8Array("AQID"))).toEqual([1, 2, 3]);
  });
});

describe("date/time formatting", () => {
  // 2026-03-08 14:30 in the runner's local timezone (the same wall-clock time Intl renders).
  const ts = Math.floor(new Date(2026, 2, 8, 14, 30, 0).getTime() / 1000);

  it("ISO 8601 mode renders YYYY-MM-DD HH:mm regardless of locale", () => {
    expect(formatDateTime(ts, DATE_FORMAT.ISO8601)).toBe("2026-03-08 14:30");
    expect(formatDate(ts, DATE_FORMAT.ISO8601)).toBe("2026-03-08");
  });

  it("ISO 8601 mode zero-pads single-digit months, days, hours and minutes", () => {
    const early = Math.floor(new Date(2026, 0, 5, 9, 7, 0).getTime() / 1000);
    expect(formatDateTime(early, DATE_FORMAT.ISO8601)).toBe("2026-01-05 09:07");
  });

  it("default mode follows the browser locale, not the UI translation language", () => {
    // The format must depend only on the browser's default locale -- passing a language
    // tag (the old behavior) must NOT change the output anymore.
    const browserDefault = new Intl.DateTimeFormat(undefined, { dateStyle: "short", timeStyle: "short" }).format(new Date(ts * 1000));
    expect(formatDateTime(ts, DATE_FORMAT.SYSTEM)).toBe(browserDefault);
    expect(formatDateTime(ts, "fr")).toBe(browserDefault);
    expect(formatDateTime(ts, "en_US")).toBe(browserDefault);
    expect(formatDateTime(ts, undefined)).toBe(browserDefault);
  });

  it("only ISO 8601 zero-pads; DMY, dot-separated DMY and MDY all drop leading zeros", () => {
    expect(formatDate(ts, DATE_FORMAT.DMY)).toBe("8/3/2026");
    expect(formatDate(ts, DATE_FORMAT.DMY_DOT)).toBe("8.3.2026");
    expect(formatDate(ts, DATE_FORMAT.MDY)).toBe("3/8/2026");
    // Double-digit day/month are left intact.
    const ts2 = Math.floor(new Date(2026, 11, 25, 14, 30, 0).getTime() / 1000);
    expect(formatDate(ts2, DATE_FORMAT.DMY)).toBe("25/12/2026");
    expect(formatDate(ts2, DATE_FORMAT.DMY_DOT)).toBe("25.12.2026");
    expect(formatDate(ts2, DATE_FORMAT.MDY)).toBe("12/25/2026");
    // The same ordering prefixes the datetime variant.
    expect(formatDateTime(ts, DATE_FORMAT.DMY)).toMatch(/^8\/3\/2026 /);
    expect(formatDateTime(ts, DATE_FORMAT.DMY_DOT)).toMatch(/^8\.3\.2026 /);
    expect(formatDateTime(ts, DATE_FORMAT.MDY)).toMatch(/^3\/8\/2026 /);
  });

  it("formatTime renders only the time, honoring the 12/24-hour choice", () => {
    const local24 = new Intl.DateTimeFormat(undefined, { timeStyle: "short", hour12: false }).format(new Date(ts * 1000));
    expect(formatTime(ts, TIME_FORMAT.H24)).toBe(local24);
    expect(formatTime(ts, TIME_FORMAT.H24)).toContain("14:30");
    expect(formatTime(ts, TIME_FORMAT.H12)).toContain("2:30");
  });

  it("time format selects 12-hour vs 24-hour independently of the date format", () => {
    const local24 = new Intl.DateTimeFormat(undefined, { timeStyle: "short", hour12: false }).format(new Date(ts * 1000));
    expect(formatDateTime(ts, DATE_FORMAT.DMY, TIME_FORMAT.H24)).toBe(`8/3/2026 ${local24}`);

    const h12 = formatDateTime(ts, DATE_FORMAT.SYSTEM, TIME_FORMAT.H12);
    const h24 = formatDateTime(ts, DATE_FORMAT.SYSTEM, TIME_FORMAT.H24);
    expect(h24).toContain("14:30");
    expect(h12).toContain("2:30");
    expect(h12).not.toBe(h24);
  });

  it("ISO 8601 is always 24-hour regardless of the time format", () => {
    expect(formatDateTime(ts, DATE_FORMAT.ISO8601, TIME_FORMAT.H12)).toBe("2026-03-08 14:30");
  });
});

describe("sanitizeUrl", () => {
  it("keeps safe absolute URLs", () => {
    expect(sanitizeUrl("https://ntfy.sh")).toBe("https://ntfy.sh");
    expect(sanitizeUrl("http://example.com/foo?bar=1")).toBe("http://example.com/foo?bar=1");
    expect(sanitizeUrl("mailto:phil@ntfy.sh")).toBe("mailto:phil@ntfy.sh");
    expect(sanitizeUrl("ftp://ftp.example.com/file.txt")).toBe("ftp://ftp.example.com/file.txt");
    expect(sanitizeUrl("ftps://ftp.example.com/file.txt")).toBe("ftps://ftp.example.com/file.txt");
  });

  it("keeps relative URLs, fragments and query strings", () => {
    expect(sanitizeUrl("/docs")).toBe("/docs");
    expect(sanitizeUrl("./foo")).toBe("./foo");
    expect(sanitizeUrl("#section")).toBe("#section");
    expect(sanitizeUrl("?q=1")).toBe("?q=1");
    expect(sanitizeUrl("foo/bar")).toBe("foo/bar");
  });

  it("does not treat a colon after a slash/query/hash as a protocol", () => {
    expect(sanitizeUrl("/path:with:colons")).toBe("/path:with:colons");
    expect(sanitizeUrl("foo?x=a:b")).toBe("foo?x=a:b");
  });

  it("strips dangerous protocols", () => {
    /* eslint-disable no-script-url -- these javascript: literals are exactly what we're testing */
    expect(sanitizeUrl("javascript:alert(document.domain)")).toBe("");
    expect(sanitizeUrl("JavaScript:alert(1)")).toBe("");
    expect(sanitizeUrl("  javascript:alert(1)")).toBe("");
    /* eslint-enable no-script-url */
    expect(sanitizeUrl("vbscript:msgbox(1)")).toBe("");
    expect(sanitizeUrl("data:text/html,<script>alert(1)</script>")).toBe("");
  });

  it("handles empty and nullish input", () => {
    expect(sanitizeUrl("")).toBe("");
    expect(sanitizeUrl(undefined)).toBe("");
    expect(sanitizeUrl(null)).toBe("");
  });
});
