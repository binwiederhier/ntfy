import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  UnauthorizedError,
  UserExistsError,
  TopicReservedError,
  AccountActionLimitReachedError,
  IncorrectPasswordError,
  EmailVerificationCodeInvalidError,
  EmailPrimaryElsewhereError,
  throwAppError,
  fetchOrThrow,
} from "./errors";

// A minimal stand-in for a fetch Response: just the bits errors.js reads.
const fakeResponse = (status, body) => ({
  status,
  json: async () => {
    if (body === undefined) throw new Error("no body");
    return body;
  },
});

beforeEach(() => {
  // errors.js logs to console.log on every error path; keep test output clean.
  vi.spyOn(console, "log").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("error classes", () => {
  it("carry the server error codes from errors.go", () => {
    expect(UserExistsError.CODE).toBe(40901);
    expect(TopicReservedError.CODE).toBe(40902);
    expect(AccountActionLimitReachedError.CODE).toBe(42906);
    expect(IncorrectPasswordError.CODE).toBe(40026);
    expect(EmailVerificationCodeInvalidError.CODE).toBe(40051);
    expect(EmailPrimaryElsewhereError.CODE).toBe(40908);
  });

  it("are Error subclasses with human-readable messages", () => {
    expect(new UnauthorizedError()).toBeInstanceOf(Error);
    expect(new UnauthorizedError().message).toBe("Unauthorized");
    expect(new UserExistsError().message).toBe("Username already exists");
  });
});

describe("throwAppError", () => {
  it("maps 401 and 403 to UnauthorizedError", async () => {
    await expect(throwAppError(fakeResponse(401))).rejects.toBeInstanceOf(UnauthorizedError);
    await expect(throwAppError(fakeResponse(403))).rejects.toBeInstanceOf(UnauthorizedError);
  });

  it("keeps the HTTP status on the error, so callers can tell bad credentials (401) from a topic ACL denial (403)", async () => {
    await expect(throwAppError(fakeResponse(401))).rejects.toHaveProperty("status", 401);
    await expect(throwAppError(fakeResponse(403))).rejects.toHaveProperty("status", 403);
  });

  it("maps known ntfy error codes to their specific error classes", async () => {
    await expect(throwAppError(fakeResponse(409, { code: UserExistsError.CODE }))).rejects.toBeInstanceOf(UserExistsError);
    await expect(throwAppError(fakeResponse(409, { code: TopicReservedError.CODE }))).rejects.toBeInstanceOf(TopicReservedError);
    await expect(throwAppError(fakeResponse(429, { code: AccountActionLimitReachedError.CODE }))).rejects.toBeInstanceOf(
      AccountActionLimitReachedError,
    );
  });

  it("wraps an unknown code that carries an error message in a generic Error", async () => {
    await expect(throwAppError(fakeResponse(400, { code: 12345, error: "boom" }))).rejects.toThrow("Error 12345: boom");
  });

  it("throws a generic 'Unexpected response' error when there is no ntfy error body", async () => {
    await expect(throwAppError(fakeResponse(500))).rejects.toThrow("Unexpected response 500");
  });
});

describe("fetchOrThrow", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the response unchanged on HTTP 200", async () => {
    const response = { status: 200 };
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => response),
    );
    await expect(fetchOrThrow("https://ntfy.sh/mytopic/json")).resolves.toBe(response);
  });

  it("throws on a non-200 response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => fakeResponse(401)),
    );
    await expect(fetchOrThrow("https://ntfy.sh/mytopic/json")).rejects.toBeInstanceOf(UnauthorizedError);
  });
});
