import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { detectLanguage, directionOf, isLanguage, LANGUAGE_STORAGE_KEY, type Language } from "./languages";
import ar from "./locales/ar.json";
import de from "./locales/de.json";
import en from "./locales/en.json";

function readStoredLanguage(): string | null {
  try {
    return localStorage.getItem(LANGUAGE_STORAGE_KEY);
  } catch {
    return null;
  }
}

function applyToDocument(language: Language): void {
  document.documentElement.setAttribute("lang", language);
  document.documentElement.setAttribute("dir", directionOf(language));
}

const initial = detectLanguage(readStoredLanguage(), navigator.languages);

void i18n.use(initReactI18next).init({
  resources: {
    de: { translation: de },
    en: { translation: en },
    ar: { translation: ar },
  },
  lng: initial,
  fallbackLng: "en",
  interpolation: { escapeValue: false },
  initAsync: false,
  react: { useSuspense: false },
});

applyToDocument(initial);

i18n.on("languageChanged", (language) => {
  if (!isLanguage(language)) {
    return;
  }
  applyToDocument(language);
  try {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
  } catch {
    // Storage can be blocked. The language still applies for this session.
  }
});

export default i18n;
