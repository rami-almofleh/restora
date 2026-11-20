import { useCallback } from "react";
import type { PagedList } from "../../lib/paged";
import { useResource, type ResourceState } from "../../lib/use-resource";
import {
  fetchApplication,
  fetchApplications,
  type ApplicationDetail,
  type ApplicationSummary,
  type VolumesFilter,
} from "./api";

/** New and removed namespaces show up on their own after this interval. */
const REFRESH_MS = 10_000;

export function useApplications(
  query: string,
  volumes: VolumesFilter,
  pages: number,
): { state: ResourceState<PagedList<ApplicationSummary>>; reload: () => void } {
  const load = useCallback(
    (signal: AbortSignal) => fetchApplications({ query, volumes }, pages, signal),
    [query, volumes, pages],
  );
  return useResource(load, `${query}|${volumes}|${pages}`, { refreshMs: REFRESH_MS, keepPrevious: true });
}

export function useApplication(name: string): { state: ResourceState<ApplicationDetail>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchApplication(name, signal), [name]);
  return useResource(load, name, { refreshMs: REFRESH_MS });
}
