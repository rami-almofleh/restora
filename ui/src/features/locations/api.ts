import { apiDelete, apiGet, apiPost, apiPut } from "../../lib/api";
import type { JobStep } from "../../lib/jobs";
import { fetchPaged, type PagedList } from "../../lib/paged";

export type LocationType = "s3" | "nfs";
export type TestState = "pending" | "succeeded" | "failed";

export interface S3Settings {
  endpoint: string;
  region?: string;
  bucket: string;
  prefix?: string;
  pathStyle: boolean;
  skipTLSVerify: boolean;
}

export interface NfsSettings {
  server: string;
  path: string;
}

/** The latest connection test. The checks are the steps in order; a step after a failed one was not tried. */
export interface LocationTest {
  state: TestState;
  at?: string;
  durationMs?: number;
  checks: JobStep[];
  errorCode?: string;
  message?: string;
  hint?: string;
}

export interface Location {
  id: string;
  name: string;
  type: LocationType;
  /** Everything but the keys, which the interface never receives. credentialsSet says whether keys are stored. */
  s3?: S3Settings & { credentialsSet: boolean };
  nfs?: NfsSettings;
  test: LocationTest;
  /** How many stored backups the location holds. */
  exports: number;
  createdAt: string;
}

/** What a person enters. The keys are write-only. Leaving both out when changing a location keeps the stored ones. */
export interface LocationInput {
  name: string;
  type: LocationType;
  s3?: S3Settings & { accessKey?: string; secretKey?: string };
  nfs?: NfsSettings;
}

export const LOCATION_PAGE_SIZE = 20;

const BASE = "/api/v1/locations";

function locationPath(id: string): string {
  return `${BASE}/${encodeURIComponent(id)}`;
}

export function fetchLocations(pages: number, signal?: AbortSignal): Promise<PagedList<Location>> {
  return fetchPaged<Location>(BASE, {}, LOCATION_PAGE_SIZE, pages, signal);
}

export function fetchLocation(id: string, signal?: AbortSignal): Promise<Location> {
  return apiGet<Location>(locationPath(id), signal);
}

export function createLocation(input: LocationInput): Promise<Location> {
  return apiPost<Location>(BASE, input);
}

export function updateLocation(id: string, input: LocationInput): Promise<Location> {
  return apiPut<Location>(locationPath(id), input);
}

export function deleteLocation(id: string): Promise<void> {
  return apiDelete(locationPath(id));
}

/** Asks Restora to test the connection again. The answer arrives later: the location shows state "pending" until then. */
export function requestLocationTest(id: string): Promise<Location> {
  return apiPost<Location>(`${locationPath(id)}/test`);
}
