import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "./api";

export type ResourceState<T> =
  | { status: "loading" }
  | { status: "error"; error: ApiError }
  | { status: "ready"; data: T };

interface Options<T> {
  /**
   * Reload silently after this many milliseconds while the tab is visible. A function can pick the delay from the
   * latest data, for example to poll faster while a job runs. No value or 0 stops reloading.
   */
  refreshMs?: number | ((data: T | undefined) => number | undefined);
  /** Keep showing the previous data while a new key loads (for search and paging). */
  keepPrevious?: boolean;
}

function toApiError(error: unknown): ApiError {
  return error instanceof ApiError ? error : new ApiError("unknown_error", String(error));
}

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/**
 * Loads one resource and exposes the four page states (loading, error, ready).
 * `key` identifies what is loaded: when it changes, the resource is loaded again.
 */
export function useResource<T>(
  load: (signal: AbortSignal) => Promise<T>,
  key: string,
  { refreshMs, keepPrevious = false }: Options<T> = {},
): { state: ResourceState<T>; reload: () => void } {
  const [state, setState] = useState<ResourceState<T>>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  const loadRef = useRef(load);
  const refreshRef = useRef(refreshMs);
  const dataRef = useRef<T | undefined>(undefined);

  useEffect(() => {
    loadRef.current = load;
    refreshRef.current = refreshMs;
  }, [load, refreshMs]);

  useEffect(() => {
    let controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;

    const schedule = () => {
      const setting = refreshRef.current;
      const delay = typeof setting === "function" ? setting(dataRef.current) : setting;
      if (stopped || !delay) {
        return;
      }
      timer = setTimeout(() => {
        if (document.hidden) {
          schedule();
        } else {
          run();
        }
      }, delay);
    };

    const run = () => {
      controller.abort();
      controller = new AbortController();
      const current = controller;
      loadRef
        .current(current.signal)
        .then((data) => {
          dataRef.current = data;
          setState({ status: "ready", data });
        })
        .catch((error: unknown) => {
          if (!isAbort(error)) {
            setState({ status: "error", error: toApiError(error) });
          }
        })
        .finally(() => {
          if (!current.signal.aborted) {
            schedule();
          }
        });
    };

    dataRef.current = undefined;
    setState((previous) => (keepPrevious && previous.status === "ready" ? previous : { status: "loading" }));
    run();

    return () => {
      stopped = true;
      controller.abort();
      clearTimeout(timer);
    };
  }, [key, attempt, keepPrevious]);

  const reload = useCallback(() => setAttempt((n) => n + 1), []);
  return { state, reload };
}
