import { Card } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { TotalsRows } from "./TotalsRows";
import type { OrderDetail } from "./types";

const money = (cents: number | null) => (cents === null ? "" : formatLKR(cents));

/** The lines of the current bill, then the totals. */
export function OrderLines({ order }: { order: Pick<OrderDetail, "lines" | "totals"> }) {
  return (
    <Card className="p-4">
      <ul className="divide-y divide-line">
        {order.lines.filter((l) => l.state !== "REMOVED" && l.state !== "MISSING").map((l) => (
          <li key={l.id} className="flex items-baseline justify-between gap-3 py-2">
            <span className="min-w-0 truncate">{l.name} <span className="text-muted">{t("order.qty", { qty: l.qty })}</span></span>
            <span className="shrink-0">{money(l.lineCents)}</span>
          </li>
        ))}
      </ul>
      <TotalsRows totals={order.totals} className="mt-2 border-t border-line pt-3" />
    </Card>
  );
}
