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
