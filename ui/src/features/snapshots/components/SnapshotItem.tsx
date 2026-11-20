import { History, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ExportList } from "../../../components/ExportList";
import { JobDetails } from "../../../components/JobDetails";
import { PhaseBadge } from "../../../components/PhaseBadge";
import { formatBytes, formatDateTime, formatDuration } from "../../../lib/format";
import { isActive, jobDurationMs } from "../../../lib/jobs";
import type { Snapshot } from "../api";

interface SnapshotItemProps {
  snapshot: Snapshot;
  /** Set when this snapshot may be restored right now. */
  onRestore?: (snapshot: Snapshot) => void;
  /** Set when this snapshot may be deleted right now. */
  onDelete?: (snapshot: Snapshot) => void;
  /** Set when a failed copy may be repeated right now. */
  onRetryExport?: (snapshot: Snapshot, location: string) => void;
}

export function SnapshotItem({ snapshot, onRestore, onDelete, onRetryExport }: SnapshotItemProps) {
  const { t, i18n } = useTranslation();
  const language = i18n.language;
  const sizeBytes = snapshot.volumes.reduce((sum, v) => sum + (v.sizeBytes ?? 0), 0);

  return (
    <li className="job">
      <div className="job__header">
        <time className="job__time" dateTime={snapshot.createdAt}>
          {formatDateTime(snapshot.createdAt, language)}
        </time>
        <PhaseBadge phase={snapshot.phase} />
      </div>

      {snapshot.policy ? <p className="job__meta">{t("activity.byPolicy", { name: snapshot.policy.name })}</p> : null}

      {snapshot.phase === "Succeeded" ? (
        <>
          <p className="job__meta">
            {t("snapshots.summary", {
              volumes: snapshot.volumes.length,
              resources: snapshot.resources?.count ?? 0,
              size: formatBytes(sizeBytes, language),
            })}
            {snapshot.finishedAt ? ` · ${t("common.duration", { duration: formatDuration(jobDurationMs(snapshot), language) })}` : null}
          </p>
        </>
      ) : null}

      <ExportList exports={snapshot.exports} onRetry={onRetryExport ? (location) => onRetryExport(snapshot, location) : undefined} />

      <JobDetails
        phase={snapshot.phase}
        steps={snapshot.steps}
        textPrefix="snapshots"
        errorCode={snapshot.errorCode}
        message={snapshot.message}
        hint={snapshot.hint}
      />

      {onRestore || onDelete ? (
        <div className="job__actions">
          {onRestore && snapshot.phase === "Succeeded" ? (
            <button type="button" className="button button--secondary" onClick={() => onRestore(snapshot)}>
              <History size={18} aria-hidden="true" />
              {t("restores.button")}
            </button>
          ) : null}
          {onDelete && !isActive(snapshot) ? (
            <button type="button" className="button button--secondary" onClick={() => onDelete(snapshot)}>
              <Trash2 size={18} aria-hidden="true" />
              {t("snapshots.delete.button")}
            </button>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}
