import { RefreshCw, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { StateMessage } from "../../../components/StateMessage";
import type { PagedList } from "../../../lib/paged";
import type { ResourceState } from "../../../lib/use-resource";
import type { Snapshot } from "../../snapshots/api";
import type { Restore } from "../api";
import { RestoreItem } from "./RestoreItem";

interface RestoreCardProps {
  state: ResourceState<PagedList<Restore>>;
  /** Used to show the time of the backup a restore came from. */
  snapshots: Snapshot[];
  onReload: () => void;
  onLoadMore: () => void;
}

/** Lists the restores of an application. It stays out of the way until the first restore happened. */
export function RestoreCard({ state, snapshots, onReload, onLoadMore }: RestoreCardProps) {
  const { t } = useTranslation();

  if (state.status === "loading" || (state.status === "ready" && state.data.items.length === 0)) {
    return null;
  }

  return (
    <Card wide title={t("restores.title")}>
      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("restores.error.title")} text={t("restores.error.text")}>
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
      ) : (
        <>
          <ul className="job-list">
            {state.data.items.map((restore) => (
              <RestoreItem
                key={restore.id}
                restore={restore}
                snapshotTime={snapshots.find((s) => s.id === restore.snapshot)?.createdAt}
              />
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
