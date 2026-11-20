import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import { LANGUAGES, type Language } from "../../../i18n/languages";
import { useLanguage } from "../../../i18n/use-language";
import type { ThemePreference } from "../../../theme/theme";
import { useTheme } from "../../../theme/use-theme";
import { writeFlag } from "../../setup/storage";

const LANGUAGE_OPTIONS: readonly SegmentedOption<Language>[] = LANGUAGES.map((l) => ({
  value: l.code,
  label: l.label,
}));

export function SettingsPage() {
  const { t } = useTranslation();
  const { language, setLanguage } = useLanguage();
  const { preference, setPreference } = useTheme();

  const themeOptions: readonly SegmentedOption<ThemePreference>[] = [
    { value: "light", label: t("theme.light") },
    { value: "dark", label: t("theme.dark") },
    { value: "system", label: t("theme.system") },
  ];

  return (
    <>
      <PageHeader title={t("settings.title")} subtitle={t("settings.subtitle")} />
      <Card title={t("settings.general")}>
        <div className="setting">
          <div className="setting__text">
            <h3 className="setting__title">{t("settings.language.title")}</h3>
            <p className="setting__description">{t("settings.language.description")}</p>
          </div>
          <SegmentedControl
            label={t("settings.language.title")}
            name="language"
            value={language}
            options={LANGUAGE_OPTIONS}
            onChange={setLanguage}
          />
        </div>
        <div className="setting">
          <div className="setting__text">
            <h3 className="setting__title">{t("settings.theme.title")}</h3>
            <p className="setting__description">{t("settings.theme.description")}</p>
          </div>
          <SegmentedControl
            label={t("settings.theme.title")}
            name="theme"
            value={preference}
            options={themeOptions}
            onChange={setPreference}
          />
        </div>
      </Card>
      <div className="setup-banner">
        <Card title={t("setup.settings.title")}>
          <p className="muted card__intro">{t("setup.settings.text")}</p>
          <Link className="button button--secondary" to="/setup" onClick={() => writeFlag("dismissed", false)}>
            {t("setup.settings.open")}
          </Link>
        </Card>
      </div>
    </>
  );
}
