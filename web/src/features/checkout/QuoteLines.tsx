import { Banner, Card, Select } from "@/components/ui";
import { TotalsRows } from "@/features/orders/TotalsRows";
import { substitutionPrefValues, type SubstitutionPref } from "@/lib/enums.gen";
import { t, type MessageKey } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import type { Quote } from "./types";

const prefText: Record<SubstitutionPref, MessageKey> = {
  REMOVE: "checkout.prefRemove",
  SIMILAR: "checkout.prefSimilar",
  CALL_ME: "checkout.prefCall",
};

interface Props {
  quote: Quote;
  prefs: Record<string, SubstitutionPref>;
  onPref: (itemId: string, pref: SubstitutionPref) => void;
}

/** The bill: each line with "if it is not available", what cannot be sold, then the totals. */
export function QuoteLines({ quote, prefs, onPref }: Props) {
  return (
    <section className="flex flex-col gap-3" aria-labelledby="bill">
      <h2 id="bill" className="text-lg font-semibold">{t("checkout.bill")}</h2>
      {quote.excluded.length > 0 && (
        <Banner tone="warning">
          <p className="font-semibold">{t("checkout.excluded")}</p>
          <ul>{quote.excluded.map((x) => <li key={x.shopProductId}>{x.name}</li>)}</ul>
        </Banner>
      )}
      <Card className="p-4">
        <ul className="divide-y divide-line">
          {quote.lines.map((l) => (
            <li key={l.shopProductId} className="flex flex-col gap-2 py-3 first:pt-0">
              <div className="flex items-baseline justify-between gap-3">
                <span className="min-w-0">{l.name} <span className="text-muted">{t("order.qty", { qty: l.qty })}</span></span>
                <span className="shrink-0">{formatLKR(l.lineCents)}</span>
              </div>
              {l.priceChanged && <p className="text-sm text-warning">{t("checkout.priceChanged")}</p>}
              <label className="flex items-center gap-2 text-sm text-muted">
                <span className="shrink-0">{t("checkout.ifMissing")}</span>
                <Select aria-label={t("checkout.ifMissingFor", { name: l.name })} className="h-9 text-sm" value={prefs[l.shopProductId] ?? l.subPref}
                  onChange={(e) => onPref(l.shopProductId, e.target.value as SubstitutionPref)}>
                  {substitutionPrefValues.map((p) => <option key={p} value={p}>{t(prefText[p])}</option>)}
                </Select>
              </label>
            </li>
          ))}
        </ul>
        <TotalsRows totals={quote.totals} className="border-t border-line pt-3" />
        <p className="mt-2 text-sm text-muted">{t("checkout.distance", { km: quote.delivery.distanceKm.toFixed(1) })}</p>
      </Card>
    </section>
  );
}
