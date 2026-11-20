import { useCallback, useState } from "react";
import { ApiError } from "../../lib/api";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { isActive } from "../../lib/jobs";
import type { PagedList } from "../../lib/paged";
import { createSnapshot, deleteSnapshot, fetchSnapshots, type Snapshot } from "./api";

/** While a backup runs the list refreshes quickly, otherwise it only catches up with other users. */
const ACTIVE_REFRESH_MS = 2_000;
const IDLE_REFRESH_MS = 10_000;

export function useSnapshots(
  application: string,
  pages: number,
): { state: ResourceState<PagedList<Snapshot>>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchSnapshots(application, pages, signal), [application, pages]);
  const refreshMs = useCallback(
    (data: PagedList<Snapshot> | undefined) => (data?.items.some(isActive) ? ACTIVE_REFRESH_MS : IDLE_REFRESH_MS),
    [],
  );
  return useResource(load, `${application}|${pages}`, { refreshMs, keepPrevious: true });
}

export type BackupState = { status: "idle" } | { status: "starting" } | { status: "error"; error: ApiError };

/** Starts a backup and reports whether the request is in flight or failed. */
export function useBackup(application: string, onStarted: () => void): { state: BackupState; start: () => void } {
  const [state, setState] = useState<BackupState>({ status: "idle" });

  const start = useCallback(() => {
    setState({ status: "starting" });
    createSnapshot(application)
      .then(() => {
        setState({ status: "idle" });
        onStarted();
      })
      .catch((error: unknown) => {
        setState({
          status: "error",
          error: error instanceof ApiError ? error : new ApiError("unknown_error", String(error)),
        });
        // A conflict means a backup runs already, so the list must show it.
        onStarted();
      });
  }, [application, onStarted]);

  return { state, start };
}

export type DeleteState = { status: "idle" } | { status: "deleting" } | { status: "error"; error: ApiError };

/** Deletes one backup and reports whether the request is in flight or failed. Resolves to true when it is gone. */
export function useDeleteSnapshot(onDone: () => void): {
  state: DeleteState;
  remove: (id: string) => Promise<boolean>;
  reset: () => void;
} {
  const [state, setState] = useState<DeleteState>({ status: "idle" });

  const remove = useCallback(
    async (id: string) => {
      setState({ status: "deleting" });
      try {
        await deleteSnapshot(id);
        setState({ status: "idle" });
        onDone();
        return true;
      } catch (error) {
        setState({
          status: "error",
          error: error instanceof ApiError ? error : new ApiError("unknown_error", String(error)),
        });
        // A conflict means the list is out of date.
        onDone();
        return false;
      }
    },
    [onDone],
  );
  const reset = useCallback(() => setState({ status: "idle" }), []);

  return { state, remove, reset };
}
