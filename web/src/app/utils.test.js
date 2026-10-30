import { describe, expect, it } from "vitest";
import { formatResetAt } from "./utils";
import { DATE_FORMAT, TIME_FORMAT } from "./Prefs";

describe("formatResetAt", () => {
  it("formats the reset time", () => {
    expect(formatResetAt(1793344380, DATE_FORMAT.ISO8601, TIME_FORMAT.H24)).toMatch(/^2026-10-/);
  });

  it("returns null without a reset time (a server from before reset_at)", () => {
    expect(formatResetAt(undefined, DATE_FORMAT.SYSTEM, TIME_FORMAT.SYSTEM)).toBeNull();
    expect(formatResetAt(undefined, DATE_FORMAT.DMY, TIME_FORMAT.H24)).toBeNull();
    expect(formatResetAt(undefined, DATE_FORMAT.ISO8601, TIME_FORMAT.H24)).toBeNull();
  });
});
