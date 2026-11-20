import { RETENTION_KEYS, type Policy, type PolicyInput, type Retention, type RetentionKey, type ScheduleType, type SelectorMode } from "./api";

/** The form works with strings the way inputs do. toInput turns it into what the API takes. */
export interface PolicyForm {
  name: string;
  mode: SelectorMode;
  applications: string[];
  /** Labels as typed: "team=a, tier=prod". */
  labels: string;
  scheduleType: ScheduleType;
  minute: string;
  /** HH:MM, as the time input gives it. */
  time: string;
  weekday: string;
  cron: string;
  timezone: string;
  /** Keep every backup. Off means the numbers below decide. */
  keepAll: boolean;
  /** The retention numbers as typed. */
  keep: Record<RetentionKey, string>;
  /** Ids of the storage locations backups are copied to. */
  exportTo: string[];
}

export type FormProblem = "name" | "applications" | "labels" | "minute" | "cron" | "retention" | "export";

const NO_KEEP: Record<RetentionKey, string> = { last: "", hourly: "", daily: "", weekly: "", monthly: "", yearly: "" };

export function emptyForm(timezone: string): PolicyForm {
  return {
    name: "",
    mode: "applications",
    applications: [],
    labels: "",
    scheduleType: "daily",
    minute: "0",
    time: "02:00",
    weekday: "0",
    cron: "",
    timezone,
    keepAll: true,
    keep: { ...NO_KEEP },
    exportTo: [],
  };
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

export function formFromPolicy(p: Policy): PolicyForm {
  const { selector, schedule } = p;
  return {
    name: p.name,
    mode: selector.mode,
    applications: selector.applications ?? [],
    labels: Object.entries(selector.matchLabels ?? {})
      .map(([k, v]) => `${k}=${v}`)
      .join(", "),
    scheduleType: schedule.type,
    minute: String(schedule.minute),
    time: `${pad(schedule.hour)}:${pad(schedule.minute)}`,
    weekday: String(schedule.weekday),
    cron: schedule.cron ?? "",
    timezone: schedule.timezone || "UTC",
    keepAll: !hasRules(p.retention),
    exportTo: p.exportTo,
    keep: Object.fromEntries(RETENTION_KEYS.map((k) => [k, p.retention[k] > 0 ? String(p.retention[k]) : ""])) as Record<RetentionKey, string>,
  };
}

function hasRules(retention: Retention): boolean {
  return RETENTION_KEYS.some((k) => retention[k] > 0);
}

/** Empty means 0. Returns undefined for anything that is not a whole number from 0 to 1000. */
function parseKeep(text: string): number | undefined {
  const trimmed = text.trim();
  if (trimmed === "") return 0;
  return /^\d{1,4}$/.test(trimmed) && Number(trimmed) <= 1000 ? Number(trimmed) : undefined;
}

/** The retention of a form. Keeping everything is all zero. */
export function retentionOf(form: PolicyForm): Retention {
  const value = (k: RetentionKey) => (form.keepAll ? 0 : (parseKeep(form.keep[k]) ?? 0));
  return { last: value("last"), hourly: value("hourly"), daily: value("daily"), weekly: value("weekly"), monthly: value("monthly"), yearly: value("yearly") };
}

/** Parses "key=value, key2=value2". Returns undefined when a part has no key or no equals sign. */
export function parseLabels(text: string): Record<string, string> | undefined {
  const labels: Record<string, string> = {};
  for (const part of text.split(",")) {
    const trimmed = part.trim();
    if (trimmed === "") continue;
    const at = trimmed.indexOf("=");
    const key = at > 0 ? trimmed.slice(0, at).trim() : "";
    if (key === "") return undefined;
    labels[key] = trimmed.slice(at + 1).trim();
  }
  return Object.keys(labels).length > 0 ? labels : undefined;
}

function parseTime(time: string): { hour: number; minute: number } | undefined {
  const match = /^(\d{1,2}):(\d{2})$/.exec(time);
  if (!match) return undefined;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  return hour <= 23 && minute <= 59 ? { hour, minute } : undefined;
}

function parseMinute(text: string): number | undefined {
  return /^\d{1,2}$/.test(text.trim()) && Number(text) <= 59 ? Number(text) : undefined;
}

/**
 * Problems that can be found without asking the server. The cron expression itself is checked by the server.
 * With `requireExport` a storage location must be chosen, as the setup needs it.
 */
export function findProblems(form: PolicyForm, options: { requireExport?: boolean } = {}): FormProblem[] {
  const problems: FormProblem[] = [];
  if (form.name.trim() === "") problems.push("name");
  if (form.mode === "applications" && form.applications.length === 0) problems.push("applications");
  if (form.mode === "labels" && parseLabels(form.labels) === undefined) problems.push("labels");
  if (form.scheduleType === "hourly" && parseMinute(form.minute) === undefined) problems.push("minute");
  if (form.scheduleType === "custom" && form.cron.trim().split(/\s+/).length !== 5) problems.push("cron");
  if (options.requireExport && form.exportTo.length === 0) problems.push("export");
  if (!form.keepAll) {
    const numbers = RETENTION_KEYS.map((k) => parseKeep(form.keep[k]));
    if (numbers.includes(undefined) || !numbers.some((n) => n !== undefined && n > 0)) problems.push("retention");
  }
  return problems;
}

/** Builds the API input. Call it only when findProblems is empty. */
export function toInput(form: PolicyForm): PolicyInput {
  const time = parseTime(form.time) ?? { hour: 0, minute: 0 };
  const minute = form.scheduleType === "hourly" ? (parseMinute(form.minute) ?? 0) : time.minute;
  return {
    name: form.name.trim(),
    selector:
      form.mode === "applications"
        ? { mode: "applications", applications: form.applications }
        : form.mode === "labels"
          ? { mode: "labels", matchLabels: parseLabels(form.labels) ?? {} }
          : { mode: "all" },
    schedule: {
      type: form.scheduleType,
      minute,
      hour: form.scheduleType === "hourly" ? 0 : time.hour,
      weekday: form.scheduleType === "weekly" ? Number(form.weekday) : 0,
      ...(form.scheduleType === "custom" ? { cron: form.cron.trim() } : {}),
      timezone: form.timezone,
    },
    retention: retentionOf(form),
    exportTo: form.exportTo,
  };
}
