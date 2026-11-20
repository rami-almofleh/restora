import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import { formatDateTime } from "../../../lib/format";
import type { Snapshot } from "../../snapshots/api";
import type { RestoreRequestState } from "../use-restores";

interface RestoreDialogProps {
  snapshot: Snapshot;
  state: RestoreRequestState;
  onConfirm: () => void;
  onClose: () => void;
}

/**
 * Asks before data is replaced. It names exactly what will happen, so nobody confirms by habit.
 * The cancel button comes first in the DOM and therefore holds the focus when the dialog opens.
 */
export function RestoreDialog({ snapshot, state, onConfirm, onClose }: RestoreDialogProps) {
  const { t, i18n } = useTranslation();
  const starting = state.status === "starting";
  const volumes = snapshot.volumes.map((v) => v.claim);

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
        {t("restores.dialog.intro", {
          application: snapshot.application,
          time: formatDateTime(snapshot.createdAt, i18n.language),
        })}
      </p>

      <ul className="consequences">
        {volumes.length > 0 ? (
          <li>{t("restores.dialog.volumes", { volumes: volumes.join(", ") })}</li>
        ) : (
          <li>{t("restores.dialog.noVolumes")}</li>
        )}
        <li>{t("restores.dialog.resources", { resources: snapshot.resources?.count ?? 0 })}</li>
        {volumes.length > 0 ? <li>{t("restores.dialog.downtime")}</li> : null}
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
