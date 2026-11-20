import { useCallback } from "react";
import { useResource, type ResourceState } from "../../lib/use-resource";
import { fetchSystemInfo, type SystemInfo } from "./api";

export function useSystemInfo(): { state: ResourceState<SystemInfo>; reload: () => void } {
  const load = useCallback((signal: AbortSignal) => fetchSystemInfo(signal), []);
  return useResource(load, "system-info");
}
