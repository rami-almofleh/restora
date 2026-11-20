import { apiDelete, apiGet, apiPost, apiPut } from "../../lib/api";
import { fetchPaged, type PagedList } from "../../lib/paged";

export type SelectorMode = "applications" | "labels" | "all";
export type ScheduleType = "hourly" | "daily" | "weekly" | "custom";

export interface Selector {
  mode: SelectorMode;
  applications?: string[];
  matchLabels?: Record<string, string>;
}

export interface Schedule {
  type: ScheduleType;
  /** 0-59, used by hourly, daily and weekly. */
  minute: number;
  /** 0-23, used by daily and weekly. */
  hour: number;
  /** 0 = Sunday ... 6 = Saturday, used by weekly. */
  weekday: number;
  /** Five-field cron expression, used by custom. */
  cron?: string;
  /** IANA name. The server treats an empty one as UTC. */
  timezone?: string;
}

/** Which backups of a policy are kept. All zero keeps everything. */
export interface Retention {
  /** The newest backups, whenever they were made. */
  last: number;
  /** The newest backup of each of the most recent hours, days, weeks, months and years that have one. */
  hourly: number;
  daily: number;
  weekly: number;
  monthly: number;
  yearly: number;
}

export const RETENTION_KEYS = ["last", "hourly", "daily", "weekly", "monthly", "yearly"] as const;
export type RetentionKey = (typeof RETENTION_KEYS)[number];

/** What a person enters. The server rejects anything else. */
export interface PolicyInput {
  name: string;
  selector: Selector;
  schedule: Schedule;
  retention: Retention;
  /** Storage locations (ids) every backup is copied to. Empty keeps the backups in the cluster. */
  exportTo: string[];
}

export interface SkippedApplication {
  application: string;
  reason: string;
}

export interface Policy extends PolicyInput {
  id: string;
  /** The expression that runs, also for the presets. Empty when the schedule is invalid. */
  cron: string;
  paused: boolean;
  /** What the selection takes right now. */
  applications: string[];
  /** Named applications that do not exist. */
  missing: string[];
  nextRunAt?: string;
  lastRun?: { at: string; snapshots: string[]; skipped: SkippedApplication[] };
  /** Set once retention has deleted backups. */
  lastCleanup?: { at: string; deleted: number };
  errorCode?: string;
  message?: string;
  hint?: string;
  createdAt: string;
}

/** What starting a policy now did: the backups that were started and the applications that were left out. */
export interface RunResult {
  at: string;
  snapshots: string[];
  skipped: SkippedApplication[];
}

export interface Preview {
  cron: string;
  nextRuns: string[];
  applications: string[];
  missing: string[];
}

/** What retention would do with the backups a policy has made so far. */
export interface RetentionPlan {
  keep: { id: string; application: string; createdAt: string; reasons: string[] }[];
  delete: { id: string; application: string; createdAt: string; reason: string }[];
  /** Applications with a backup in progress. Nothing of theirs is planned while it runs. */
  busy: string[];
}

export const POLICY_PAGE_SIZE = 20;

const BASE = "/api/v1/policies";

function policyPath(id: string): string {
  return `${BASE}/${encodeURIComponent(id)}`;
}

export function fetchPolicies(pages: number, signal?: AbortSignal): Promise<PagedList<Policy>> {
  return fetchPaged<Policy>(BASE, {}, POLICY_PAGE_SIZE, pages, signal);
}

export function fetchPolicy(id: string, signal?: AbortSignal): Promise<Policy> {
  return apiGet<Policy>(policyPath(id), signal);
}

export function createPolicy(input: PolicyInput): Promise<Policy> {
  return apiPost<Policy>(BASE, input);
}

export function updatePolicy(id: string, input: PolicyInput): Promise<Policy> {
  return apiPut<Policy>(policyPath(id), input);
}

export function deletePolicy(id: string): Promise<void> {
  return apiDelete(policyPath(id));
}

export function setPolicyPaused(id: string, paused: boolean): Promise<Policy> {
  return apiPost<Policy>(`${policyPath(id)}/${paused ? "pause" : "resume"}`);
}

/** Shows the next runs and the matching applications of an input without saving it. */
export function previewPolicy(input: PolicyInput, signal?: AbortSignal): Promise<Preview> {
  return apiPost<Preview>(`${BASE}/preview`, input, signal);
}

/** Shows which backups of a policy retention would delete if the input were saved. Nothing is deleted. */
export function previewRetention(id: string, input: PolicyInput, signal?: AbortSignal): Promise<RetentionPlan> {
  return apiPost<RetentionPlan>(`${policyPath(id)}/retention-preview`, input, signal);
}

/** Starts the backups of a policy now, outside its schedule. They are copied to the storage locations of the policy. */
export function runPolicy(id: string): Promise<RunResult> {
  return apiPost<RunResult>(`${policyPath(id)}/run`);
}
