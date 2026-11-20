import { Circle, CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { JobStep } from "../lib/jobs";

const ICONS = { Pending: Circle, Running: LoaderCircle, Succeeded: CircleCheck, Failed: CircleAlert } as const;

interface StepListProps {
  steps: JobStep[];
  /** Translation prefix of the step names, for example "snapshots.steps". */
  labelPrefix: string;
}

/** The steps of one backup or restore. Each step shows an icon and its name, never color alone. */
export function StepList({ steps, labelPrefix }: StepListProps) {
  const { t } = useTranslation();
  return (
    <ol className="step-list">
      {steps.map((step) => {
        const Icon = ICONS[step.phase];
        return (
          <li key={step.id} className={`step-list__item step-list__item--${step.phase.toLowerCase()}`}>
            <Icon size={16} aria-hidden="true" className={step.phase === "Running" ? "spin" : undefined} />
            <span>{t(`${labelPrefix}.${step.id}`, { defaultValue: step.id })}</span>
            <span className="sr-only">{t(`common.phase.${step.phase}`)}</span>
          </li>
        );
      })}
    </ol>
  );
}
