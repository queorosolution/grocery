import { Banner } from "@/components/ui";
import { t } from "@/lib/i18n";
import type { Reachable } from "./types";

/** After a pin is saved: which shops deliver there (FR-ADDR-02). No shop is a notice, not an error. */
export function DeliveryNote({ shops }: { shops: readonly Reachable[] }) {
  if (shops.length === 0) return <Banner tone="warning">{t("address.noShops")}</Banner>;
  return (
    <Banner tone="success">
      <p className="font-semibold">{shops.length === 1 ? t("address.shopOne") : t("address.shops", { count: shops.length })}</p>
      <ul className="mt-1">
        {shops.slice(0, 3).map((s) => (
          <li key={s.slug}>{t("address.shopLine", { name: s.name, km: s.distanceKm.toFixed(1) })}</li>
        ))}
      </ul>
    </Banner>
  );
}
