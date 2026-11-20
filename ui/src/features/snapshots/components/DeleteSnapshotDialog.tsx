import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import { formatDateTime } from "../../../lib/format";
import type { Snapshot } from "../api";
import type { DeleteState } from "../use-snapshots";

interface DeleteSnapshotDialogProps {
  snapshot: Snapshot;
  state: DeleteState;
  onConfirm: () => void;
  onClose: () => void;
}

/** Asks before a backup is deleted for good. Cancel comes first in the DOM and holds the focus. */
export function DeleteSnapshotDialog({ snapshot, state, onConfirm, onClose }: DeleteSnapshotDialogProps) {
  const { t, i18n } = useTranslation();
  const deleting = state.status === "deleting";
  const restorable = snapshot.phase === "Succeeded";

  return (
    <Dialog
      title={t("snapshots.delete.title")}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t("snapshots.delete.cancel")}
          </button>
          <button type="button" className="button button--danger" onClick={onConfirm} disabled={deleting}>
            {deleting ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
            {t("snapshots.delete.confirm")}
          </button>
        </>
      }
    >
      <p>
        {t("snapshots.delete.intro", {
          application: snapshot.application,
          time: formatDateTime(snapshot.createdAt, i18n.language),
        })}
      </p>
      <ul className="consequences">
        {restorable ? <li>{t("snapshots.delete.loses")}</li> : null}
        <li>{t("snapshots.delete.permanent")}</li>
      </ul>
      {state.status === "error" ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`snapshots.delete.errors.${state.error.code}`, { defaultValue: t("snapshots.delete.errors.unknown") })}</p>
            <p className="muted">{state.error.hint ?? state.error.message}</p>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
