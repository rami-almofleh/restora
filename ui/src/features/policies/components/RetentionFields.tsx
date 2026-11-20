import { useTranslation } from "react-i18next";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import { RETENTION_KEYS } from "../api";
import type { FormProblem, PolicyForm } from "../form-model";
import { Field } from "../../../components/Field";

interface RetentionFieldsProps {
  form: PolicyForm;
  problems: readonly FormProblem[];
  onChange: (change: Partial<PolicyForm>) => void;
}

/** Which backups of the policy stay: all of them, or the newest ones by the rules of the numbers. */
export function RetentionFields({ form, problems, onChange }: RetentionFieldsProps) {
  const { t } = useTranslation();
  const options: readonly SegmentedOption<"all" | "rules">[] = [
    { value: "all", label: t("policies.form.retention.all") },
    { value: "rules", label: t("policies.form.retention.rules") },
  ];

  return (
    <div className="form-section">
      <SegmentedControl
        label={t("policies.form.retention.label")}
        name="policy-retention"
        value={form.keepAll ? "all" : "rules"}
        options={options}
        onChange={(value) => onChange({ keepAll: value === "all" })}
      />

      {form.keepAll ? <p className="muted">{t("policies.form.retention.allHint")}</p> : null}

      {form.keepAll ? null : (
        <>
          <p className="muted">{t("policies.form.retention.hint")}</p>
          <div className="field-grid">
            {RETENTION_KEYS.map((key) => (
              <Field key={key} label={t(`policies.retention.labels.${key}`)}>
                {({ id, invalid }) => (
                  <input
                    id={id}
                    className="field__control"
                    type="number"
                    inputMode="numeric"
                    min={0}
                    max={1000}
                    placeholder="0"
                    value={form.keep[key]}
                    aria-invalid={invalid || problems.includes("retention")}
                    onChange={(e) => onChange({ keep: { ...form.keep, [key]: e.target.value } })}
                  />
                )}
              </Field>
            ))}
          </div>
          {problems.includes("retention") ? <p className="field__error">{t("policies.form.problems.retention")}</p> : null}
        </>
      )}
    </div>
  );
}
