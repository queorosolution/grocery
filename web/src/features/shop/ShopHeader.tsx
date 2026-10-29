import { Badge, Thumb } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { safeColor, textOn } from "@/lib/safeColor";
import type { ShopDetail } from "./types";

/** The shop's banner in its own colour: name, logo, open or closed, hours and the minimum order. */
export function ShopHeader({ shop }: { shop: ShopDetail }) {
  const color = safeColor(shop.themeColor);
  const open = shop.openNow && shop.acceptingOrders;
  return (
    <section
      className="bg-shop"
      style={color ? { backgroundColor: color, color: textOn(color) } : undefined}
    >
      <div className="mx-auto flex max-w-[1120px] items-center gap-3 px-4 py-4">
        <Thumb src={shop.logoUrl} className="size-14 shrink-0 bg-white" />
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold">{shop.name}</h1>
          <p className="text-sm opacity-90">
            {shop.hoursToday ? t("shop.hours", shop.hoursToday) : t("shop.closedToday")}
            {shop.minOrderCents > 0 && ` · ${t("shop.minOrder", { amount: formatLKR(shop.minOrderCents, { compact: true }) })}`}
          </p>
        </div>
        <Badge tone={open ? "success" : "neutral"} className="ml-auto shrink-0">{open ? t("shop.open") : t("shop.closed")}</Badge>
      </div>
    </section>
  );
}
