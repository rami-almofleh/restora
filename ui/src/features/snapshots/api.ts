import { apiDelete, apiPost } from "../../lib/api";
import type { JobPhase, JobStep } from "../../lib/jobs";
import { fetchPaged, type PagedList } from "../../lib/paged";
import type { ExportCopy } from "../../components/ExportList";

export interface SnapshotVolume {
  claim: string;
  volumeSnapshot: string;
  className: string;
  ready: boolean;
  sizeBytes?: number;
  error?: string;
}

export interface Snapshot {
  id: string;
  application: string;
  phase: JobPhase;
  message?: string;
  errorCode?: string;
  hint?: string;
  createdAt: string;
  finishedAt?: string;
  steps: JobStep[];
  volumes: SnapshotVolume[];
  resources?: { count: number; bytes: number };
  /** Set when a policy started the backup. */
  policy?: { id: string; name: string };
  /** The copy to each storage location. Empty for backups that stay in the cluster. */
  exports: ExportCopy[];
}

export const SNAPSHOT_PAGE_SIZE = 10;

function snapshotsPath(application: string): string {
  return `/api/v1/applications/${encodeURIComponent(application)}/snapshots`;
}

/** Loads the newest `pages` pages of snapshots. */
export function fetchSnapshots(application: string, pages: number, signal?: AbortSignal): Promise<PagedList<Snapshot>> {
  return fetchPaged<Snapshot>(snapshotsPath(application), {}, SNAPSHOT_PAGE_SIZE, pages, signal);
}

/** Asks Restora to back up the application now. The backup runs in the background. */
export function createSnapshot(application: string): Promise<Snapshot> {
  return apiPost<Snapshot>(snapshotsPath(application));
}

/** Asks Restora to copy a backup to a storage location again after the copy failed. It runs in the background. */
export function retryExport(id: string, location: string): Promise<Snapshot> {
  return apiPost<Snapshot>(`/api/v1/snapshots/${encodeURIComponent(id)}/exports/${encodeURIComponent(location)}/retry`);
}

/** Deletes a backup with its volume snapshots and saved files. This cannot be undone. */
export function deleteSnapshot(id: string): Promise<void> {
  return apiDelete(`/api/v1/snapshots/${encodeURIComponent(id)}`);
}
