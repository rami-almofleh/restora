import { Moon, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../theme/use-theme";

export function ThemeToggle() {
  const { t } = useTranslation();
  const { resolved, setPreference } = useTheme();
  const isDark = resolved === "dark";

  return (
    <button
      type="button"
      className="icon-button"
      onClick={() => setPreference(isDark ? "light" : "dark")}
      aria-label={isDark ? t("theme.switchToLight") : t("theme.switchToDark")}
    >
      {isDark ? <Sun size={20} aria-hidden="true" /> : <Moon size={20} aria-hidden="true" />}
    </button>
  );
}
