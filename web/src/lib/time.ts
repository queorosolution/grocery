// Dates are shown in Sri Lanka time with Intl, whatever the device zone is (docs/08 section 10).

const LOCALE = "en-LK";
const ZONE = "Asia/Colombo";

const day = new Intl.DateTimeFormat(LOCALE, { timeZone: ZONE, weekday: "short", day: "numeric", month: "short" });
const clock = new Intl.DateTimeFormat(LOCALE, { timeZone: ZONE, hour: "numeric", minute: "2-digit" });
const full = new Intl.DateTimeFormat(LOCALE, {
  timeZone: ZONE, day: "numeric", month: "short", year: "numeric", hour: "numeric", minute: "2-digit",
});

function parse(iso: string): Date {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) throw new RangeError("invalid date");
  return d;
}

export const formatDay = (iso: string) => day.format(parse(iso));
export const formatTime = (iso: string) => clock.format(parse(iso));
export const formatDateTime = (iso: string) => full.format(parse(iso));

/** "Mon, 2 Nov · 8:30 am – 10:00 am" for a delivery slot. */
export function formatSlot(startIso: string, endIso: string): string {
  return `${formatDay(startIso)} · ${formatTime(startIso)} – ${formatTime(endIso)}`;
}

const colomboDate = new Intl.DateTimeFormat("en-CA", { timeZone: ZONE, year: "numeric", month: "2-digit", day: "2-digit" });

/** A Colombo calendar day as YYYY-MM-DD: `colomboDay(0)` today, `colomboDay(-6)` six days ago. */
export function colomboDay(offsetDays: number, now: Date = new Date()): string {
  return colomboDate.format(new Date(now.getTime() + offsetDays * 86_400_000));
}

/** "2026-11-02" + "08:30" as the moment in Colombo (UTC+5:30, no daylight saving), in ISO form for the API. */
export const colomboIso = (date: string, time: string): string => `${date}T${time}:00+05:30`;

/** A day shifted by whole days, both as Colombo dates: addDays("2026-11-02", 7) = "2026-11-09". */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

const COLOMBO_OFFSET_MS = 5.5 * 3_600_000;

/** An instant as the text of a datetime-local box in Colombo time: "2026-11-02T08:30". */
export const toColomboLocal = (iso: string): string => new Date(new Date(iso).getTime() + COLOMBO_OFFSET_MS).toISOString().slice(0, 16);

/** The text of a datetime-local box read as Colombo time, in ISO form for the API. Null when it is empty or not a date. */
export function fromColomboLocal(local: string): string | null {
  const m = /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})$/.exec(local);
  if (!m) return null;
  const d = new Date(colomboIso(m[1]!, m[2]!));
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}
