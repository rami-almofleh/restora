import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { fetchLocations } from "../../locations/api";
import { useResource } from "../../../lib/use-resource";
import { useCallback } from "react";
import type { PolicyForm } from "../form-model";

interface ExportFieldsProps {
  form: PolicyForm;
  onChange: (change: Partial<PolicyForm>) => void;
}

const MAX_TARGETS = 5;

/** Which storage locations a backup is copied to. None keeps the backups in the cluster. */
export function ExportFields({ form, onChange }: ExportFieldsProps) {
  const { t } = useTranslation();
  const load = useCallback((signal: AbortSignal) => fetchLocations(1, signal), []);
  const { state, reload } = useResource(load, "locations");

  const toggle = (id: string, checked: boolean) =>
    onChange({ exportTo: checked ? [...form.exportTo, id] : form.exportTo.filter((l) => l !== id) });

  return (
    <div className="form-section">
      <p className="muted">{t("policies.form.export.hint")}</p>

      {state.status === "loading" ? (
        <div className="skeleton skeleton--row" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
        </div>
      ) : null}

      {state.status === "error" ? (
        <div className="notice notice--danger" role="alert">
          <div>
            <p>{t("policies.form.export.error")}</p>
            <button type="button" className="button button--secondary" onClick={reload}>
              <RefreshCw size={18} aria-hidden="true" />
              {t("common.retry")}
            </button>
          </div>
        </div>
      ) : null}

      {state.status === "ready" && state.data.items.length === 0 ? (
        <div className="notice" role="status">
          <p>
            {t("policies.form.export.none")}{" "}
            <Link className="job__link" to="/locations/new">
              {t("policies.form.export.create")}
            </Link>
          </p>
        </div>
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <ul className="checklist" role="group" aria-label={t("policies.form.export.label")}>
          {state.data.items.map((l) => {
            const checked = form.exportTo.includes(l.id);
            const limitReached = !checked && form.exportTo.length >= MAX_TARGETS;
            return (
              <li key={l.id}>
                <label className="checklist__item">
                  <input type="checkbox" checked={checked} disabled={limitReached} onChange={(e) => toggle(l.id, e.target.checked)} />
                  <span className="checklist__text">
                    <bdi>{l.name}</bdi>
                    <span className="checklist__hint">
                      {t(`locations.type.${l.type}`)}
                      {l.test.state === "failed" ? ` · ${t("policies.form.export.testFailed")}` : ""}
                    </span>
                  </span>
                </label>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}
