import { useTranslation } from "react-i18next";
import { DEFAULT_LANGUAGE, isLanguage, type Language } from "./languages";

export function useLanguage(): { language: Language; setLanguage: (language: Language) => void } {
  const { i18n } = useTranslation();
  const language = isLanguage(i18n.language) ? i18n.language : DEFAULT_LANGUAGE;
  return { language, setLanguage: (next) => void i18n.changeLanguage(next) };
}
