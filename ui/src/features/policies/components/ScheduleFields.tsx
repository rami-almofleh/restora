import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import type { ScheduleType } from "../api";
import type { FormProblem, PolicyForm } from "../form-model";
import { weekdayName } from "../schedule";
import { timezoneOptions } from "../timezones";
import { Field } from "../../../components/Field";

interface ScheduleFieldsProps {
  form: PolicyForm;
  problems: readonly FormProblem[];
  onChange: (change: Partial<PolicyForm>) => void;
}

const WEEKDAYS = [1, 2, 3, 4, 5, 6, 0] as const;

/** When the policy runs: a preset with its few fields, or a cron expression. */
export function ScheduleFields({ form, problems, onChange }: ScheduleFieldsProps) {
  const { t, i18n } = useTranslation();
  const zones = useMemo(() => timezoneOptions([form.timezone]), [form.timezone]);
  const options: readonly SegmentedOption<ScheduleType>[] = [
    { value: "hourly", label: t("policies.form.schedule.hourly") },
    { value: "daily", label: t("policies.form.schedule.daily") },
    { value: "weekly", label: t("policies.form.schedule.weekly") },
    { value: "custom", label: t("policies.form.schedule.custom") },
  ];

  return (
    <div className="form-section">
      <SegmentedControl label={t("policies.form.schedule.label")} name="policy-schedule" value={form.scheduleType} options={options} onChange={(scheduleType) => onChange({ scheduleType })} />

      {form.scheduleType === "hourly" ? (
        <Field
          label={t("policies.form.minute.label")}
          hint={t("policies.form.minute.hint")}
          error={problems.includes("minute") ? t("policies.form.problems.minute") : undefined}
        >
          {({ id, describedBy, invalid }) => (
            <input
              id={id}
              className="field__control field__control--short"
              type="number"
              inputMode="numeric"
              min={0}
              max={59}
              value={form.minute}
              aria-describedby={describedBy}
              aria-invalid={invalid}
              onChange={(e) => onChange({ minute: e.target.value })}
            />
          )}
        </Field>
      ) : null}

      {form.scheduleType === "weekly" ? (
        <Field label={t("policies.form.weekday.label")}>
          {({ id }) => (
            <select id={id} className="field__control" value={form.weekday} onChange={(e) => onChange({ weekday: e.target.value })}>
              {WEEKDAYS.map((day) => (
                <option key={day} value={day}>
                  {weekdayName(day, i18n.language)}
                </option>
              ))}
            </select>
          )}
        </Field>
      ) : null}

      {form.scheduleType === "daily" || form.scheduleType === "weekly" ? (
        <Field label={t("policies.form.time.label")}>
          {({ id }) => (
            <input id={id} className="field__control field__control--short" type="time" value={form.time} required onChange={(e) => onChange({ time: e.target.value })} />
          )}
        </Field>
      ) : null}

      {form.scheduleType === "custom" ? (
        <Field
          label={t("policies.form.cron.label")}
          hint={t("policies.form.cron.hint")}
          error={problems.includes("cron") ? t("policies.form.problems.cron") : undefined}
        >
          {({ id, describedBy, invalid }) => (
            <input
              id={id}
              className="field__control"
              dir="ltr"
              value={form.cron}
              placeholder="*/15 * * * *"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              aria-describedby={describedBy}
              aria-invalid={invalid}
              onChange={(e) => onChange({ cron: e.target.value })}
            />
          )}
        </Field>
      ) : null}

      <Field label={t("policies.form.timezone.label")} hint={t("policies.form.timezone.hint")}>
        {({ id, describedBy }) => (
          <select id={id} className="field__control" dir="ltr" value={form.timezone} aria-describedby={describedBy} onChange={(e) => onChange({ timezone: e.target.value })}>
            {zones.map((zone) => (
              <option key={zone} value={zone}>
                {zone}
              </option>
            ))}
          </select>
        )}
      </Field>
    </div>
  );
}
