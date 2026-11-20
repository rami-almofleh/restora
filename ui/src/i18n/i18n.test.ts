import { describe, expect, it } from "vitest";
import { detectLanguage, directionOf } from "./languages";
import ar from "./locales/ar.json";
import de from "./locales/de.json";
import en from "./locales/en.json";

type Tree = { [key: string]: string | Tree };

function keysOf(tree: Tree, prefix = ""): string[] {
  return Object.entries(tree).flatMap(([key, value]) =>
    typeof value === "string" ? [`${prefix}${key}`] : keysOf(value, `${prefix}${key}.`),
  );
}

describe("locale files", () => {
  const reference = keysOf(de).sort();

  it.each([
    { name: "en", tree: en },
    { name: "ar", tree: ar },
  ])("$name has exactly the same keys as de", ({ tree }) => {
    expect(keysOf(tree).sort()).toEqual(reference);
  });

  it("has no empty texts", () => {
    for (const tree of [de, en, ar]) {
      for (const key of keysOf(tree)) {
        const value = key.split(".").reduce<string | Tree>((node, part) => (node as Tree)[part], tree);
        expect(value, key).not.toBe("");
      }
    }
  });
});

describe("detectLanguage", () => {
  it("prefers the stored language", () => {
    expect(detectLanguage("ar", ["en-US"])).toBe("ar");
  });

  it("falls back to the first supported browser language", () => {
    expect(detectLanguage(null, ["fr-FR", "en-GB", "de"])).toBe("en");
  });

  it("ignores an unknown stored value", () => {
    expect(detectLanguage("xx", ["de-DE"])).toBe("de");
  });

  it("uses the default when nothing matches", () => {
    expect(detectLanguage(null, ["fr-FR"])).toBe("de");
  });
});

describe("directionOf", () => {
  it("is right-to-left only for Arabic", () => {
    expect(directionOf("ar")).toBe("rtl");
    expect(directionOf("de")).toBe("ltr");
    expect(directionOf("en")).toBe("ltr");
  });
});
