import type { ActivityEntry } from "../activity/api";
import type { Location } from "../locations/api";
import type { Policy } from "../policies/api";

/** The four steps of the setup, in order. */
export const SETUP_STEPS = ["location", "policy", "run", "restore"] as const;
export type SetupStep = (typeof SETUP_STEPS)[number];

/** What the cluster knows. The setup reads its progress from it, so it can be left and picked up again. */
export interface SetupInput {
  locations: Location[];
  /** Newest first. */
  policies: Policy[];
  /** Backups that succeeded, newest first. */
  backups: ActivityEntry[];
  /** Restores that succeeded, newest first. */
  restores: ActivityEntry[];
  /** The person chose to skip the test restore. */
  restoreSkipped: boolean;
}

export interface SetupProgress {
  done: Record<SetupStep, boolean>;
  /** The first step that is not done, or "done" when all are. */
  current: SetupStep | "done";
  /** Position of the current step, 0-based. Equals the number of steps when everything is done. */
  index: number;
  /** The newest storage location whose test succeeded. */
  location?: Location;
  /** The newest policy: the one the setup created. */
  policy?: Policy;
}

/** A backup counts for the test run when a policy made it and its copy to a storage location worked. */
function isStoredRun(entry: ActivityEntry): boolean {
  return (
    entry.policy !== undefined &&
    entry.phase === "Succeeded" &&
    entry.exports.length > 0 &&
    entry.exports.every((e) => e.phase === "Succeeded")
  );
}

/** A restore counts for the test restore when it came from a storage location. */
function isRestoreFromStorage(entry: ActivityEntry): boolean {
  return entry.phase === "Succeeded" && entry.restore?.exportLocation !== undefined;
}

export function computeProgress(input: SetupInput): SetupProgress {
  const location = input.locations.find((l) => l.test.state === "succeeded");
  const done: Record<SetupStep, boolean> = {
    location: location !== undefined,
    policy: input.policies.length > 0,
    run: input.backups.some(isStoredRun),
    restore: input.restoreSkipped || input.restores.some(isRestoreFromStorage),
  };
  const index = SETUP_STEPS.findIndex((step) => !done[step]);
  return {
    done,
    current: index === -1 ? "done" : SETUP_STEPS[index],
    index: index === -1 ? SETUP_STEPS.length : index,
    location,
    policy: input.policies[0],
  };
}
