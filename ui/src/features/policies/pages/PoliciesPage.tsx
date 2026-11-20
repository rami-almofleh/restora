import { CalendarClock, CircleCheck, Plus, RefreshCw, TriangleAlert } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { StateMessage } from "../../../components/StateMessage";
import { useLocations } from "../../locations/use-locations";
import type { Policy } from "../api";
import { DeletePolicyDialog } from "../components/DeletePolicyDialog";
import { PolicyCard } from "../components/PolicyCard";
import { usePolicies, usePolicyActions } from "../use-policies";

export function PoliciesPage() {
  const { t } = useTranslation();
  const [pages, setPages] = useState(1);
  const [toDelete, setToDelete] = useState<Policy | null>(null);
  const { state, reload } = usePolicies(pages);
  const locations = useLocations(1);
  const locationNames: Record<string, string> =
    locations.state.status === "ready" ? Object.fromEntries(locations.state.data.items.map((l) => [l.id, l.name])) : {};
  const actions = usePolicyActions(useCallback(() => reload(), [reload]));
  // The empty state has its own button, so the page does not offer a second main action next to it.
  const isEmpty = state.status === "ready" && state.data.items.length === 0;
  const workingId = actions.state.status === "working" ? actions.state.id : null;

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
          <h1 className="page-header__title">{t("policies.title")}</h1>
          <p className="page-header__subtitle">{t("policies.subtitle")}</p>
        </div>
        {isEmpty ? null : (
          <div className="action-bar">
            <Link className="button button--primary action-bar__button" to="/policies/new">
              <Plus size={18} aria-hidden="true" />
              {t("policies.create")}
            </Link>
          </div>
        )}
      </header>

      {actions.state.status === "started" ? (
        <div className="notice notice--success" role="status">
          <CircleCheck size={18} aria-hidden="true" />
          <div>
            <p>{t("policies.run.started", { n: actions.state.result.snapshots.length })}</p>
            {actions.state.result.skipped.length > 0 ? (
              <ul className="name-list">
                {actions.state.result.skipped.map((skipped) => (
                  <li key={skipped.application}>
                    <bdi>{skipped.application}</bdi>: {t(`policies.skipped.${skipped.reason}`, { defaultValue: t("policies.skipped.request_failed") })}
                  </li>
                ))}
              </ul>
            ) : null}
            <Link className="job__link" to="/activity">
              {t("policies.run.toActivity")}
            </Link>
          </div>
        </div>
      ) : null}

      {actions.state.status === "error" && !toDelete ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t("policies.actionFailed")}</p>
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
        <StateMessage tone="danger" icon={TriangleAlert} title={t("policies.error.title")} text={`${t("policies.error.text")} ${t("policies.error.hint")}`}>
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
        <StateMessage icon={CalendarClock} title={t("policies.empty.title")} text={t("policies.empty.text")}>
          <div className="state__actions">
            <Link className="button button--primary" to="/policies/new">
              <Plus size={18} aria-hidden="true" />
              {t("policies.create")}
            </Link>
          </div>
        </StateMessage>
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <>
          <div className="card-grid">
            {state.data.items.map((policy) => (
              <PolicyCard
                key={policy.id}
                policy={policy}
                busy={workingId === policy.id}
                locationNames={locationNames}
                onRun={(p) => void actions.runNow(p.id)}
                onTogglePause={(p) => void actions.setPaused(p.id, !p.paused)}
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
        <DeletePolicyDialog
          policy={toDelete}
          deleting={workingId === toDelete.id}
          error={actions.state.status === "error" ? actions.state.error : undefined}
          onConfirm={confirmDelete}
          onClose={closeDelete}
        />
      ) : null}
    </div>
  );
}
