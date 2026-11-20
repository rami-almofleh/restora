import { CloudUpload, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { formatBytes } from "../lib/format";
import type { JobPhase } from "../lib/jobs";
import { PhaseBadge } from "./PhaseBadge";

/** The copy of one backup to one storage location, as the API reports it. */
export interface ExportCopy {
  location: string;
  locationName?: string;
  phase: JobPhase;
  errorCode?: string;
  message?: string;
  hint?: string;
  /** How much the repository grew by this copy. Data stored before does not count. */
  addedBytes?: number;
  volumes?: { claim: string; phase: JobPhase; sizeBytes?: number; addedBytes?: number }[];
}

/** One line per storage location: where the backup was copied, how much that added, or why it failed. */
export function ExportList({ exports, onRetry }: { exports: ExportCopy[]; onRetry?: (location: string) => void }) {
  const { t, i18n } = useTranslation();
  if (exports.length === 0) {
    return null;
  }
  const language = i18n.language;

  return (
    <ul className="export-list" aria-label={t("exports.title")}>
      {exports.map((copy) => {
        const size = (copy.volumes ?? []).reduce((sum, v) => sum + (v.sizeBytes ?? 0), 0);
        const done = (copy.volumes ?? []).filter((v) => v.phase === "Succeeded").length;
        return (
          <li key={copy.location} className="export-list__item">
            <div className="export-list__header">
              <span className="export-list__name">
                <CloudUpload size={16} aria-hidden="true" />
                <bdi>{copy.locationName || copy.location}</bdi>
              </span>
              <PhaseBadge phase={copy.phase} />
            </div>
            {copy.phase === "Succeeded" ? (
              <p className="job__meta">{t("exports.summary", { size: formatBytes(size, language), added: formatBytes(copy.addedBytes ?? 0, language) })}</p>
            ) : null}
            {copy.phase === "Running" ? (
              <p className="job__meta">
                {(copy.volumes ?? []).length > 0 ? t("exports.progress", { done, total: copy.volumes?.length ?? 0 }) : t("exports.progressResources")}
              </p>
            ) : null}
            {copy.phase === "Failed" ? (
              <div className="failure">
                <p>{t(`exports.errors.${copy.errorCode}.text`, { defaultValue: t("exports.errors.unknown.text") })}</p>
                <p className="muted">{t(`exports.errors.${copy.errorCode}.hint`, { defaultValue: t("exports.errors.unknown.hint") })}</p>
                <details className="details">
                  <summary>{t("common.technicalDetails")}</summary>
                  <code>
                    {copy.errorCode}: {copy.message}
                  </code>
                </details>
                {onRetry ? (
                  <div className="job__actions">
                    <button type="button" className="button button--secondary" onClick={() => onRetry(copy.location)}>
                      <RefreshCw size={18} aria-hidden="true" />
                      {t("exports.retry")}
                    </button>
                  </div>
                ) : null}
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
