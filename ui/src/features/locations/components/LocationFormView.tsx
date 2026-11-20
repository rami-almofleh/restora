import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { Card } from "../../../components/Card";
import { Checkbox } from "../../../components/Checkbox";
import { Field } from "../../../components/Field";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import type { LocationType } from "../api";
import { findProblems, toInput, type LocationForm } from "../form-model";
import { useSaveLocation } from "../use-locations";

interface LocationFormViewProps {
  /** Set when an existing location is edited. The type and the stored keys cannot be seen or changed there. */
  id?: string;
  /** Whether keys are stored already. Only used when editing. */
  credentialsSet?: boolean;
  initial: LocationForm;
  /** Used inside the setup: saving calls onSaved instead of going to the list, and cancelling calls onCancel. */
  embedded?: boolean;
  onSaved?: () => void;
  onCancel?: () => void;
}

/** Create or edit a storage location. Problems are shown after the first try to save, not while typing. */
export function LocationFormView({ id, credentialsSet = false, initial, embedded = false, onSaved, onCancel }: LocationFormViewProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [form, setForm] = useState(initial);
  const [tried, setTried] = useState(false);
  const [attempts, setAttempts] = useState(0);
  const summaryRef = useRef<HTMLDivElement>(null);
  const { state, save } = useSaveLocation(id);

  const editing = id !== undefined;
  const problems = useMemo(() => findProblems(form, editing), [form, editing]);
  const visible = tried ? problems : [];
  const saving = state.status === "saving";
  const serverError = state.status === "error" ? state.error : null;
  const change = (patch: Partial<LocationForm>) => setForm((current) => ({ ...current, ...patch }));
  const problem = (p: (typeof problems)[number]) => (visible.includes(p) ? t(`locations.form.problems.${p}`) : undefined);

  const typeOptions: readonly SegmentedOption<LocationType>[] = [
    { value: "s3", label: t("locations.type.s3") },
    { value: "nfs", label: t("locations.type.nfs") },
  ];

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setTried(true);
    setAttempts((n) => n + 1);
    if (problems.length > 0) {
      return;
    }
    if (await save(toInput(form))) {
      if (embedded) {
        onSaved?.();
      } else {
        navigate("/locations");
      }
    }
  };

  // The save button sits at the bottom, the problems at the top: bring them into view and have them read out.
  useEffect(() => {
    if (attempts > 0 && (problems.length > 0 || serverError)) {
      summaryRef.current?.focus();
    }
    // Only a new attempt moves the focus, not every keystroke afterwards, so `problems` is left out on purpose.
  }, [attempts, serverError]);

  return (
    <form className="form" onSubmit={(e) => void submit(e)} noValidate autoComplete="off">
      {tried && problems.length > 0 ? (
        <div className="notice notice--danger" role="alert" tabIndex={-1} ref={summaryRef}>
          <TriangleAlert size={18} aria-hidden="true" />
          <p>{t("locations.form.problems.summary")}</p>
        </div>
      ) : null}
      {serverError ? (
        <div className="notice notice--danger" role="alert" tabIndex={-1} ref={problems.length === 0 ? summaryRef : undefined}>
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`locations.form.saveErrors.${serverError.code}`, { defaultValue: t("locations.form.saveErrors.unknown") })}</p>
            <p className="muted">{serverError.hint ?? serverError.message}</p>
            {serverError.code === "invalid_location" ? <p className="muted">{serverError.message}</p> : null}
          </div>
        </div>
      ) : null}

      <Card title={t("locations.form.general")}>
        <div className="form-section">
          <Field label={t("locations.form.name.label")} error={problem("name")}>
            {({ id: fieldId, describedBy, invalid }) => (
              <input
                id={fieldId}
                className="field__control"
                value={form.name}
                maxLength={64}
                placeholder={t("locations.form.name.placeholder")}
                aria-describedby={describedBy}
                aria-invalid={invalid}
                onChange={(e) => change({ name: e.target.value })}
              />
            )}
          </Field>
          {editing ? null : (
            <SegmentedControl label={t("locations.form.type")} name="location-type" value={form.type} options={typeOptions} onChange={(type) => change({ type })} />
          )}
        </div>
      </Card>

      {form.type === "s3" ? (
        <>
          <Card title={t("locations.form.storage")}>
            <div className="form-section">
              <Field label={t("locations.form.endpoint.label")} hint={t("locations.form.endpoint.hint")} error={problem("endpoint")}>
                {({ id: fieldId, describedBy, invalid }) => (
                  <input id={fieldId} className="field__control" dir="ltr" value={form.endpoint} placeholder="https://s3.example.com" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} aria-invalid={invalid} onChange={(e) => change({ endpoint: e.target.value })} />
                )}
              </Field>
              <Field label={t("locations.form.bucket.label")} hint={t("locations.form.bucket.hint")} error={problem("bucket")}>
                {({ id: fieldId, describedBy, invalid }) => (
                  <input id={fieldId} className="field__control" dir="ltr" value={form.bucket} placeholder="restora" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} aria-invalid={invalid} onChange={(e) => change({ bucket: e.target.value })} />
                )}
              </Field>
              <Field label={t("locations.form.prefix.label")} hint={t("locations.form.prefix.hint")}>
                {({ id: fieldId, describedBy }) => (
                  <input id={fieldId} className="field__control" dir="ltr" value={form.prefix} placeholder="backups/production" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} onChange={(e) => change({ prefix: e.target.value })} />
                )}
              </Field>
              <Field label={t("locations.form.region.label")} hint={t("locations.form.region.hint")}>
                {({ id: fieldId, describedBy }) => (
                  <input id={fieldId} className="field__control field__control--short" dir="ltr" value={form.region} placeholder="eu-central-1" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} onChange={(e) => change({ region: e.target.value })} />
                )}
              </Field>
              <Checkbox label={t("locations.form.pathStyle.label")} hint={t("locations.form.pathStyle.hint")} checked={form.pathStyle} onChange={(pathStyle) => change({ pathStyle })} />
              <Checkbox label={t("locations.form.skipTLS.label")} hint={t("locations.form.skipTLS.hint")} checked={form.skipTLSVerify} onChange={(skipTLSVerify) => change({ skipTLSVerify })}>
                {form.skipTLSVerify ? (
                  <p className="notice notice--warning" role="status">
                    {t("locations.form.skipTLS.warning")}
                  </p>
                ) : null}
              </Checkbox>
            </div>
          </Card>

          <Card title={t("locations.form.credentials")}>
            <div className="form-section">
              <p className="muted">{editing && credentialsSet ? t("locations.form.keys.keepHint") : t("locations.form.keys.hint")}</p>
              <Field label={t("locations.form.accessKey.label")} error={problem("accessKey")}>
                {({ id: fieldId, describedBy, invalid }) => (
                  <input
                    id={fieldId}
                    className="field__control"
                    dir="ltr"
                    value={form.accessKey}
                    placeholder={editing && credentialsSet ? t("locations.form.keys.unchanged") : ""}
                    autoComplete="off"
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    aria-describedby={describedBy}
                    aria-invalid={invalid}
                    onChange={(e) => change({ accessKey: e.target.value })}
                  />
                )}
              </Field>
              <Field label={t("locations.form.secretKey.label")} error={problem("secretKey")}>
                {({ id: fieldId, describedBy, invalid }) => (
                  <input
                    id={fieldId}
                    className="field__control"
                    dir="ltr"
                    type="password"
                    value={form.secretKey}
                    placeholder={editing && credentialsSet ? t("locations.form.keys.unchanged") : ""}
                    autoComplete="new-password"
                    aria-describedby={describedBy}
                    aria-invalid={invalid}
                    onChange={(e) => change({ secretKey: e.target.value })}
                  />
                )}
              </Field>
            </div>
          </Card>
        </>
      ) : (
        <Card title={t("locations.form.share")}>
          <div className="form-section">
            <p className="muted">{t("locations.form.nfs.hint")}</p>
            <Field label={t("locations.form.server.label")} error={problem("server")}>
              {({ id: fieldId, describedBy, invalid }) => (
                <input id={fieldId} className="field__control" dir="ltr" value={form.nfsServer} placeholder="nfs.example.com" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} aria-invalid={invalid} onChange={(e) => change({ nfsServer: e.target.value })} />
              )}
            </Field>
            <Field label={t("locations.form.path.label")} hint={t("locations.form.path.hint")} error={problem("path")}>
              {({ id: fieldId, describedBy, invalid }) => (
                <input id={fieldId} className="field__control" dir="ltr" value={form.nfsPath} placeholder="/exports/restora" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={describedBy} aria-invalid={invalid} onChange={(e) => change({ nfsPath: e.target.value })} />
              )}
            </Field>
          </div>
        </Card>
      )}

      <p className="muted">{t("locations.form.testHint")}</p>

      <div className="form__actions">
        {embedded ? (
          onCancel ? (
            <button type="button" className="button button--secondary" onClick={onCancel}>
              {t("locations.form.cancel")}
            </button>
          ) : null
        ) : (
          <Link className="button button--secondary" to="/locations">
            {t("locations.form.cancel")}
          </Link>
        )}
        <button type="submit" className="button button--primary" disabled={saving}>
          {saving ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
          {saving ? t("locations.form.saving") : editing ? t("locations.form.save") : t("locations.form.create")}
        </button>
      </div>
    </form>
  );
}
