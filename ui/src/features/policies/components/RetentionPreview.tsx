import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { PolicyInput } from "../api";
import { useRetentionPlan } from "../use-policies";
import { RetentionPlanList } from "./RetentionPlanList";

interface RetentionPreviewProps {
  policyId: string;
  input: PolicyInput;
}

/** Shows, before saving, how many of the backups the policy has made so far the new rules would delete. */
export function RetentionPreview({ policyId, input }: RetentionPreviewProps) {
  const { t } = useTranslation();
  const { state } = useRetentionPlan(policyId, input);

  if (state.status === "loading") {
    return (
      <div className="skeleton skeleton--row" aria-busy="true">
        <span className="sr-only">{t("common.loading")}</span>
      </div>
    );
  }
  if (state.status === "error") {
    return (
      <div className="notice notice--danger" role="alert">
        <TriangleAlert size={18} aria-hidden="true" />
        <p>{t("policies.retention.previewFailed")}</p>
      </div>
    );
  }

  const { delete: doomed, keep, busy } = state.data;
  return (
    <div className="retention-preview" aria-live="polite">
      <h3 className="preview__heading">{t("policies.retention.previewTitle")}</h3>
      {doomed.length === 0 ? (
        <p>{t("policies.retention.deletesNothing")}</p>
      ) : (
        <>
          <p className="retention-preview__warning">{t("policies.retention.deletes", { n: doomed.length, kept: keep.length })}</p>
          <RetentionPlanList plan={state.data} />
        </>
      )}
      {busy.length > 0 ? <p className="muted">{t("policies.retention.busy", { names: busy.join(", ") })}</p> : null}
    </div>
  );
}
