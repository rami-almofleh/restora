import { apiPost } from "../../lib/api";
import type { JobPhase, JobStep } from "../../lib/jobs";
import { fetchPaged, type PagedList } from "../../lib/paged";

export interface Restore {
  id: string;
  application: string;
  /** Empty when the restore comes from a storage location. */
  snapshot?: string;
  /** Set when the restore comes from a storage location. */
  export?: { id: string; locationName?: string; createdAt?: string };
  phase: JobPhase;
  message?: string;
  errorCode?: string;
  hint?: string;
  createdAt: string;
  finishedAt?: string;
  steps: JobStep[];
  resources?: { created: number; updated: number; skipped: number };
  warnings: string[];
}

export const RESTORE_PAGE_SIZE = 5;

/** Loads the newest `pages` pages of restores of an application. */
export function fetchRestores(application: string, pages: number, signal?: AbortSignal): Promise<PagedList<Restore>> {
  return fetchPaged<Restore>(
    `/api/v1/applications/${encodeURIComponent(application)}/restores`,
    {},
    RESTORE_PAGE_SIZE,
    pages,
    signal,
  );
}

/** Asks Restora to restore a snapshot into its application. This replaces the application's volumes. */
export function createRestore(snapshotId: string): Promise<Restore> {
  return apiPost<Restore>(`/api/v1/snapshots/${encodeURIComponent(snapshotId)}/restores`);
}

/**
 * Asks Restora to restore a backup that is stored in a storage location. The application may be gone: its namespace
 * is created again. This replaces the application's volumes.
 */
export function createRestoreFromExport(exportId: string): Promise<Restore> {
  return apiPost<Restore>(`/api/v1/exports/${encodeURIComponent(exportId)}/restores`);
}
