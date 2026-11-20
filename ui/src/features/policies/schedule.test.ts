import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";
import type { Policy } from "./api";
import { describeRetention, describeSchedule, describeSelector, formatClock, weekdayName } from "./schedule";

// A translation stand-in that shows the key and its parameters.
const t = ((key: string, params?: Record<string, unknown>) => `${key}${params ? JSON.stringify(params) : ""}`) as unknown as TFunction;

describe("formatClock", () => {
  it("pads hours and minutes", () => {
    expect(formatClock(2, 5)).toBe("02:05");
    expect(formatClock(23, 59)).toBe("23:59");
  });
});

describe("weekdayName", () => {
  it("counts from Sunday", () => {
    expect(weekdayName(0, "en")).toBe("Sunday");
    expect(weekdayName(1, "en")).toBe("Monday");
    expect(weekdayName(6, "en")).toBe("Saturday");
    expect(weekdayName(1, "de")).toBe("Montag");
  });
});

describe("describeSchedule", () => {
  it("names the type and the zone", () => {
    const text = describeSchedule(t, "en", { type: "daily", minute: 30, hour: 2, weekday: 0, timezone: "Europe/Berlin" });
    expect(text).toContain("policies.schedule.inZone");
    expect(text).toContain("Europe/Berlin");
    expect(text).toContain("02:30");
  });

  it("falls back to UTC and shows the cron of a custom schedule", () => {
    const text = describeSchedule(t, "en", { type: "custom", minute: 0, hour: 0, weekday: 0, cron: "*/5 * * * *" });
    expect(text).toContain("UTC");
    expect(text).toContain("*/5 * * * *");
  });
});

describe("describeSelector", () => {
  const base: Policy = {
    id: "p", name: "x", cron: "", paused: false, applications: ["demo", "shop"], missing: [], createdAt: "",
    selector: { mode: "all" }, schedule: { type: "hourly", minute: 0, hour: 0, weekday: 0 },
    retention: { last: 0, hourly: 0, daily: 0, weekly: 0, monthly: 0, yearly: 0 },
    exportTo: [],
  };

  it("counts all applications", () => {
    expect(describeSelector(t, base)).toContain('"n":2');
  });

  it("lists named applications", () => {
    expect(describeSelector(t, { ...base, selector: { mode: "applications", applications: ["a", "b"] } })).toBe("a, b");
  });

  it("shows labels with the number of matches", () => {
    const text = describeSelector(t, { ...base, selector: { mode: "labels", matchLabels: { team: "a" } } });
    expect(text).toContain("team=a");
    expect(text).toContain('"n":2');
  });
});

describe("describeRetention", () => {
  const none = { last: 0, hourly: 0, daily: 0, weekly: 0, monthly: 0, yearly: 0 };

  it("says that everything is kept when no rule is set", () => {
    expect(describeRetention(t, "en", none)).toBe("policies.retention.keepAll");
  });

  it("lists the rules that are set, in order, and leaves out the others", () => {
    // A stand-in that shows only the key and the number or the joined parts.
    const plain = ((key: string, params?: { n?: number; parts?: string }) => `${key}[${params?.n ?? params?.parts ?? ""}]`) as unknown as TFunction;

    const text = describeRetention(plain, "en", { ...none, yearly: 1, last: 2, daily: 7 });
    expect(text.startsWith("policies.retention.keeps[")).toBe(true);
    const order = ["parts.last[2]", "parts.daily[7]", "parts.yearly[1]"].map((part) => text.indexOf(part));
    expect(order.every((at) => at > 0)).toBe(true);
    expect(order).toEqual([...order].sort((a, b) => a - b));
    expect(text).not.toContain("parts.weekly");
  });
});
