import { ArrowLeft, CircleCheck, RefreshCw, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";
import { LocationStep } from "../components/LocationStep";
import { PolicyStep } from "../components/PolicyStep";
import { RestoreStep } from "../components/RestoreStep";
import { RunStep } from "../components/RunStep";
import { StepIndicator } from "../components/StepIndicator";
import { computeProgress } from "../progress";
import { readFlag, writeFlag } from "../storage";
import { useSetupFacts } from "../use-setup";

/** While the setup is open it looks at the cluster every few seconds, so a test or a backup moves it on by itself. */
const REFRESH_MS = 3_000;

export function SetupPage() {
  const { t } = useTranslation();
  const { state, reload } = useSetupFacts(REFRESH_MS);
  const [restoreSkipped, setRestoreSkipped] = useState(() => readFlag("restoreSkipped"));

  useEffect(() => {
    // Whoever has seen the setup is not sent here again by the overview.
    writeFlag("seen", true);
  }, []);

  const skipRestore = () => {
    writeFlag("restoreSkipped", true);
    setRestoreSkipped(true);
  };
  const progress = state.status === "ready" ? computeProgress({ ...state.data, restoreSkipped }) : undefined;

  return (
    <>
      <Link className="back-link" to="/">
        <ArrowLeft size={18} aria-hidden="true" />
        {t("setup.later")}
      </Link>
      <PageHeader title={t("setup.title")} subtitle={t("setup.subtitle")} />

      {state.status === "loading" ? (
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
        </div>
      ) : null}

      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("setup.error.title")} text={t("setup.error.text")}>
          <div className="state__actions">
            <button type="button" className="button button--secondary" onClick={reload}>
              <RefreshCw size={18} aria-hidden="true" />
              {t("common.retry")}
            </button>
          </div>
        </StateMessage>
      ) : null}

      {state.status === "ready" && progress ? (
        <div className="setup">
          <StepIndicator progress={progress} />

          {progress.current === "done" ? (
            <Card wide title={t("setup.done.title")}>
              <div className="setup-done">
                <CircleCheck size={32} aria-hidden="true" />
                <p>{t("setup.done.text")}</p>
              </div>
              <div className="card__actions">
                <Link className="button button--primary" to="/policies">
                  {t("setup.done.policies")}
                </Link>
                <Link className="button button--secondary" to="/activity">
                  {t("setup.done.activity")}
                </Link>
              </div>
            </Card>
          ) : (
            <section className="setup-step" aria-labelledby="setup-step-title">
              <h2 className="setup-step__title" id="setup-step-title">
                {t(`setup.${progress.current}.title`)}
              </h2>
              {progress.current === "location" ? <LocationStep locations={state.data.locations} onChanged={reload} /> : null}
              {progress.current === "policy" && progress.location ? <PolicyStep location={progress.location} onSaved={reload} /> : null}
              {progress.current === "run" && progress.policy ? <RunStep policy={progress.policy} onChanged={reload} /> : null}
              {progress.current === "restore" && progress.policy ? (
                <RestoreStep application={progress.policy.applications[0] ?? ""} onSkip={skipRestore} onChanged={reload} />
              ) : null}
            </section>
          )}
        </div>
      ) : null}
    </>
  );
}
