import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { Card } from "../../../components/Card";
import { useDebouncedValue } from "../../../lib/use-debounced-value";
import { previewRetention, type RetentionPlan } from "../api";
import { findProblems, toInput, type PolicyForm } from "../form-model";
import { useSavePolicy } from "../use-policies";
import { ConfirmRetentionDialog } from "./ConfirmRetentionDialog";
import { ExportFields } from "./ExportFields";
import { Field } from "../../../components/Field";
import { PolicyPreview } from "./PolicyPreview";
import { RetentionFields } from "./RetentionFields";
import { RetentionPreview } from "./RetentionPreview";
import { ScheduleFields } from "./ScheduleFields";
import { SelectorFields } from "./SelectorFields";

const PREVIEW_DELAY_MS = 300;

interface PolicyFormViewProps {
  /** Set when an existing policy is edited. */
  id?: string;
  initial: PolicyForm;
  /**
   * Used inside the setup: a storage location must be chosen, the retention is left out (everything is kept until
   * the person decides otherwise) and saving calls onSaved instead of going to the list.
   */
  embedded?: boolean;
  onSaved?: () => void;
}

/** Create or edit a policy. Problems are shown after the first try to save, not while the person is still typing. */
export function PolicyFormView({ id, initial, embedded = false, onSaved }: PolicyFormViewProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [form, setForm] = useState(initial);
  const [tried, setTried] = useState(false);
  const [attempts, setAttempts] = useState(0);
  const summaryRef = useRef<HTMLDivElement>(null);
  const { state, save } = useSavePolicy(id);
  // The backups that saving would delete, shown once to be confirmed.
  const [pendingPlan, setPendingPlan] = useState<RetentionPlan | null>(null);
  const [checking, setChecking] = useState(false);
  const [checkFailed, setCheckFailed] = useState(false);

  const problems = useMemo(() => findProblems(form, { requireExport: embedded }), [form, embedded]);
  const visible = tried ? problems : [];
  const saving = state.status === "saving" || checking;
  const change = (patch: Partial<PolicyForm>) => setForm((current) => ({ ...current, ...patch }));

  // The preview needs a complete selection and schedule, but neither the name nor the retention.
  const previewReady = problems.every((p) => p === "name" || p === "retention");
  const previewKey = useDebouncedValue(previewReady ? JSON.stringify(toInput(form)) : "", PREVIEW_DELAY_MS);
  const previewInput = useMemo(() => (previewKey ? JSON.parse(previewKey) : null), [previewKey]);

  // The preview of what retention deletes needs valid rules, a valid schedule (for its time zone) and nothing else.
  const retentionKey = useDebouncedValue(
    problems.every((p) => p === "name" || p === "applications" || p === "labels") && !form.keepAll ? JSON.stringify(toInput(form)) : "",
    PREVIEW_DELAY_MS,
  );
  const retentionPreviewInput = useMemo(() => (retentionKey ? JSON.parse(retentionKey) : null), [retentionKey]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setTried(true);
    setAttempts((n) => n + 1);
    if (problems.length > 0) {
      return;
    }
    const input = toInput(form);
    // Changed rules can delete backups that exist already. Look at the current state right before saving, so that
    // the question names what would really go.
    if (id && !form.keepAll) {
      setChecking(true);
      setCheckFailed(false);
      try {
        const plan = await previewRetention(id, input);
        if (plan.delete.length > 0) {
          setPendingPlan(plan);
          return;
        }
      } catch {
        setCheckFailed(true);
        return;
      } finally {
        setChecking(false);
      }
    }
    await saveAndLeave();
  };

  const saveAndLeave = async () => {
    if (await save(toInput(form))) {
      if (onSaved) {
        onSaved();
      } else {
        navigate("/policies");
      }
    }
  };

  const confirmDeleting = async () => {
    await saveAndLeave();
    setPendingPlan(null);
  };

  const serverError = state.status === "error" ? state.error : null;

  // The save button sits at the bottom, the problems at the top: bring them into view and have them read out.
  useEffect(() => {
    if (attempts > 0 && (problems.length > 0 || serverError)) {
      summaryRef.current?.focus();
    }
    // Only a new attempt moves the focus, not every keystroke afterwards, so `problems` is left out on purpose.
  }, [attempts, serverError]);

  return (
    <form className="form" onSubmit={(e) => void submit(e)} noValidate>
      {tried && problems.length > 0 ? (
        <div className="notice notice--danger" role="alert" tabIndex={-1} ref={summaryRef}>
          <TriangleAlert size={18} aria-hidden="true" />
          <p>{t("policies.form.problems.summary")}</p>
        </div>
      ) : null}
      {checkFailed ? (
        <div className="notice notice--danger" role="alert" tabIndex={-1}>
          <TriangleAlert size={18} aria-hidden="true" />
          <p>{t("policies.retention.checkFailed")}</p>
        </div>
      ) : null}
      {serverError ? (
        <div className="notice notice--danger" role="alert" tabIndex={-1} ref={problems.length === 0 ? summaryRef : undefined}>
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`policies.form.saveErrors.${serverError.code}`, { defaultValue: t("policies.form.saveErrors.unknown") })}</p>
            <p className="muted">{serverError.hint ?? serverError.message}</p>
          </div>
        </div>
      ) : null}

      <Card title={t("policies.form.general")}>
        <Field label={t("policies.form.name.label")} error={visible.includes("name") ? t("policies.form.problems.name") : undefined}>
          {({ id: fieldId, describedBy, invalid }) => (
            <input
              id={fieldId}
              className="field__control"
              value={form.name}
              maxLength={64}
              autoComplete="off"
              placeholder={t("policies.form.name.placeholder")}
              aria-describedby={describedBy}
              aria-invalid={invalid}
              onChange={(e) => change({ name: e.target.value })}
            />
          )}
        </Field>
      </Card>

      <Card title={t("policies.form.what")}>
        <SelectorFields form={form} problems={visible} onChange={change} />
      </Card>

      <Card title={t("policies.form.when")}>
        <ScheduleFields form={form} problems={visible} onChange={change} />
      </Card>

      <Card title={t("policies.form.exportTitle")}>
        <ExportFields form={form} onChange={change} />
        {visible.includes("export") ? <p className="field__error">{t("policies.form.problems.export")}</p> : null}
      </Card>

      {embedded ? null : (
        <Card title={t("policies.form.keep")}>
          <RetentionFields form={form} problems={visible} onChange={change} />
          {id && !form.keepAll && retentionPreviewInput ? <RetentionPreview policyId={id} input={retentionPreviewInput} /> : null}
          {id ? null : <p className="muted">{t("policies.retention.newHint")}</p>}
        </Card>
      )}

      {previewInput ? (
        <PolicyPreview input={previewInput} />
      ) : (
        <Card title={t("policies.preview.title")}>
          <p className="muted">{t("policies.preview.incomplete")}</p>
        </Card>
      )}

      <div className="form__actions">
        {embedded ? null : (
          <Link className="button button--secondary" to="/policies">
            {t("policies.form.cancel")}
          </Link>
        )}
        <button type="submit" className="button button--primary" disabled={saving}>
          {saving ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
          {saving ? t("policies.form.saving") : id ? t("policies.form.save") : t("policies.form.create")}
        </button>
      </div>
      {pendingPlan ? (
        <ConfirmRetentionDialog plan={pendingPlan} saving={state.status === "saving"} onConfirm={() => void confirmDeleting()} onClose={() => setPendingPlan(null)} />
      ) : null}
    </form>
  );
}
