import { LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import type { RetentionPlan } from "../api";
import { RetentionPlanList } from "./RetentionPlanList";

interface ConfirmRetentionDialogProps {
  plan: RetentionPlan;
  saving: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

/** Saving these rules deletes backups for good. This asks first and names them. Cancel holds the focus. */
export function ConfirmRetentionDialog({ plan, saving, onConfirm, onClose }: ConfirmRetentionDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      title={t("policies.retention.confirm.title")}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t("policies.retention.confirm.cancel")}
          </button>
          <button type="button" className="button button--danger" onClick={onConfirm} disabled={saving}>
            {saving ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
            {t("policies.retention.confirm.confirm", { n: plan.delete.length })}
          </button>
        </>
      }
    >
      <p>{t("policies.retention.confirm.intro", { n: plan.delete.length, kept: plan.keep.length })}</p>
      <RetentionPlanList plan={plan} />
      <p className="muted">{t("policies.retention.confirm.permanent")}</p>
    </Dialog>
  );
}
