import { useId } from "react";
import { useTranslation } from "react-i18next";
import { isLanguage, LANGUAGES } from "../../i18n/languages";
import { useLanguage } from "../../i18n/use-language";

export function LanguageSelect() {
  const { t } = useTranslation();
  const { language, setLanguage } = useLanguage();
  const id = useId();

  return (
    <>
      <label className="sr-only" htmlFor={id}>
        {t("common.language")}
      </label>
      <select
        id={id}
        className="select"
        value={language}
        onChange={(event) => {
          if (isLanguage(event.target.value)) {
            setLanguage(event.target.value);
          }
        }}
      >
        {LANGUAGES.map((option) => (
          <option key={option.code} value={option.code}>
            {option.label}
          </option>
        ))}
      </select>
    </>
  );
}
