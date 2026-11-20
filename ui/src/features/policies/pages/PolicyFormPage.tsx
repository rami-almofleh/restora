import { ArrowLeft, RefreshCw, SearchX, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";
import { PolicyFormView } from "../components/PolicyFormView";
import { emptyForm, formFromPolicy } from "../form-model";
import { browserTimezone } from "../timezones";
import { usePolicy } from "../use-policies";

function BackLink() {
  const { t } = useTranslation();
  return (
    <Link className="back-link" to="/policies">
      <ArrowLeft size={18} aria-hidden="true" />
      {t("policies.back")}
    </Link>
  );
}

/** The page of /policies/new. */
export function NewPolicyPage() {
  const { t } = useTranslation();
  return (
    <>
      <BackLink />
      <PageHeader title={t("policies.form.titleNew")} subtitle={t("policies.form.subtitle")} />
      <PolicyFormView initial={emptyForm(browserTimezone())} />
    </>
  );
}

/** The page of /policies/:id. */
export function EditPolicyPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const { state, reload } = usePolicy(id);

  if (state.status === "loading") {
    return (
      <>
        <BackLink />
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--title" />
          <div className="skeleton skeleton--row" />
        </div>
      </>
    );
  }

  if (state.status === "error") {
    const notFound = state.error.code === "policy_not_found";
    return (
      <>
        <BackLink />
        {notFound ? (
          <StateMessage icon={SearchX} title={t("policies.notFound.title")} text={t("policies.notFound.text")}>
            <div className="state__actions">
              <Link className="button button--primary" to="/policies">
                {t("policies.back")}
              </Link>
            </div>
          </StateMessage>
        ) : (
          <StateMessage tone="danger" icon={TriangleAlert} title={t("policies.error.title")} text={`${t("policies.error.text")} ${t("policies.error.hint")}`}>
            <div className="state__actions">
              <button type="button" className="button button--secondary" onClick={reload}>
                <RefreshCw size={18} aria-hidden="true" />
                {t("common.retry")}
              </button>
            </div>
          </StateMessage>
        )}
      </>
    );
  }

  return (
    <>
      <BackLink />
      <PageHeader title={t("policies.form.titleEdit")} subtitle={state.data.name} />
      <PolicyFormView id={state.data.id} initial={formFromPolicy(state.data)} />
    </>
  );
}
