import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import type { ApiError } from "../../../lib/api";
import type { Policy } from "../api";

interface DeletePolicyDialogProps {
  policy: Policy;
  deleting: boolean;
  error?: ApiError;
  onConfirm: () => void;
  onClose: () => void;
}

/** Says what deleting does: the policy stops, its backups stay. Cancel comes first and holds the focus. */
export function DeletePolicyDialog({ policy, deleting, error, onConfirm, onClose }: DeletePolicyDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      title={t("policies.delete.title")}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t("policies.delete.cancel")}
          </button>
          <button type="button" className="button button--danger" onClick={onConfirm} disabled={deleting}>
            {deleting ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
            {t("policies.delete.confirm")}
          </button>
        </>
      }
    >
      <p>{t("policies.delete.intro", { name: policy.name })}</p>
      <ul className="consequences">
        <li>{t("policies.delete.stops")}</li>
        <li>{t("policies.delete.keeps")}</li>
      </ul>
      {error ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t("policies.delete.failed")}</p>
            <p className="muted">{error.hint ?? error.message}</p>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
