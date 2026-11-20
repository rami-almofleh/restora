import { useTranslation } from "react-i18next";
import type { JobStep } from "../lib/jobs";
import { StepList } from "./StepList";

interface FailureDetailsProps {
  steps: JobStep[];
  stepPrefix: string;
  /** Translation prefix of the error texts, for example "snapshots.errors". Each code has .text and .hint. */
  errorPrefix: string;
  code?: string;
  message?: string;
  hint?: string;
}

/** What happened, why, what to do. The server's English text is the fallback and the technical detail. */
export function FailureDetails({ steps, stepPrefix, errorPrefix, code = "unknown", message, hint }: FailureDetailsProps) {
  const { t } = useTranslation();
  return (
    <div className="failure">
      <StepList steps={steps} labelPrefix={stepPrefix} />
      <p>{t(`${errorPrefix}.${code}.text`, { defaultValue: t(`${errorPrefix}.unknown.text`) })}</p>
      <p className="muted">{t(`${errorPrefix}.${code}.hint`, { defaultValue: t(`${errorPrefix}.unknown.hint`) })}</p>
      <details className="details">
        <summary>{t("common.technicalDetails")}</summary>
        <code>
          {code}: {message}
          {hint ? ` ${hint}` : ""}
        </code>
      </details>
    </div>
  );
}
