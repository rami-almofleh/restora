import { describe, expect, it } from "vitest";
import { isActive, jobDurationMs } from "./jobs";

describe("isActive", () => {
  it.each([
    { phase: "Pending", expected: true },
    { phase: "Running", expected: true },
    { phase: "Succeeded", expected: false },
    { phase: "Failed", expected: false },
  ] as const)("$phase is active: $expected", ({ phase, expected }) => {
    expect(isActive({ phase })).toBe(expected);
  });
});

describe("jobDurationMs", () => {
  const createdAt = "2026-11-19T10:00:00Z";

  it("uses the finish time of a finished job", () => {
    expect(jobDurationMs({ createdAt, finishedAt: "2026-11-19T10:00:42Z" }, Date.parse("2026-11-19T12:00:00Z"))).toBe(42_000);
  });

  it("counts up to now for a running job", () => {
    expect(jobDurationMs({ createdAt }, Date.parse("2026-11-19T10:01:00Z"))).toBe(60_000);
  });

  it("never goes negative when the clocks differ", () => {
    expect(jobDurationMs({ createdAt }, Date.parse("2026-11-19T09:59:00Z"))).toBe(0);
  });
});
