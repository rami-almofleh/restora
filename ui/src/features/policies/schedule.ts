import type { TFunction } from "i18next";
import { RETENTION_KEYS, type Policy, type Retention, type Schedule } from "./api";

export function formatClock(hour: number, minute: number): string {
  return `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
}

// 22 November 2026 is a Sunday, so day 22 + n is weekday n.
const SUNDAY = Date.UTC(2026, 10, 22);
const DAY_MS = 86_400_000;

/** Name of a weekday (0 = Sunday) in the UI language. */
export function weekdayName(weekday: number, language: string): string {
  return new Intl.DateTimeFormat(language, { weekday: "long", timeZone: "UTC" }).format(new Date(SUNDAY + weekday * DAY_MS));
}

/** One sentence about when a schedule runs, for example "Daily at 02:00 · Europe/Berlin". */
export function describeSchedule(t: TFunction, language: string, schedule: Schedule): string {
  const time = formatClock(schedule.hour, schedule.minute);
  const text =
    schedule.type === "hourly"
      ? t("policies.schedule.hourly", { minute: schedule.minute })
      : schedule.type === "daily"
        ? t("policies.schedule.daily", { time })
        : schedule.type === "weekly"
          ? t("policies.schedule.weekly", { weekday: weekdayName(schedule.weekday, language), time })
          : t("policies.schedule.custom", { cron: schedule.cron ?? "" });
  return t("policies.schedule.inZone", { schedule: text, timezone: schedule.timezone || "UTC" });
}

/** Which applications a policy takes, for example "demo, shop". */
export function describeSelector(t: TFunction, policy: Policy): string {
  const { selector } = policy;
  if (selector.mode === "all") {
    return t("policies.selector.all", { n: policy.applications.length });
  }
  if (selector.mode === "labels") {
    const labels = Object.entries(selector.matchLabels ?? {})
      .map(([k, v]) => `${k}=${v}`)
      .join(", ");
    return t("policies.selector.labels", { labels, n: policy.applications.length });
  }
  return (selector.applications ?? []).join(", ");
}

/** What a retention keeps, for example "the last 2, 7 daily". Without rules: everything stays. */
export function describeRetention(t: TFunction, language: string, retention: Retention): string {
  const parts = RETENTION_KEYS.filter((k) => retention[k] > 0).map((k) => t(`policies.retention.parts.${k}`, { n: retention[k] }));
  if (parts.length === 0) {
    return t("policies.retention.keepAll");
  }
  // The list format uses the separator of the language, for example the Arabic comma.
  return t("policies.retention.keeps", { parts: new Intl.ListFormat(language, { style: "narrow", type: "unit" }).format(parts) });
}
