import { ArrowLeft, RefreshCw, TriangleAlert } from "lucide-react";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { Card } from "../../../components/Card";
import { StateMessage } from "../../../components/StateMessage";
import { formatBytes, formatDateTime } from "../../../lib/format";
import type { ApplicationDetail } from "../api";
import { HealthBadge } from "../components/HealthBadge";
import { VolumePhaseBadge } from "../components/VolumePhaseBadge";
import { DeletedApplication } from "../../exports/components/DeletedApplication";
import { CopyProtection } from "../../exports/components/CopyProtection";
import { ExportCard } from "../../exports/components/ExportCard";
import { ExportRestoreDialog } from "../../exports/components/ExportRestoreDialog";
import type { StoredBackup } from "../../exports/api";
import { useStoredBackups } from "../../exports/use-exports";
import { RestoreCard } from "../../restores/components/RestoreCard";
import { RestoreDialog } from "../../restores/components/RestoreDialog";
import type { Snapshot } from "../../snapshots/api";
import { BackupButton } from "../../snapshots/components/BackupButton";
import { DeleteSnapshotDialog } from "../../snapshots/components/DeleteSnapshotDialog";
import { retryExport } from "../../snapshots/api";
import { SnapshotCard } from "../../snapshots/components/SnapshotCard";
import { useDeleteSnapshot } from "../../snapshots/use-snapshots";
import { useApplication } from "../use-applications";
import { useProtection } from "../use-protection";

function BackLink() {
  const { t } = useTranslation();
  return (
    <Link className="back-link" to="/applications">
      <ArrowLeft size={18} aria-hidden="true" />
      {t("applications.detail.back")}
    </Link>
  );
}

export function ApplicationDetailPage() {
  const { t } = useTranslation();
  const { name = "" } = useParams();
  const { state, reload } = useApplication(name);

  if (state.status === "loading") {
    return (
      <>
        <BackLink />
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--title" />
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
        </div>
      </>
    );
  }

  if (state.status === "error") {
    const notFound = state.error.code === "application_not_found";
    return (
      <>
        <BackLink />
        {notFound ? (
          <DeletedApplication name={name} />
        ) : (
          <StateMessage
            tone="danger"
            icon={TriangleAlert}
            title={t("applications.detail.error.title")}
            text={`${t("applications.error.text")} ${t("applications.error.hint")}`}
          >
            <div className="state__actions">
              <button type="button" className="button button--secondary" onClick={reload}>
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
        )}
      </>
    );
  }

  return (
    <>
      <BackLink />
      <ApplicationDetailView app={state.data} />
    </>
  );
}

