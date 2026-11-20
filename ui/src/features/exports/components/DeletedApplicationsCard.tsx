import { ArchiveRestore } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { formatDateTime } from "../../../lib/format";
import { useDeletedApplications } from "../use-exports";

/**
 * Applications that are gone from the cluster but whose backups are still in a storage location. They are in no list
 * of applications, so this is the way back to them. It shows nothing when there are none.
 */
export function DeletedApplicationsCard() {
  const { t, i18n } = useTranslation();
  const { state } = useDeletedApplications();
  if (state.status !== "ready" || state.data.length === 0) {
    return null;
  }

  return (
    <div className="deleted-applications">
      <Card wide title={t("applications.deleted.title")}>
        <p className="muted card__intro">{t("applications.deleted.text")}</p>
        <ul className="job-list">
          {state.data.map((app) => (
            <li className="job" key={app.application}>
              <div className="job__header">
                <span className="job__title">
                  <ArchiveRestore size={18} aria-hidden="true" />
                  <bdi>{app.application}</bdi>
                </span>
                <Link className="button button--secondary" to={`/applications/${encodeURIComponent(app.application)}`}>
                  {t("applications.deleted.open")}
                </Link>
              </div>
              <p className="job__meta">
                {t("applications.deleted.summary", { n: app.exports, time: formatDateTime(app.latestAt, i18n.language) })}
              </p>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
