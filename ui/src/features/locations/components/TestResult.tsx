import { useTranslation } from "react-i18next";
import { FailureDetails } from "../../../components/FailureDetails";
import { StepList } from "../../../components/StepList";
import { formatDateTime, formatDuration } from "../../../lib/format";
import type { LocationTest } from "../api";

/** The steps of the latest test and, after a failure, what happened and what to do. */
export function TestResult({ test }: { test: LocationTest }) {
  const { t, i18n } = useTranslation();
  if (test.state === "pending") {
    return <p className="muted">{t("locations.test.pendingHint")}</p>;
  }
  const when = test.at ? formatDateTime(test.at, i18n.language) : "";
  // A test that fails at once took no time worth mentioning.
  const took = test.durationMs !== undefined && test.durationMs >= 1000 ? ` · ${t("common.duration", { duration: formatDuration(test.durationMs, i18n.language) })}` : "";

  return (
    <div className="location-test">
      <p className="muted">{`${t("locations.test.at", { time: when })}${took}`}</p>
      {test.state === "failed" ? (
        <FailureDetails
          steps={test.checks}
          stepPrefix="locations.checks"
          errorPrefix="locations.errors"
          code={test.errorCode}
          message={test.message}
          hint={test.hint}
        />
      ) : (
        <StepList steps={test.checks} labelPrefix="locations.checks" />
      )}
    </div>
  );
}
