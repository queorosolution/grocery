import Link from "next/link";
import { Badge, Thumb } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatDay, formatTime } from "@/lib/time";
import type { ShopCardData } from "./types";

/** One shop on the home page: logo, name, open or closed, distance and the next delivery window. */
export function ShopTile({ shop }: { shop: ShopCardData }) {
  return (
    <Link href={`/s/${shop.slug}`}
      className="flex items-center gap-3 rounded-card border border-line bg-bg p-3 md:hover:shadow-raise">
      <Thumb src={shop.logoUrl} className="size-14 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-base font-semibold">{shop.name}</div>
        <div className="truncate text-sm text-muted">
          {[
            shop.distanceKm !== null && t("home.km", { km: shop.distanceKm.toFixed(1) }),
            shop.nextSlot && t("home.nextSlot", { time: `${formatDay(shop.nextSlot)} ${formatTime(shop.nextSlot)}` }),
          ].filter(Boolean).join(" · ")}
        </div>
      </div>
      <Badge tone={shop.openNow ? "success" : "neutral"}>{shop.openNow ? t("shop.open") : t("shop.closed")}</Badge>
    </Link>
  );
}
