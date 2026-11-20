import { describe, expect, it } from "vitest";
import { formatBytes, formatDateTimeIn, formatDuration } from "./format";

describe("formatBytes", () => {
  it.each([
    { bytes: 0, expected: "0 B" },
    { bytes: 512, expected: "512 B" },
    { bytes: 1024, expected: "1 KiB" },
    { bytes: 1 << 30, expected: "1 GiB" },
    { bytes: 1.5 * 1024 ** 3, expected: "1.5 GiB" },
    { bytes: 20 * 1024 ** 3, expected: "20 GiB" },
    { bytes: -5, expected: "0 B" },
  ])("formats $bytes as $expected", ({ bytes, expected }) => {
    expect(formatBytes(bytes, "en")).toBe(expected);
  });
});

describe("formatDuration", () => {
  it.each([
    { ms: 0, expected: "0s" },
    { ms: 4400, expected: "4s" },
    { ms: 59_000, expected: "59s" },
    { ms: 125_000, expected: "2m" },
    { ms: 2 * 3600_000, expected: "2h" },
    { ms: -50, expected: "0s" },
  ])("formats $ms ms as $expected", ({ ms, expected }) => {
    expect(formatDuration(ms, "en")).toBe(expected);
  });
});

describe("formatDateTimeIn", () => {
  it("shows the time of the named zone together with its abbreviation", () => {
    const text = formatDateTimeIn("2026-11-20T01:00:00Z", "en", "Europe/Berlin");
    expect(text).toContain("2:00");
    expect(text).toMatch(/GMT\+1|CET/);
  });

  it("keeps a text that is not a date", () => {
    expect(formatDateTimeIn("soon", "en", "UTC")).toBe("soon");
  });
});
