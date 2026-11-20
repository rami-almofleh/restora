import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { formatDateTimeIn } from "../../../lib/format";
import type { PolicyInput } from "../api";
import { usePreview } from "../use-policies";

interface PolicyPreviewProps {
  input: PolicyInput;
}

/** Shows the next runs and the applications of the form as it is filled in, before anything is saved. */
export function PolicyPreview({ input }: PolicyPreviewProps) {
  const { t, i18n } = useTranslation();
  const { state } = usePreview(input);
  const zone = input.schedule.timezone || "UTC";

  return (
    <Card title={t("policies.preview.title")}>
      {state.status === "loading" ? (
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--row" />
        </div>
      ) : null}

      {state.status === "error" ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`policies.preview.errors.${state.error.code}`, { defaultValue: t("policies.preview.errors.unknown") })}</p>
            <details className="details">
              <summary>{t("common.technicalDetails")}</summary>
              <code>
                {state.error.code}: {state.error.message}
              </code>
            </details>
          </div>
        </div>
      ) : null}

      {state.status === "ready" ? (
        <div className="form-section">
          <div>
            <h3 className="preview__heading">{t("policies.preview.runs")}</h3>
            <ol className="preview__list">
              {state.data.nextRuns.map((run) => (
                <li key={run}>
                  <time dateTime={run}>{formatDateTimeIn(run, i18n.language, zone)}</time>
                </li>
              ))}
            </ol>
          </div>
          <div>
            <h3 className="preview__heading">{t("policies.preview.applications")}</h3>
            {state.data.applications.length > 0 ? (
              <p>
                <bdi>{state.data.applications.join(", ")}</bdi>
              </p>
            ) : (
              <p className="muted">{t("policies.preview.noApplications")}</p>
            )}
            {state.data.missing.length > 0 ? (
              <p className="muted">{t("policies.card.missing", { names: state.data.missing.join(", ") })}</p>
            ) : null}
          </div>
        </div>
      ) : null}
    </Card>
  );
}
