import { RefreshCw, ShieldCheck, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Navigate } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";
import { StatusBadge } from "../../../components/StatusBadge";
import { formatDateTime } from "../../../lib/format";
import { SetupBanner } from "../../setup/components/SetupBanner";
import { computeProgress } from "../../setup/progress";
import { readFlag } from "../../setup/storage";
import { useSetupFacts } from "../../setup/use-setup";
import { useSystemInfo } from "../use-system-info";

export function OverviewPage() {
  const { t, i18n } = useTranslation();
  const { state, reload } = useSystemInfo();
  const setup = useSetupFacts();

  // A fresh installation opens the setup once. After that, the banner below is the way back to it.
  const facts = setup.state.status === "ready" ? setup.state.data : undefined;
  if (facts && facts.locations.length === 0 && facts.policies.length === 0 && !readFlag("seen") && !readFlag("dismissed")) {
    return <Navigate to="/setup" replace />;
  }
  const progress = facts ? computeProgress({ ...facts, restoreSkipped: readFlag("restoreSkipped") }) : undefined;

  return (
    <>
      <PageHeader title={t("overview.title")} subtitle={t("overview.subtitle")} />
      {progress && !readFlag("dismissed") ? (
        <div className="setup-banner">
          <SetupBanner progress={progress} />
        </div>
      ) : null}
      <div className="card-grid">
        <Card
          title={t("overview.system.title")}
          aside={
            state.status === "ready" ? (
              <StatusBadge tone="success" label={t("overview.system.running")} />
            ) : state.status === "error" ? (
              <StatusBadge tone="danger" label={t("overview.system.unreachable")} />
            ) : null
          }
        >
          {state.status === "loading" ? (
            <div className="skeleton-stack" aria-busy="true">
              <span className="sr-only">{t("common.loading")}</span>
              <div className="skeleton skeleton--line" />
              <div className="skeleton skeleton--line" />
            </div>
          ) : null}

          {state.status === "error" ? (
            <StateMessage
              tone="danger"
              icon={TriangleAlert}
              title={t("overview.system.errorTitle")}
              text={`${t("overview.system.errorText")} ${t("overview.system.errorHint")}`}
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
          ) : null}

          {state.status === "ready" ? (
            <dl className="definition-list">
              <div className="definition-list__row">
                <dt className="definition-list__term">{t("overview.system.version")}</dt>
                <dd className="definition-list__value">{state.data.version}</dd>
              </div>
              <div className="definition-list__row">
                <dt className="definition-list__term">{t("overview.system.startedAt")}</dt>
                <dd className="definition-list__value">{formatDateTime(state.data.startedAt, i18n.language)}</dd>
              </div>
            </dl>
          ) : null}
        </Card>

        <Card title={t("overview.protection.title")}>
          <StateMessage
            icon={ShieldCheck}
            title={t("overview.protection.emptyTitle")}
            text={t("overview.protection.emptyText")}
          />
        </Card>
      </div>
    </>
  );
}
