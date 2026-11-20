import { apiGet } from "../../lib/api";
import { fetchPaged, type PagedList } from "../../lib/paged";

export type Health = "running" | "degraded" | "stopped";
export type VolumesFilter = "all" | "with" | "without";

export interface ApplicationSummary {
  name: string;
  health: Health;
  engines: string[];
  workloads: number;
  volumes: number;
  configMaps: number;
  secrets: number;
  createdAt: string;
}

export interface Workload {
  kind: string;
  name: string;
  ready: number;
  desired: number;
  images: string[];
  engine?: string;
}

export interface Volume {
  name: string;
  storageClass?: string;
  capacityBytes: number;
  phase: string;
}

export interface ApplicationDetail extends ApplicationSummary {
  workloadItems: Workload[];
  volumeItems: Volume[];
  configMapNames: string[];
  secretNames: string[];
}

export const PAGE_SIZE = 50;

export function fetchApplications(
  filter: { query: string; volumes: VolumesFilter },
  pages: number,
  signal?: AbortSignal,
): Promise<PagedList<ApplicationSummary>> {
  const query: Record<string, string> = {};
  if (filter.query) query.q = filter.query;
  if (filter.volumes !== "all") query.volumes = filter.volumes;
  return fetchPaged<ApplicationSummary>("/api/v1/applications", query, PAGE_SIZE, pages, signal);
}

export function fetchApplication(name: string, signal?: AbortSignal): Promise<ApplicationDetail> {
  return apiGet<ApplicationDetail>(`/api/v1/applications/${encodeURIComponent(name)}`, signal);
}
