import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import type { SelectorMode } from "../api";
import type { FormProblem, PolicyForm } from "../form-model";
import { useApplicationNames } from "../use-policies";
import { Field } from "../../../components/Field";

interface SelectorFieldsProps {
  form: PolicyForm;
  problems: readonly FormProblem[];
  onChange: (change: Partial<PolicyForm>) => void;
}

/** Which applications the policy protects: named ones, those with a label, or all. */
export function SelectorFields({ form, problems, onChange }: SelectorFieldsProps) {
  const { t } = useTranslation();
  const { state, reload } = useApplicationNames();
  const options: readonly SegmentedOption<SelectorMode>[] = [
    { value: "applications", label: t("policies.form.selector.applications") },
    { value: "labels", label: t("policies.form.selector.labels") },
    { value: "all", label: t("policies.form.selector.all") },
  ];

  const toggle = (name: string, checked: boolean) =>
    onChange({ applications: checked ? [...form.applications, name] : form.applications.filter((a) => a !== name) });

  return (
    <div className="form-section">
      <SegmentedControl label={t("policies.form.selector.label")} name="policy-selector" value={form.mode} options={options} onChange={(mode) => onChange({ mode })} />

      {form.mode === "applications" ? (
        <div className="field">
          <span className="field__label" id="policy-applications-label">
            {t("policies.form.applications.label")}
          </span>
          {state.status === "loading" ? (
            <div className="skeleton skeleton--row" aria-busy="true">
              <span className="sr-only">{t("common.loading")}</span>
            </div>
          ) : null}
          {state.status === "error" ? (
            <div className="notice notice--danger" role="alert">
              <div>
                <p>{t("policies.form.applications.error")}</p>
                <button type="button" className="button button--secondary" onClick={reload}>
                  <RefreshCw size={18} aria-hidden="true" />
                  {t("common.retry")}
                </button>
              </div>
            </div>
          ) : null}
          {state.status === "ready" && state.data.length === 0 ? <p className="muted">{t("policies.form.applications.none")}</p> : null}
          {state.status === "ready" && state.data.length > 0 ? (
            <ul className="checklist" role="group" aria-labelledby="policy-applications-label">
              {state.data.map((name) => (
                <li key={name}>
                  <label className="checklist__item">
                    <input type="checkbox" checked={form.applications.includes(name)} onChange={(e) => toggle(name, e.target.checked)} />
                    <bdi>{name}</bdi>
                  </label>
                </li>
              ))}
            </ul>
          ) : null}
          {problems.includes("applications") ? <p className="field__error">{t("policies.form.problems.applications")}</p> : null}
        </div>
      ) : null}

      {form.mode === "labels" ? (
        <Field
          label={t("policies.form.labels.label")}
          hint={t("policies.form.labels.hint")}
          error={problems.includes("labels") ? t("policies.form.problems.labels") : undefined}
        >
          {({ id, describedBy, invalid }) => (
            <input
              id={id}
              className="field__control"
              dir="ltr"
              value={form.labels}
              placeholder="team=backend"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              aria-describedby={describedBy}
              aria-invalid={invalid}
              onChange={(e) => onChange({ labels: e.target.value })}
            />
          )}
        </Field>
      ) : null}

      {form.mode === "all" ? <p className="muted">{t("policies.form.selector.allHint")}</p> : null}
    </div>
  );
}
