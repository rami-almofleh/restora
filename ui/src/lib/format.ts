/** Formats an ISO 8601 timestamp for display in the given UI language. */
export function formatDateTime(iso: string, language: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

const BYTE_UNITS = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"] as const;

/** Formats a size with binary units, the way Kubernetes counts storage. */
export function formatBytes(bytes: number, language: string): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }
  const exponent = Math.min(Math.floor(Math.log2(bytes) / 10), BYTE_UNITS.length - 1);
  const value = bytes / 1024 ** exponent;
  const formatted = new Intl.NumberFormat(language, { maximumFractionDigits: value < 10 ? 1 : 0 }).format(value);
  return `${formatted} ${BYTE_UNITS[exponent]}`;
}

/** Formats a duration in milliseconds as seconds, minutes or hours, in the given UI language. */
export function formatDuration(ms: number, language: string): string {
  const seconds = Math.max(0, Math.round(ms / 1000));
  const [value, unit] =
    seconds < 60 ? [seconds, "second"] : seconds < 3600 ? [Math.round(seconds / 60), "minute"] : [Math.round(seconds / 3600), "hour"];
  return new Intl.NumberFormat(language, { style: "unit", unit, unitDisplay: "narrow" }).format(value);
}

/** Like formatDateTime, but in a named time zone and with its abbreviation, for times that follow a schedule. */
export function formatDateTimeIn(iso: string, language: string, timeZone: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat(language, {
    // dateStyle and timeStyle cannot be combined with timeZoneName, so the fields are spelled out.
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
    timeZoneName: "short",
  }).format(date);
}
