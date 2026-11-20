import { History, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { ExportList } from "../../../components/ExportList";
import { JobDetails } from "../../../components/JobDetails";
import { PhaseBadge } from "../../../components/PhaseBadge";
import { WarningList } from "../../../components/WarningList";
import { formatBytes, formatDateTime, formatDuration } from "../../../lib/format";
import { isActive, jobDurationMs } from "../../../lib/jobs";
import type { ActivityEntry } from "../api";

/** One backup or restore in the timeline. A failed entry shows what happened and what to do right away. */
export function ActivityItem({ entry }: { entry: ActivityEntry }) {
  const { t, i18n } = useTranslation();
  const language = i18n.language;
  const isBackup = entry.type === "backup";
  const Icon = isBackup ? ShieldCheck : History;
  const duration = formatDuration(jobDurationMs(entry), language);

  return (
    <li className="job">
      <div className="job__header">
        <h3 className="job__title">
          <Icon size={18} aria-hidden="true" />
          {t(`activity.type.${entry.type}`)}
        </h3>
        <PhaseBadge phase={entry.phase} />
      </div>

      <p className="job__meta">
        <Link className="job__link" to={`/applications/${encodeURIComponent(entry.application)}`}>
          <bdi>{entry.application}</bdi>
        </Link>
        {" · "}
        <time dateTime={entry.createdAt}>{formatDateTime(entry.createdAt, language)}</time>
        {" · "}
        {isActive(entry) ? t("activity.runningFor", { duration }) : t("common.duration", { duration })}
      </p>

      {entry.policy ? <p className="job__meta">{t("activity.byPolicy", { name: entry.policy.name })}</p> : null}

      {entry.phase === "Succeeded" && entry.backup ? (
        <p className="job__meta">
          {t("snapshots.summary", {
            volumes: entry.backup.volumes,
            resources: entry.backup.resources,
            size: formatBytes(entry.backup.bytes, language),
          })}
        </p>
      ) : null}

      {entry.restore ? (
        <p className="job__meta">
          {entry.restore.exportLocation !== undefined
            ? t("restores.fromExport", {
                location: entry.restore.exportLocation,
                time: entry.restore.snapshotCreatedAt ? formatDateTime(entry.restore.snapshotCreatedAt, language) : "",
              })
            : entry.restore.snapshotCreatedAt
              ? t("restores.fromSnapshot", { time: formatDateTime(entry.restore.snapshotCreatedAt, language) })
              : t("restores.fromUnknownSnapshot")}
          {entry.phase === "Succeeded"
            ? ` · ${t("restores.summary", { created: entry.restore.created, updated: entry.restore.updated })}`
            : null}
        </p>
      ) : null}

      <ExportList exports={entry.exports} />

      <JobDetails
        phase={entry.phase}
        steps={entry.steps}
        textPrefix={isBackup ? "snapshots" : "restores"}
        errorCode={entry.errorCode}
        message={entry.message}
        hint={entry.hint}
      />
      {entry.restore && entry.phase === "Succeeded" ? <WarningList warnings={entry.restore.warnings} /> : null}
    </li>
  );
}
