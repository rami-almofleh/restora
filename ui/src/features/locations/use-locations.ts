import { useCallback, useState } from "react";
import { ApiError } from "../../lib/api";
import type { PagedList } from "../../lib/paged";
import { useResource, type ResourceState } from "../../lib/use-resource";
import {
  createLocation,
  deleteLocation,
  fetchLocation,
  fetchLocations,
  requestLocationTest,
  updateLocation,
  type Location,
  type LocationInput,
} from "./api";

/** While a test runs the list refreshes quickly, otherwise it only catches up with other users. */
const ACTIVE_REFRESH_MS = 2_000;
const IDLE_REFRESH_MS = 10_000;

function toApiError(error: unknown): ApiError {
  return error instanceof ApiError ? error : new ApiError("unknown_error", String(error));
}

export function useLocations(pages: number): { state: ResourceState<PagedList<Location>>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchLocations(pages, signal), [pages]);
  const refreshMs = useCallback(
    (data: PagedList<Location> | undefined) => (data?.items.some((l) => l.test.state === "pending") ? ACTIVE_REFRESH_MS : IDLE_REFRESH_MS),
    [],
  );
  return useResource(load, String(pages), { refreshMs, keepPrevious: true });
}

export function useLocation(id: string): { state: ResourceState<Location>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchLocation(id, signal), [id]);
  return useResource(load, id);
}

export type ActionState = { status: "idle" } | { status: "working"; id: string } | { status: "error"; error: ApiError };

/** Test and delete from the list. Only one action runs at a time. */
export function useLocationActions(onDone: () => void): {
  state: ActionState;
  test: (id: string) => Promise<void>;
  remove: (id: string) => Promise<boolean>;
  dismiss: () => void;
} {
  const [state, setState] = useState<ActionState>({ status: "idle" });

  const run = useCallback(
    async (id: string, action: () => Promise<unknown>): Promise<boolean> => {
      setState({ status: "working", id });
      try {
        await action();
        setState({ status: "idle" });
        onDone();
        return true;
      } catch (error) {
        setState({ status: "error", error: toApiError(error) });
        onDone();
        return false;
      }
    },
    [onDone],
  );

  return {
    state,
    test: async (id) => {
      await run(id, () => requestLocationTest(id));
    },
    remove: (id) => run(id, () => deleteLocation(id)),
    dismiss: useCallback(() => setState({ status: "idle" }), []),
  };
}

export type SaveState = { status: "idle" } | { status: "saving" } | { status: "error"; error: ApiError };

/** Creates a location, or changes the one with the given id. Resolves to true when it was saved. */
export function useSaveLocation(id?: string): { state: SaveState; save: (input: LocationInput) => Promise<boolean> } {
  const [state, setState] = useState<SaveState>({ status: "idle" });
  const save = useCallback(
    async (input: LocationInput) => {
      setState({ status: "saving" });
      try {
        await (id ? updateLocation(id, input) : createLocation(input));
        setState({ status: "idle" });
        return true;
      } catch (error) {
        setState({ status: "error", error: toApiError(error) });
        return false;
      }
    },
    [id],
  );
  return { state, save };
}
