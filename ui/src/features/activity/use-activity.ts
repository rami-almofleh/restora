import { useCallback } from "react";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { fetchActivities, type ActivityFilter, type ActivityList } from "./api";

/** While something runs the list refreshes quickly, otherwise it only catches up with other users. */
const ACTIVE_REFRESH_MS = 2_000;
const IDLE_REFRESH_MS = 10_000;

export function useActivity(filter: ActivityFilter, pages: number): { state: ResourceState<ActivityList>; reload: () => void } {
  const { query, type, status } = filter;
  const load = useCallback(
    (signal: AbortSignal) => fetchActivities({ query, type, status }, pages, signal),
    [query, type, status, pages],
  );
  const refreshMs = useCallback(
    (data: ActivityList | undefined) => (data && data.meta.counts.active > 0 ? ACTIVE_REFRESH_MS : IDLE_REFRESH_MS),
    [],
  );
  return useResource(load, `${query}|${type}|${status}|${pages}`, { refreshMs, keepPrevious: true });
}
