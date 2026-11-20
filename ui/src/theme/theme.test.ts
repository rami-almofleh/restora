import { describe, expect, it } from "vitest";
import { isThemePreference, resolveTheme } from "./theme";

describe("resolveTheme", () => {
  it.each([
    { preference: "light", systemDark: true, want: "light" },
    { preference: "dark", systemDark: false, want: "dark" },
    { preference: "system", systemDark: true, want: "dark" },
    { preference: "system", systemDark: false, want: "light" },
  ] as const)("$preference with system dark=$systemDark gives $want", ({ preference, systemDark, want }) => {
    expect(resolveTheme(preference, systemDark)).toBe(want);
  });
});

describe("isThemePreference", () => {
  it("accepts the three known values", () => {
    expect(["light", "dark", "system"].every(isThemePreference)).toBe(true);
  });

  it("rejects everything else", () => {
    expect(isThemePreference("blue")).toBe(false);
    expect(isThemePreference(null)).toBe(false);
  });
});
