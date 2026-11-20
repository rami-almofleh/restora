import { apiGet } from "../../lib/api";
import { fetchPaged, type PagedList } from "../../lib/paged";

/** A backup that is stored in a storage location. It outlives the local backup it was copied from. */
export interface StoredBackup {
  id: string;
  application: string;
  snapshot: string;
  location: { id: string; name: string };
  /** When the backup was taken. */
  createdAt: string;
  policy?: { id: string; name: string };
  volumes: { claim: string; sizeBytes: number }[];
  resources: number;
  /** The size of the volumes. */
  sizeBytes: number;
  /** How much the repository grew by this copy. */
  addedBytes: number;
}

export interface ExportedApplication {
  application: string;
  exports: number;
  latestAt: string;
}

export const EXPORT_PAGE_SIZE = 10;

/** Loads the newest `pages` pages of the stored backups of an application. */
export function fetchStoredBackups(application: string, pages: number, signal?: AbortSignal): Promise<PagedList<StoredBackup>> {
  return fetchPaged<StoredBackup>("/api/v1/exports", { application }, EXPORT_PAGE_SIZE, pages, signal);
}

/** The applications that have stored backups, also those that no longer exist in the cluster. */
export async function fetchExportedApplications(signal?: AbortSignal): Promise<ExportedApplication[]> {
  return (await apiGet<{ items: ExportedApplication[] }>("/api/v1/export-applications", signal)).items;
}