function ApplicationDetailView({ app }: { app: ApplicationDetail }) {
  const { t, i18n } = useTranslation();
  const totalBytes = app.volumeItems.reduce((sum, v) => sum + v.capacityBytes, 0);
  const explanationId = useId();
  const protection = useProtection(app.name);
  const [toRestore, setToRestore] = useState<Snapshot | null>(null);
  const [toDelete, setToDelete] = useState<Snapshot | null>(null);
  const [toRestoreStored, setToRestoreStored] = useState<StoredBackup | null>(null);
  const [storedPages, setStoredPages] = useState(1);
  const stored = useStoredBackups(app.name, storedPages);
  const deletion = useDeleteSnapshot(protection.snapshots.reload);
  const retryCopy = (snapshot: Snapshot, location: string) => {
    // A conflict only means the copy is not failed any more, and the reloaded list shows exactly that.
    void retryExport(snapshot.id, location)
      .catch(() => undefined)
      .finally(protection.snapshots.reload);
  };

  const starting = protection.backup.state.status === "starting" || protection.restore.state.status === "starting";
  const busyText = protection.busy ? t(`${protection.busy === "restore" ? "restores" : "snapshots"}.running`) : "";
  const closeDialog = () => {
    protection.restore.reset();
    setToRestore(null);
  };
  const closeDeleteDialog = () => {
    deletion.reset();
    setToDelete(null);
  };
  const confirmDelete = () => {
    if (toDelete) {
      void deletion.remove(toDelete.id).then((deleted) => {
        if (deleted) {
          setToDelete(null);
        }
      });
    }
  };
  const closeStoredDialog = () => {
    protection.restore.reset();
    setToRestoreStored(null);
  };
  const confirmStoredRestore = () => {
    if (toRestoreStored) {
      void protection.restore.startFromExport(toRestoreStored.id).then((started) => {
        if (started) {
          setToRestoreStored(null);
        }
      });
    }
  };
  const confirmRestore = () => {
    if (toRestore) {
      void protection.restore.start(toRestore.id).then((started) => {
        if (started) {
          setToRestore(null);
        }
      });
    }
  };

  return (
    <div className="has-action-bar">
      <header className="page-header page-header--with-badge">
        <h1 className="page-header__title">
          <bdi>{app.name}</bdi>
        </h1>
        <HealthBadge health={app.health} />
        <BackupButton
          starting={protection.backup.state.status === "starting"}
          blocked={protection.busy !== null || protection.restore.state.status === "starting"}
          explanationId={explanationId}
          onClick={protection.backup.start}
        />
      </header>

      <div className="card-grid">
        <Card wide title={t("applications.detail.overview.title")}>
          <dl className="definition-list">
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("applications.columns.engines")}</dt>
              <dd className="definition-list__value">
                {app.engines.length > 0 ? <bdi>{app.engines.join(", ")}</bdi> : "–"}
              </dd>
            </div>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("applications.detail.overview.storage")}</dt>
              <dd className="definition-list__value">
                <bdi>{formatBytes(totalBytes, i18n.language)}</bdi>
              </dd>
            </div>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("applications.detail.overview.createdAt")}</dt>
              <dd className="definition-list__value">{formatDateTime(app.createdAt, i18n.language)}</dd>
            </div>
          </dl>
        </Card>

        <Card wide title={t("applications.detail.workloads.title")}>
          {app.workloadItems.length === 0 ? (
            <p className="muted">{t("applications.detail.workloads.empty")}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">{t("applications.detail.workloads.name")}</th>
                  <th scope="col">{t("applications.detail.workloads.kind")}</th>
                  <th scope="col">{t("applications.detail.workloads.ready")}</th>
                  <th scope="col">{t("applications.detail.workloads.images")}</th>
                </tr>
              </thead>
              <tbody>
                {app.workloadItems.map((w) => (
                  <tr key={`${w.kind}/${w.name}`}>
                    <td className="table__primary">
                      <bdi>{w.name}</bdi>
                    </td>
                    <td data-label={t("applications.detail.workloads.kind")}>
                      <bdi>{w.kind}</bdi>
                    </td>
                    <td data-label={t("applications.detail.workloads.ready")}>
                      {t("applications.detail.workloads.readyOf", { ready: w.ready, desired: w.desired })}
                    </td>
                    <td data-label={t("applications.detail.workloads.images")} className="table__wrap">
                      <bdi>{w.images.join(", ")}</bdi>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>

        <Card wide title={t("applications.detail.volumes.title")}>
          {app.volumeItems.length === 0 ? (
            <p className="muted">{t("applications.detail.volumes.empty")}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">{t("applications.detail.volumes.name")}</th>
                  <th scope="col">{t("applications.detail.volumes.size")}</th>
                  <th scope="col">{t("applications.detail.volumes.storageClass")}</th>
                  <th scope="col">{t("applications.detail.volumes.status")}</th>
                </tr>
              </thead>
              <tbody>
                {app.volumeItems.map((v) => (
                  <tr key={v.name}>
                    <td className="table__primary">
                      <bdi>{v.name}</bdi>
                    </td>
                    <td data-label={t("applications.detail.volumes.size")}>
                      <bdi>{formatBytes(v.capacityBytes, i18n.language)}</bdi>
                    </td>
                    <td data-label={t("applications.detail.volumes.storageClass")}>
                      {v.storageClass ? <bdi>{v.storageClass}</bdi> : "–"}
                    </td>
                    <td data-label={t("applications.detail.volumes.status")}>
                      <VolumePhaseBadge phase={v.phase} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>

        <CopyProtection snapshots={protection.snapshotList} />

        <SnapshotCard
          state={protection.snapshots.state}
          backup={protection.backup.state}
          explanationId={explanationId}
          busyText={busyText}
          onRestore={protection.busy === null && !starting ? setToRestore : undefined}
          onDelete={protection.busy === null && !starting ? setToDelete : undefined}
          onRetryExport={protection.busy === null && !starting ? retryCopy : undefined}
          onReload={protection.snapshots.reload}
          onBackup={protection.backup.start}
          onLoadMore={protection.loadMoreSnapshots}
        />

        <ExportCard
          state={stored.state}
          onRestore={protection.busy === null && !starting ? setToRestoreStored : undefined}
          onReload={stored.reload}
          onLoadMore={() => setStoredPages((n) => n + 1)}
        />

        <RestoreCard
          state={protection.restores.state}
          snapshots={protection.snapshotList}
          onReload={protection.restores.reload}
          onLoadMore={protection.loadMoreRestores}
        />

        <Card title={t("applications.detail.configMaps.title")}>
          <NameList names={app.configMapNames} emptyText={t("applications.detail.configMaps.empty")} />
        </Card>

        <Card title={t("applications.detail.secrets.title")}>
          <NameList names={app.secretNames} emptyText={t("applications.detail.secrets.empty")} />
          {app.secretNames.length > 0 ? <p className="hint">{t("applications.detail.secrets.note")}</p> : null}
        </Card>
      </div>

      {toDelete ? (
        <DeleteSnapshotDialog snapshot={toDelete} state={deletion.state} onConfirm={confirmDelete} onClose={closeDeleteDialog} />
      ) : null}

      {toRestoreStored ? (
        <ExportRestoreDialog
          backup={toRestoreStored}
          applicationMissing={false}
          state={protection.restore.state}
          onConfirm={confirmStoredRestore}
          onClose={closeStoredDialog}
        />
      ) : null}

      {toRestore ? (
        <RestoreDialog snapshot={toRestore} state={protection.restore.state} onConfirm={confirmRestore} onClose={closeDialog} />
      ) : null}
    </div>
  );
}

function NameList({ names, emptyText }: { names: string[]; emptyText: string }) {
  if (names.length === 0) {
    return <p className="muted">{emptyText}</p>;
  }
  return (
    <ul className="name-list">
      {names.map((n) => (
        <li key={n}>
          <bdi>{n}</bdi>
        </li>
      ))}
    </ul>
  );
}
