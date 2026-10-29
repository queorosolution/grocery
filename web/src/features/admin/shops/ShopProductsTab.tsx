"use client";

import type { ShopDetail } from "../types";
import { MarkupsCard } from "./MarkupsCard";
import { OnSaleCard } from "./OnSaleCard";

export function ShopProductsTab({ detail }: { detail: ShopDetail }) {
  return (
    <div className="flex flex-col gap-4">
      <OnSaleCard shopId={detail.shop.id} />
      <MarkupsCard shopId={detail.shop.id} />
    </div>
  );
}
