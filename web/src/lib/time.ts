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
