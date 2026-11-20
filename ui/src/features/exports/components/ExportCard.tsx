import { CloudDownload, RefreshCw, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { StateMessage } from "../../../components/StateMessage";
import { formatBytes, formatDateTime } from "../../../lib/format";
import type { PagedList } from "../../../lib/paged";
import type { ResourceState } from "../../../lib/use-resource";
import type { StoredBackup } from "../api";

interface ExportCardProps {
  state: ResourceState<PagedList<StoredBackup>>;
  /** Opens the restore dialog. Not set while something else runs on the application. */
  onRestore?: (backup: StoredBackup) => void;
  onReload: () => void;
  onLoadMore: () => void;
}

/** The backups of an application that are stored in storage locations. It stays out of the way until there is one. */
export function ExportCard({ state, onRestore, onReload, onLoadMore }: ExportCardProps) {
  const { t, i18n } = useTranslation();
  const language = i18n.language;

  if (state.status === "loading" || (state.status === "ready" && state.data.items.length === 0)) {
    return null;
  }

  return (
    <Card wide title={t("exports.card.title")}>
      <p className="muted card__intro">{t("exports.card.intro")}</p>
      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("exports.card.error.title")} text={t("exports.card.error.text")}>
          <div className="state__actions">
            <button type="button" className="button button--secondary" onClick={onReload}>
              <RefreshCw size={18} aria-hidden="true" />
              {t("common.retry")}
            </button>
          </div>
        </StateMessage>
      ) : (
        <>
          <ul className="job-list">
            {state.data.items.map((backup) => (
              <li className="job" key={backup.id}>
                <div className="job__header">
                  <time className="job__time" dateTime={backup.createdAt}>
                    {formatDateTime(backup.createdAt, language)}
                  </time>
                  <span className="job__where">
                    <bdi>{backup.location.name}</bdi>
                  </span>
                </div>
                <p className="job__meta">
                  {t("exports.card.summary", { volumes: backup.volumes.length, resources: backup.resources, size: formatBytes(backup.sizeBytes, language) })}
                </p>
                {backup.policy ? <p className="job__meta">{t("activity.byPolicy", { name: backup.policy.name })}</p> : null}
                {onRestore ? (
                  <div className="job__actions">
                    <button type="button" className="button button--secondary" onClick={() => onRestore(backup)}>
                      <CloudDownload size={18} aria-hidden="true" />
                      {t("exports.card.restore")}
                    </button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
          {state.data.hasMore ? (
            <div className="load-more">
              <button type="button" className="button button--secondary" onClick={onLoadMore}>
                {t("common.loadMore")}
              </button>
            </div>
          ) : null}
        </>
      )}
    </Card>
  );
}
