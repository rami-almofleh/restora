import { LoaderCircle, Play, TriangleAlert } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { ApiError } from "../../../lib/api";
import { isActive } from "../../../lib/jobs";
import { useResource } from "../../../lib/use-resource";
import { fetchActivities } from "../../activity/api";
import { ActivityItem } from "../../activity/components/ActivityItem";
import type { Policy } from "../../policies/api";
import { runPolicy } from "../../policies/api";

const POLL_MS = 2_000;
const SHOWN = 3;

interface RunStepProps {
  policy: Policy;
  onChanged: () => void;
}

/** Step 3: a trial run. The policy backs up now and copies to the storage location, and the step shows how that goes. */
export function RunStep({ policy, onChanged }: RunStepProps) {
  const { t } = useTranslation();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const [skipped, setSkipped] = useState<string[]>([]);

  const load = useCallback(async (signal: AbortSignal) => {
    const page = await fetchActivities({ query: "", type: "backup", status: "all" }, 1, signal);
    return page.items.filter((e) => e.policy?.id === policy.id).slice(0, SHOWN);
  }, [policy.id]);
  const { state, reload } = useResource(load, policy.id, { refreshMs: POLL_MS, keepPrevious: true });
  const runs = state.status === "ready" ? state.data : [];
  const running = runs.some(isActive);
  const failedLast = runs[0] && !isActive(runs[0]) && (runs[0].phase === "Failed" || runs[0].exports.some((e) => e.phase === "Failed"));

  const start = async () => {
    setStarting(true);
    setError(null);
    try {
      const result = await runPolicy(policy.id);
      setSkipped(result.skipped.map((s) => s.application));
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError("unknown_error", String(e)));
    } finally {
      setStarting(false);
      reload();
      onChanged();
    }
  };

  const noApplications = policy.applications.length === 0;

  return (
    <>
      <p className="setup-step__intro">{t("setup.run.intro", { applications: policy.applications.join(", ") })}</p>
      {noApplications ? (
        <p className="notice" role="status">
          {t("setup.run.noApplications")}
        </p>
      ) : null}
      {error ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t("setup.run.startFailed")}</p>
            <p className="muted">{error.hint ?? error.message}</p>
          </div>
        </div>
      ) : null}
      {skipped.length > 0 ? <p className="notice">{t("setup.run.skipped", { names: skipped.join(", ") })}</p> : null}

      <div className="setup-step__actions">
        <button type="button" className="button button--primary" onClick={() => void start()} disabled={starting || running || noApplications}>
          {starting || running ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : <Play size={18} aria-hidden="true" />}
          {running ? t("setup.run.running") : failedLast ? t("setup.run.again") : t("setup.run.start")}
        </button>
      </div>

      {runs.length > 0 ? (
        <Card title={t("setup.run.result")}>
          <ul className="job-list">
            {runs.map((entry) => (
              <ActivityItem key={entry.id} entry={entry} />
            ))}
          </ul>
        </Card>
      ) : null}
    </>
  );
}
