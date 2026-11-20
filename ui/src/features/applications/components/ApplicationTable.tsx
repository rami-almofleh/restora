import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { ApplicationSummary } from "../api";
import { HealthBadge } from "./HealthBadge";

/** One row per application. On phones each row becomes a card (see .table in components.css). */
export function ApplicationTable({ items }: { items: ApplicationSummary[] }) {
  const { t } = useTranslation();
  return (
    <div className="table-surface">
      <table className="table table--rows">
        <thead>
          <tr>
            <th scope="col">{t("applications.columns.name")}</th>
            <th scope="col">{t("applications.columns.status")}</th>
            <th scope="col">{t("applications.columns.engines")}</th>
            <th scope="col" className="table__number">
              {t("applications.columns.workloads")}
            </th>
            <th scope="col" className="table__number">
              {t("applications.columns.volumes")}
            </th>
          </tr>
        </thead>
        <tbody>
          {items.map((app) => (
            <tr key={app.name}>
              <td className="table__primary">
                <Link className="table__row-link" to={`/applications/${encodeURIComponent(app.name)}`}>
                  <bdi>{app.name}</bdi>
                </Link>
              </td>
              <td data-label={t("applications.columns.status")}>
                <HealthBadge health={app.health} />
              </td>
              <td data-label={t("applications.columns.engines")}>
                {app.engines.length > 0 ? <bdi>{app.engines.join(", ")}</bdi> : <span className="muted">–</span>}
              </td>
              <td className="table__number" data-label={t("applications.columns.workloads")}>
                {app.workloads}
              </td>
              <td className="table__number" data-label={t("applications.columns.volumes")}>
                {app.volumes}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
