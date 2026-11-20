import { History, RefreshCw, ShieldCheck, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { StateMessage } from "../../../components/StateMessage";
import type { ResourceState } from "../../../lib/use-resource";
import type { PagedList } from "../../../lib/paged";
import type { Snapshot } from "../api";
import type { BackupState } from "../use-snapshots";
import { SnapshotItem } from "./SnapshotItem";

interface SnapshotCardProps {
  state: ResourceState<PagedList<Snapshot>>;
  backup: BackupState;
  explanationId: string;
  /** Why starting a new backup is not possible right now. Empty when it is. */
  busyText: string;
  /** Opens the restore dialog. Not set while something else runs on the application. */
  onRestore?: (snapshot: Snapshot) => void;
  /** Opens the delete dialog. Not set while something else runs on the application. */
  onDelete?: (snapshot: Snapshot) => void;
  /** Repeats a failed copy to a storage location. Not set while something else runs on the application. */
  onRetryExport?: (snapshot: Snapshot, location: string) => void;
  onReload: () => void;
  onBackup: () => void;
  onLoadMore: () => void;
}

export function SnapshotCard({ state, backup, explanationId, busyText, onRestore, onDelete, onRetryExport, onReload, onBackup, onLoadMore }: SnapshotCardProps) {
  const { t } = useTranslation();

  return (
    <Card wide title={t("snapshots.title")}>
      {backup.status === "error" ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`snapshots.backup.errors.${backup.error.code}`, { defaultValue: t("snapshots.backup.errors.unknown") })}</p>
            <p className="muted">{backup.error.hint ?? backup.error.message}</p>
          </div>
        </div>
      ) : null}

      {busyText ? (
        <p className="notice" id={explanationId} role="status">
          {busyText}
        </p>
      ) : null}

      {state.status === "loading" ? (
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
        </div>
      ) : null}

      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("snapshots.error.title")} text={t("snapshots.error.text")}>
          <div className="state__actions">
            <button type="button" className="button button--secondary" onClick={onReload}>
              <RefreshCw size={18} aria-hidden="true" />
              {t("common.retry")}
            </button>
          </div>
          <details className="details">
            <summary>{t("common.technicalDetails")}</summary>
            <code>
              {state.error.code}: {state.error.message}
            </code>
          </details>
        </StateMessage>
      ) : null}

      {state.status === "ready" && state.data.items.length === 0 ? (
        <StateMessage icon={History} title={t("snapshots.empty.title")} text={t("snapshots.empty.text")}>
          <div className="state__actions">
            <button type="button" className="button button--primary" onClick={onBackup} disabled={backup.status === "starting"}>
              <ShieldCheck size={18} aria-hidden="true" />
              {t("snapshots.backup.button")}
            </button>
          </div>
        </StateMessage>
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <>
          <ul className="job-list">
            {state.data.items.map((snapshot) => (
              <SnapshotItem key={snapshot.id} snapshot={snapshot} onRestore={onRestore} onDelete={onDelete} onRetryExport={onRetryExport} />
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
      ) : null}
    </Card>
  );
}
