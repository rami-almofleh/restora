import { useTranslation } from "react-i18next";
import { JobDetails } from "../../../components/JobDetails";
import { PhaseBadge } from "../../../components/PhaseBadge";
import { WarningList } from "../../../components/WarningList";
import { formatDateTime, formatDuration } from "../../../lib/format";
import { jobDurationMs } from "../../../lib/jobs";
import type { Restore } from "../api";

interface RestoreItemProps {
  restore: Restore;
  /** Creation time of the snapshot that is restored, when it is known. */
  snapshotTime?: string;
}

export function RestoreItem({ restore, snapshotTime }: RestoreItemProps) {
  const { t, i18n } = useTranslation();
  const language = i18n.language;

  return (
    <li className="job">
      <div className="job__header">
        <time className="job__time" dateTime={restore.createdAt}>
          {formatDateTime(restore.createdAt, language)}
        </time>
        <PhaseBadge phase={restore.phase} />
      </div>
      <p className="job__meta">
        {restore.export
          ? t("restores.fromExport", {
              location: restore.export.locationName ?? "",
              time: restore.export.createdAt ? formatDateTime(restore.export.createdAt, language) : "",
            })
          : snapshotTime
            ? t("restores.fromSnapshot", { time: formatDateTime(snapshotTime, language) })
            : t("restores.fromUnknownSnapshot")}
      </p>

      <JobDetails
        phase={restore.phase}
        steps={restore.steps}
        textPrefix="restores"
        errorCode={restore.errorCode}
        message={restore.message}
        hint={restore.hint}
      />

      {restore.phase === "Succeeded" ? (
        <>
          <p className="job__meta">
            {t("restores.summary", {
              created: restore.resources?.created ?? 0,
              updated: restore.resources?.updated ?? 0,
            })}
            {restore.finishedAt ? ` · ${t("common.duration", { duration: formatDuration(jobDurationMs(restore), language) })}` : null}
          </p>
          <WarningList warnings={restore.warnings} />
        </>
      ) : null}
    </li>
  );
}
