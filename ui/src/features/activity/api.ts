import type { JobPhase, JobStep } from "../../lib/jobs";
import { fetchPaged, type PagedList } from "../../lib/paged";
import type { ExportCopy } from "../../components/ExportList";

export type ActivityType = "backup" | "restore";
export type TypeFilter = "all" | ActivityType;
export type StatusFilter = "all" | "active" | "succeeded" | "failed";

export interface ActivityEntry {
  type: ActivityType;
  id: string;
  application: string;
  phase: JobPhase;
  message?: string;
  errorCode?: string;
  hint?: string;
  createdAt: string;
  finishedAt?: string;
  steps: JobStep[];
  /** Set for backups that a policy started. */
  policy?: { id: string; name: string };
  /** The copy of a backup to each storage location. */
  exports: ExportCopy[];
  /** Set for backups. */
  backup?: { volumes: number; resources: number; bytes: number };
  /** Set for restores. */
  restore?: { snapshot: string; snapshotCreatedAt?: string; exportLocation?: string; created: number; updated: number; warnings: string[] };
}

/** Entries per status. They ignore the status filter, so the numbers stay put while the user switches. */
export interface ActivityCounts {
  total: number;
  active: number;
  succeeded: number;
  failed: number;
}

export type ActivityList = PagedList<ActivityEntry, { counts: ActivityCounts }>;

export interface ActivityFilter {
  query: string;
  type: TypeFilter;
  status: StatusFilter;
}

export const ACTIVITY_PAGE_SIZE = 20;

export function fetchActivities(filter: ActivityFilter, pages: number, signal?: AbortSignal): Promise<ActivityList> {
  const query: Record<string, string> = {};
  if (filter.query) query.q = filter.query;
  if (filter.type !== "all") query.type = filter.type;
  if (filter.status !== "all") query.status = filter.status;
  return fetchPaged<ActivityEntry, { counts: ActivityCounts }>("/api/v1/activities", query, ACTIVITY_PAGE_SIZE, pages, signal);
}
