import { useCallback } from "react";
import type { PagedList } from "../../lib/paged";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { fetchApplications } from "../applications/api";
import { fetchExportedApplications, fetchStoredBackups, type ExportedApplication, type StoredBackup } from "./api";

const REFRESH_MS = 10_000;

export function useStoredBackups(application: string, pages: number): { state: ResourceState<PagedList<StoredBackup>>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchStoredBackups(application, pages, signal), [application, pages]);
  return useResource(load, `${application}|${pages}`, { refreshMs: REFRESH_MS, keepPrevious: true });
}

const APPLICATION_PAGES = 4;

/**
 * The applications that have stored backups but do not exist in the cluster any more. Their namespace was deleted,
 * so they are in no list of applications, yet they can be restored from the storage location.
 */
export function useDeletedApplications(): { state: ResourceState<ExportedApplication[]>; reload: () => void } {
  const load = useCallback(async (signal: AbortSignal) => {
    const [exported, live] = await Promise.all([
      fetchExportedApplications(signal),
      fetchApplications({ query: "", volumes: "all" }, APPLICATION_PAGES, signal),
    ]);
    const names = new Set(live.items.map((a) => a.name));
    return exported.filter((e) => !names.has(e.application));
  }, []);
  return useResource(load, "deleted", { refreshMs: REFRESH_MS });
}
