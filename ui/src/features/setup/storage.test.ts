import { afterEach, describe, expect, it, vi } from "vitest";
import { readFlag, writeFlag } from "./storage";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => data[k] ?? null,
    setItem: (k: string, v: string) => void (data[k] = v),
    removeItem: (k: string) => void delete data[k],
  });
  return data;
}

describe("setup flags", () => {
  it("are off until they are set", () => {
    stubStorage();
    expect(readFlag("dismissed")).toBe(false);
    writeFlag("dismissed", true);
    expect(readFlag("dismissed")).toBe(true);
    writeFlag("dismissed", false);
    expect(readFlag("dismissed")).toBe(false);
  });

  it("do not disturb one another", () => {
    stubStorage();
    writeFlag("seen", true);
    expect(readFlag("seen")).toBe(true);
    expect(readFlag("restoreSkipped")).toBe(false);
  });

  it("survive a browser without storage", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
      removeItem: () => {
        throw new Error("blocked");
      },
    });
    expect(readFlag("seen")).toBe(false);
    expect(() => writeFlag("seen", true)).not.toThrow();
  });
});
