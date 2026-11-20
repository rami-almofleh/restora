export const LANGUAGES = [
  { code: "de", label: "Deutsch" },
  { code: "en", label: "English" },
  { code: "ar", label: "العربية" },
] as const;

export type Language = (typeof LANGUAGES)[number]["code"];

export const DEFAULT_LANGUAGE: Language = "de";

// Keep in sync with public/theme-init.js.
export const LANGUAGE_STORAGE_KEY = "restora.language";

export function isLanguage(value: unknown): value is Language {
  return LANGUAGES.some((language) => language.code === value);
}

export function directionOf(language: Language): "ltr" | "rtl" {
  return language === "ar" ? "rtl" : "ltr";
}

/** Picks the stored language, else the first matching browser language, else the default. */
export function detectLanguage(stored: string | null, browserLanguages: readonly string[]): Language {
  if (isLanguage(stored)) {
    return stored;
  }
  for (const tag of browserLanguages) {
    const base = tag.toLowerCase().split("-")[0];
    if (isLanguage(base)) {
      return base;
    }
  }
  return DEFAULT_LANGUAGE;
}
