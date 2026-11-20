import { HardDrive, Plus, RefreshCw, TriangleAlert } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { StateMessage } from "../../../components/StateMessage";
import type { Location } from "../api";
import { DeleteLocationDialog } from "../components/DeleteLocationDialog";
import { LocationCard } from "../components/LocationCard";
import { useLocationActions, useLocations } from "../use-locations";

export function LocationsPage() {
  const { t } = useTranslation();
  const [pages, setPages] = useState(1);
  const [toDelete, setToDelete] = useState<Location | null>(null);
  const { state, reload } = useLocations(pages);
  const actions = useLocationActions(useCallback(() => reload(), [reload]));
  const workingId = actions.state.status === "working" ? actions.state.id : null;
  // The empty state has its own button, so the page does not offer a second main action next to it.
  const isEmpty = state.status === "ready" && state.data.items.length === 0;

  const confirmDelete = () => {
    if (toDelete) {
      void actions.remove(toDelete.id).then((deleted) => {
        if (deleted) setToDelete(null);
      });
    }
  };
  const closeDelete = () => {
    actions.dismiss();
    setToDelete(null);
  };

  return (
    <div className="has-action-bar">
      <header className="page-header page-header--with-badge">
        <div>
          <h1 className="page-header__title">{t("locations.title")}</h1>
          <p className="page-header__subtitle">{t("locations.subtitle")}</p>
        </div>
        {isEmpty ? null : (
          <div className="action-bar">
            <Link className="button button--primary action-bar__button" to="/locations/new">
              <Plus size={18} aria-hidden="true" />
              {t("locations.create")}
            </Link>
          </div>
        )}
      </header>

      {actions.state.status === "error" && !toDelete ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t("locations.actionFailed")}</p>
            <p className="muted">{actions.state.error.hint ?? actions.state.error.message}</p>
          </div>
        </div>
      ) : null}

      {state.status === "loading" ? (
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
        </div>
      ) : null}

      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("locations.error.title")} text={`${t("locations.error.text")} ${t("locations.error.hint")}`}>
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

      {isEmpty ? (
        <StateMessage icon={HardDrive} title={t("locations.empty.title")} text={t("locations.empty.text")}>
          <div className="state__actions">
            <Link className="button button--primary" to="/locations/new">
              <Plus size={18} aria-hidden="true" />
              {t("locations.create")}
            </Link>
          </div>
        </StateMessage>
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <>
          <div className="card-grid">
            {state.data.items.map((location) => (
              <LocationCard
                key={location.id}
                location={location}
                busy={workingId === location.id}
                onTest={(l) => void actions.test(l.id)}
                onDelete={setToDelete}
              />
            ))}
          </div>
          {state.data.hasMore ? (
            <div className="load-more">
              <button type="button" className="button button--secondary" onClick={() => setPages((n) => n + 1)}>
                {t("common.loadMore")}
              </button>
            </div>
          ) : null}
        </>
      ) : null}

      {toDelete ? (
        <DeleteLocationDialog
          location={toDelete}
          deleting={workingId === toDelete.id}
          error={actions.state.status === "error" ? actions.state.error : undefined}
          onConfirm={confirmDelete}
          onClose={closeDelete}
        />
      ) : null}
    </div>
  );
}
