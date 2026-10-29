// Money is a whole number of cents (docs/05). Nothing here accepts a float.

const whole = new Intl.NumberFormat("en-LK", { maximumFractionDigits: 0 });

function assertCents(cents: number): void {
  if (!Number.isSafeInteger(cents)) throw new RangeError("money must be a whole number of cents");
}

/** formatLKR(143500) = "Rs 1,435.00". `compact` drops ".00": "Rs 1,435". */
export function formatLKR(cents: number, opts: { compact?: boolean } = {}): string {
  assertCents(cents);
  const abs = Math.abs(cents);
  const rupees = whole.format(Math.trunc(abs / 100));
  const rest = abs % 100;
  const frac = rest === 0 && opts.compact ? "" : `.${String(rest).padStart(2, "0")}`;
  return `${cents < 0 ? "-" : ""}Rs ${rupees}${frac}`;
}

/** Whole cents saved when a struck-through MRP is higher than the price, else 0. */
export function saveCents(priceCents: number, mrpCents?: number | null): number {
  assertCents(priceCents);
  if (mrpCents == null) return 0;
  assertCents(mrpCents);
  return Math.max(0, mrpCents - priceCents);
}

/**
 * Rupees typed by a person ("1435", "1,435.50") to whole cents, with no floating point: the text is
 * split at the dot. Null when it is not money (letters, a minus, three decimals, empty).
 */
export function parseRupees(text: string): number | null {
  const m = /^(\d{1,9})(?:\.(\d{1,2}))?$/.exec(text.replace(/[,\s]/g, "").replace(/^Rs\.?/i, ""));
  if (!m) return null;
  return Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0"));
}

/** 143500 -> "1435.00", the text for an input box. */
export function centsToRupeesText(cents: number): string {
  assertCents(cents);
  return `${Math.trunc(cents / 100)}.${String(Math.abs(cents) % 100).padStart(2, "0")}`;
}
