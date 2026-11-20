import type { JobPhase, JobStep } from "../lib/jobs";
import { isActive } from "../lib/jobs";
import { FailureDetails } from "./FailureDetails";
import { StepList } from "./StepList";

interface JobDetailsProps {
  phase: JobPhase;
  steps: JobStep[];
  /** "snapshots" or "restores": the translation prefix of the steps (.steps) and the error texts (.errors). */
  textPrefix: "snapshots" | "restores";
  errorCode?: string;
  message?: string;
  hint?: string;
}

/** What a backup or restore shows below its header while it runs or after it failed. */
export function JobDetails({ phase, steps, textPrefix, errorCode, message, hint }: JobDetailsProps) {
  if (isActive({ phase })) {
    return <StepList steps={steps} labelPrefix={`${textPrefix}.steps`} />;
  }
  if (phase === "Failed") {
    return (
      <FailureDetails
        steps={steps}
        stepPrefix={`${textPrefix}.steps`}
        errorPrefix={`${textPrefix}.errors`}
        code={errorCode}
        message={message}
        hint={hint}
      />
    );
  }
  return null;
}
