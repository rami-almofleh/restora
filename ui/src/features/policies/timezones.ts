/** The time zone of this browser, which is what people mean by "at 02:00". */
export function browserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

/** UTC first, then every zone the browser knows. `extra` is added when it is missing (for example a zone the browser lacks). */
export function timezoneOptions(extra: string[] = []): string[] {
  const known = typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : [];
  const rest = new Set([...known, ...extra]);
  rest.delete("UTC");
  return ["UTC", ...[...rest].sort()];
}
