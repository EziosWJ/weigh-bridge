const beijingTimeZone = "Asia/Shanghai";

function parseInstant(value: string): Date | null {
  const trimmed = value.trim();
  if (!trimmed) return null;

  // API timestamps are RFC3339. Treat legacy timestamps without an explicit
  // offset as UTC so display does not depend on the browser's local timezone.
  const instant = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(trimmed) ? trimmed : `${trimmed.replace(" ", "T")}Z`;
  const millisecondPrecision = instant.replace(/(\.\d{3})\d+(?=Z|[+-]\d{2}:?\d{2}$)/i, "$1");
  const parsed = new Date(millisecondPrecision);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

function beijingParts(value: string) {
  const parsed = parseInstant(value);
  if (!parsed) return null;
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: beijingTimeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(parsed);
  return Object.fromEntries(parts.map(({ type, value: part }) => [type, part]));
}

export function formatDateOnly(value?: string | null) {
  if (!value) return "-";

  const trimmed = value.trim();
  if (!trimmed) return "-";
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return trimmed;

  const parts = beijingParts(trimmed);
  return parts ? `${parts.year}-${parts.month}-${parts.day}` : trimmed.slice(0, 10) || "-";
}

export function formatDateTime(value?: string | null) {
  if (!value) return "-";

  const parts = beijingParts(value);
  return parts
    ? `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
    : value.trim().replace("T", " ").replace(/\.\d{1,9}(?=Z?$)/i, "").replace(/Z$/i, "").slice(0, 19) || "-";
}
