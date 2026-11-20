import { describe, expect, it } from "vitest";
import { browserTimezone, timezoneOptions } from "./timezones";

describe("timezoneOptions", () => {
  it("starts with UTC and lists a zone only once", () => {
    const options = timezoneOptions(["UTC", "Europe/Berlin"]);
    expect(options[0]).toBe("UTC");
    expect(options.filter((o) => o === "UTC")).toHaveLength(1);
    expect(options.filter((o) => o === "Europe/Berlin")).toHaveLength(1);
  });

  it("adds a zone the browser does not list", () => {
    expect(timezoneOptions(["Mars/Base"])).toContain("Mars/Base");
  });
});

describe("browserTimezone", () => {
  it("returns a zone name", () => {
    expect(browserTimezone()).not.toBe("");
  });
});
