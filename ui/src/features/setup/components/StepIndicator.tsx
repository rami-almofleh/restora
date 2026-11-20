import { Circle, CircleCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { SETUP_STEPS, type SetupProgress } from "../progress";

/** The four steps with their state. The current one is named in full, which keeps the row short on a phone. */
export function StepIndicator({ progress }: { progress: SetupProgress }) {
  const { t } = useTranslation();
  const finished = progress.current === "done";

  return (
    <div className="setup-steps">
      <ol className="setup-steps__list" aria-label={t("setup.steps.label")}>
        {SETUP_STEPS.map((step, i) => {
          const done = progress.done[step];
          const current = progress.current === step;
          return (
            <li
              key={step}
              className={`setup-steps__item${done ? " setup-steps__item--done" : ""}${current ? " setup-steps__item--current" : ""}`}
              aria-current={current ? "step" : undefined}
            >
              <span className="setup-steps__mark">
                {done ? <CircleCheck size={20} aria-hidden="true" /> : current ? <span aria-hidden="true">{i + 1}</span> : <Circle size={20} aria-hidden="true" />}
              </span>
              <span className="setup-steps__name">{t(`setup.steps.${step}`)}</span>
              <span className="sr-only">{done ? t("setup.steps.done") : current ? t("setup.steps.current") : t("setup.steps.todo")}</span>
            </li>
          );
        })}
      </ol>
      <p className="setup-steps__summary" aria-live="polite">
        {finished ? t("setup.steps.allDone") : t("setup.steps.position", { n: progress.index + 1, total: SETUP_STEPS.length, name: t(`setup.steps.${progress.current}`) })}
      </p>
    </div>
  );
}
