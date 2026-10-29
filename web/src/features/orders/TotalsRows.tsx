import clsx from "clsx";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import type { Totals } from "./types";

/** Items, delivery, fees, discount and the total: the same on the bill page and the order page. */
export function TotalsRows({ totals, className }: { totals: Totals; className?: string }) {
  const rows: [string, number, boolean][] = [
    [t("order.items"), totals.itemsCents, true],
    [t("order.fee"), totals.deliveryFeeCents, true],
    [t("order.service"), totals.serviceFeeCents, totals.serviceFeeCents > 0],
    [t("order.discount"), -totals.discountCents, totals.discountCents > 0],
  ];
  return (
    <dl className={clsx("flex flex-col gap-1 text-sm", className)}>
      {rows.filter(([, , show]) => show).map(([label, cents]) => (
        <div key={label} className="flex justify-between"><dt className="text-muted">{label}</dt><dd>{formatLKR(cents)}</dd></div>
      ))}
      <div className="flex justify-between text-base font-semibold"><dt>{t("order.total")}</dt><dd>{formatLKR(totals.totalCents)}</dd></div>
    </dl>
  );
}
