import { ArrowLeft, RefreshCw, SearchX, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";
import { LocationFormView } from "../components/LocationFormView";
import { emptyForm, formFromLocation } from "../form-model";
import { useLocation } from "../use-locations";

function BackLink() {
  const { t } = useTranslation();
  return (
    <Link className="back-link" to="/locations">
      <ArrowLeft size={18} aria-hidden="true" />
      {t("locations.back")}
    </Link>
  );
}

/** The page of /locations/new. */
export function NewLocationPage() {
  const { t } = useTranslation();
  return (
    <>
      <BackLink />
      <PageHeader title={t("locations.form.titleNew")} subtitle={t("locations.form.subtitle")} />
      <LocationFormView initial={emptyForm()} />
    </>
  );
}

/** The page of /locations/:id. */
export function EditLocationPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const { state, reload } = useLocation(id);

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
    const notFound = state.error.code === "location_not_found";
    return (
      <>
        <BackLink />
        {notFound ? (
          <StateMessage icon={SearchX} title={t("locations.notFound.title")} text={t("locations.notFound.text")}>
            <div className="state__actions">
              <Link className="button button--primary" to="/locations">
                {t("locations.back")}
              </Link>
            </div>
          </StateMessage>
        ) : (
          <StateMessage tone="danger" icon={TriangleAlert} title={t("locations.error.title")} text={`${t("locations.error.text")} ${t("locations.error.hint")}`}>
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
      <PageHeader title={t("locations.form.titleEdit")} subtitle={state.data.name} />
      <LocationFormView id={state.data.id} credentialsSet={state.data.s3?.credentialsSet ?? false} initial={formFromLocation(state.data)} />
    </>
  );
}
