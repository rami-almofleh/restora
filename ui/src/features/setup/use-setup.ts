import { useCallback } from "react";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { fetchActivities, type ActivityEntry } from "../activity/api";
import { fetchLocations, type Location } from "../locations/api";
import { fetchPolicies, type Policy } from "../policies/api";

export interface SetupFacts {
  locations: Location[];
  policies: Policy[];
  backups: ActivityEntry[];
  restores: ActivityEntry[];
}

/** Reads what the setup needs from the cluster. While the setup is open it looks again every few seconds. */
export function useSetupFacts(refreshMs?: number): { state: ResourceState<SetupFacts>; reload: () => void } {
  const load = useCallback(async (signal: AbortSignal): Promise<SetupFacts> => {
    const [locations, policies, backups, restores] = await Promise.all([
      fetchLocations(1, signal),
      fetchPolicies(1, signal),
      fetchActivities({ query: "", type: "backup", status: "succeeded" }, 1, signal),
      fetchActivities({ query: "", type: "restore", status: "succeeded" }, 1, signal),
    ]);
    return { locations: locations.items, policies: policies.items, backups: backups.items, restores: restores.items };
  }, []);
  return useResource(load, "setup", { refreshMs, keepPrevious: true });
}
