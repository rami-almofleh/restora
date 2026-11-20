import { useTranslation } from "react-i18next";

/** Notes of a restore that succeeded, folded away so they do not hide the result. */
export function WarningList({ warnings }: { warnings: string[] }) {
  const { t } = useTranslation();
  if (warnings.length === 0) {
    return null;
  }
  return (
    <details className="details">
      <summary>{t("restores.warnings", { skipped: warnings.length })}</summary>
      <ul className="name-list">
        {warnings.map((w) => (
          <li key={w}>
            <bdi>{w}</bdi>
          </li>
        ))}
      </ul>
    </details>
  );
}
