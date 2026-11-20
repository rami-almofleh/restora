import { useCallback, useState } from "react";
import { ApiError } from "../../lib/api";
import type { PagedList } from "../../lib/paged";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { fetchApplications } from "../applications/api";
import {
  createPolicy,
  deletePolicy,
  fetchPolicies,
  fetchPolicy,
  previewPolicy,
  previewRetention,
  runPolicy,
  setPolicyPaused,
  updatePolicy,
  type Policy,
  type PolicyInput,
  type Preview,
  type RetentionPlan,
  type RunResult,
} from "./api";

/** Policies change rarely and only by people, so a slow refresh is enough to catch other users and the controller. */
const REFRESH_MS = 10_000;

function toApiError(error: unknown): ApiError {
  return error instanceof ApiError ? error : new ApiError("unknown_error", String(error));
}

export function usePolicies(pages: number): { state: ResourceState<PagedList<Policy>>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchPolicies(pages, signal), [pages]);
  return useResource(load, String(pages), { refreshMs: REFRESH_MS, keepPrevious: true });
}

export function usePolicy(id: string): { state: ResourceState<Policy>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchPolicy(id, signal), [id]);
  return useResource(load, id);
}

/** Shows what an input would do. The server checks the schedule, so an invalid one comes back as an error. */
export function usePreview(input: PolicyInput): { state: ResourceState<Preview> } {
  const load = useCallback((signal: AbortSignal) => previewPolicy(input, signal), [input]);
  return { state: useResource(load, JSON.stringify(input), { keepPrevious: true }).state };
}

/** Shows which backups of an existing policy retention would delete. Not available for a policy that is not saved yet. */
export function useRetentionPlan(id: string, input: PolicyInput): { state: ResourceState<RetentionPlan> } {
  const load = useCallback((signal: AbortSignal) => previewRetention(id, input, signal), [id, input]);
  return { state: useResource(load, `${id}|${JSON.stringify(input)}`, { keepPrevious: true }).state };
}

const APPLICATION_PAGES = 4;

/** The names of the applications a policy can pick, up to 200. */
export function useApplicationNames(): { state: ResourceState<string[]>; reload: () => void } {
  const load = useCallback(
    async (signal: AbortSignal) =>
      (await fetchApplications({ query: "", volumes: "all" }, APPLICATION_PAGES, signal)).items.map((a) => a.name),
    [],
  );
  return useResource(load, "names");
}

export type ActionState =
  | { status: "idle" }
  | { status: "working"; id: string }
  | { status: "error"; error: ApiError }
  /** A policy was started now: which backups began and which applications were left out. */
  | { status: "started"; id: string; result: RunResult };

/** Pause, resume and delete from the list. Only one action runs at a time. */
export function usePolicyActions(onDone: () => void): {
  state: ActionState;
  setPaused: (id: string, paused: boolean) => Promise<void>;
  runNow: (id: string) => Promise<void>;
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
    setPaused: async (id, paused) => {
      await run(id, () => setPolicyPaused(id, paused));
    },
    remove: (id) => run(id, () => deletePolicy(id)),
    runNow: async (id) => {
      setState({ status: "working", id });
      try {
        const result = await runPolicy(id);
        setState({ status: "started", id, result });
      } catch (error) {
        setState({ status: "error", error: toApiError(error) });
      }
      onDone();
    },
    dismiss: useCallback(() => setState({ status: "idle" }), []),
  };
}

export type SaveState = { status: "idle" } | { status: "saving" } | { status: "error"; error: ApiError };

/** Creates a policy, or changes the one with the given id. Resolves to true when it was saved. */
export function useSavePolicy(id?: string): { state: SaveState; save: (input: PolicyInput) => Promise<boolean> } {
  const [state, setState] = useState<SaveState>({ status: "idle" });
  const save = useCallback(
    async (input: PolicyInput) => {
      setState({ status: "saving" });
      try {
        await (id ? updatePolicy(id, input) : createPolicy(input));
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
