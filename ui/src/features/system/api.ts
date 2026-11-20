import { apiGet } from "../../lib/api";

export interface SystemInfo {
  name: string;
  version: string;
  startedAt: string;
}

export function fetchSystemInfo(signal?: AbortSignal): Promise<SystemInfo> {
  return apiGet<SystemInfo>("/api/v1/info", signal);
}
