import { useCallback, useState } from "react";
import { ApiError } from "../../lib/api";
import { isActive } from "../../lib/jobs";
import type { PagedList } from "../../lib/paged";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { createRestore, createRestoreFromExport, fetchRestores, type Restore } from "./api";

/** While a restore runs the list refreshes quickly, otherwise it only catches up with other users. */
const ACTIVE_REFRESH_MS = 2_000;
const IDLE_REFRESH_MS = 10_000;

export function useRestores(
  application: string,
  pages: number,
): { state: ResourceState<PagedList<Restore>>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchRestores(application, pages, signal), [application, pages]);
  const refreshMs = useCallback(
    (data: PagedList<Restore> | undefined) => (data?.items.some(isActive) ? ACTIVE_REFRESH_MS : IDLE_REFRESH_MS),
    [],
  );
  return useResource(load, `${application}|${pages}`, { refreshMs, keepPrevious: true });
}

export type RestoreRequestState = { status: "idle" } | { status: "starting" } | { status: "error"; error: ApiError };

/** Starts a restore of one snapshot and reports whether the request is in flight or failed. */
export function useStartRestore(onStarted: () => void): {
  state: RestoreRequestState;
  start: (snapshotId: string) => Promise<boolean>;
  /** Restores from a backup in a storage location. */
  startFromExport: (exportId: string) => Promise<boolean>;
  reset: () => void;
} {
  const [state, setState] = useState<RestoreRequestState>({ status: "idle" });

  const begin = useCallback(
    async (action: () => Promise<unknown>) => {
      setState({ status: "starting" });
      try {
        await action();
        setState({ status: "idle" });
        onStarted();
        return true;
      } catch (error) {
        setState({
          status: "error",
          error: error instanceof ApiError ? error : new ApiError("unknown_error", String(error)),
        });
        // A conflict means something runs already, so the lists must show it.
        onStarted();
        return false;
      }
    },
    [onStarted],
  );
  const start = useCallback((snapshotId: string) => begin(() => createRestore(snapshotId)), [begin]);
  const startFromExport = useCallback((exportId: string) => begin(() => createRestoreFromExport(exportId)), [begin]);
  const reset = useCallback(() => setState({ status: "idle" }), []);

  return { state, start, startFromExport, reset };
}
