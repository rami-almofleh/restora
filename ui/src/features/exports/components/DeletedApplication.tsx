import { ArchiveRestore, SearchX } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { StateMessage } from "../../../components/StateMessage";
import { StatusBadge } from "../../../components/StatusBadge";
import { RestoreCard } from "../../restores/components/RestoreCard";
import { useProtection } from "../../applications/use-protection";
import type { StoredBackup } from "../api";
import { useStoredBackups } from "../use-exports";
import { ExportCard } from "./ExportCard";
import { ExportRestoreDialog } from "./ExportRestoreDialog";

/**
 * The page of an application that does not exist in the cluster. When storage locations still hold backups of it, it
 * offers to bring it back from there. Without any, it is the usual "not found".
 */
export function DeletedApplication({ name }: { name: string }) {
  const { t } = useTranslation();
  const [pages, setPages] = useState(1);
  const [toRestore, setToRestore] = useState<StoredBackup | null>(null);
  const stored = useStoredBackups(name, pages);
  const protection = useProtection(name);

  const nothingStored = stored.state.status === "ready" && stored.state.data.items.length === 0;
  if (stored.state.status === "loading" || nothingStored) {
    return stored.state.status === "loading" ? (
      <div className="skeleton-stack" aria-busy="true">
        <span className="sr-only">{t("common.loading")}</span>
        <div className="skeleton skeleton--title" />
        <div className="skeleton skeleton--row" />
      </div>
    ) : (
      <StateMessage icon={SearchX} title={t("applications.detail.notFound.title")} text={t("applications.detail.notFound.text")}>
        <div className="state__actions">
          <Link className="button button--primary" to="/applications">
            {t("applications.detail.back")}
          </Link>
        </div>
      </StateMessage>
    );
  }

  const closeDialog = () => {
    protection.restore.reset();
    setToRestore(null);
  };
  const confirm = () => {
    if (toRestore) {
      void protection.restore.startFromExport(toRestore.id).then((started) => {
        if (started) setToRestore(null);
      });
    }
  };
  const busy = protection.busy !== null || protection.restore.state.status === "starting";

  return (
    <>
      <header className="page-header page-header--with-badge">
        <h1 className="page-header__title">
          <bdi>{name}</bdi>
        </h1>
        <StatusBadge tone="neutral" label={t("exports.deleted.badge")} />
      </header>

      <div className="card-grid">
        <div className="notice notice--info card--wide" role="status">
          <ArchiveRestore size={18} aria-hidden="true" />
          <p>{t("exports.deleted.text")}</p>
        </div>
        <ExportCard
          state={stored.state}
          onRestore={busy ? undefined : setToRestore}
          onReload={stored.reload}
          onLoadMore={() => setPages((n) => n + 1)}
        />
        <RestoreCard state={protection.restores.state} snapshots={[]} onReload={protection.restores.reload} onLoadMore={protection.loadMoreRestores} />
      </div>

      {toRestore ? (
        <ExportRestoreDialog backup={toRestore} applicationMissing state={protection.restore.state} onConfirm={confirm} onClose={closeDialog} />
      ) : null}
    </>
  );
}
