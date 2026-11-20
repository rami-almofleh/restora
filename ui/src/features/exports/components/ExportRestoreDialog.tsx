import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import { formatBytes, formatDateTime } from "../../../lib/format";
import type { RestoreRequestState } from "../../restores/use-restores";
import type { StoredBackup } from "../api";

interface ExportRestoreDialogProps {
  backup: StoredBackup;
  /** The application does not exist in the cluster any more: its namespace is created again. */
  applicationMissing: boolean;
  state: RestoreRequestState;
  onConfirm: () => void;
  onClose: () => void;
}

/**
 * Asks before data is replaced by a backup from a storage location. It names exactly what will happen.
 * The cancel button comes first in the DOM and therefore holds the focus when the dialog opens.
 */
export function ExportRestoreDialog({ backup, applicationMissing, state, onConfirm, onClose }: ExportRestoreDialogProps) {
  const { t, i18n } = useTranslation();
  const starting = state.status === "starting";
  const volumes = backup.volumes.map((v) => v.claim);

  return (
    <Dialog
      title={t("restores.dialog.title")}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t("restores.dialog.cancel")}
          </button>
          <button type="button" className="button button--danger" onClick={onConfirm} disabled={starting}>
            {starting ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
            {starting ? t("restores.dialog.starting") : t("restores.dialog.confirm")}
          </button>
        </>
      }
    >
      <p>
        {t("restores.exportDialog.intro", {
          application: backup.application,
          location: backup.location.name,
          time: formatDateTime(backup.createdAt, i18n.language),
        })}
      </p>

      <ul className="consequences">
        {applicationMissing ? <li>{t("restores.exportDialog.missing", { application: backup.application })}</li> : null}
        {volumes.length > 0 ? (
          <li>{t(applicationMissing ? "restores.exportDialog.volumesNew" : "restores.dialog.volumes", { volumes: volumes.join(", ") })}</li>
        ) : (
          <li>{t("restores.dialog.noVolumes")}</li>
        )}
        <li>{t("restores.dialog.resources", { resources: backup.resources })}</li>
        {volumes.length > 0 ? <li>{t("restores.exportDialog.duration", { size: formatBytes(backup.sizeBytes, i18n.language) })}</li> : null}
        {volumes.length > 0 && !applicationMissing ? <li>{t("restores.dialog.downtime")}</li> : null}
      </ul>

      {state.status === "error" ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`restores.errors.${state.error.code}.text`, { defaultValue: t("restores.dialog.startFailed") })}</p>
            <p className="muted">{state.error.hint ?? state.error.message}</p>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
