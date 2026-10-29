"use client";

import { useRouteId } from "@/lib/useRouteId";
import { OrderQueue } from "./OrderQueue";
import { ShopFrame } from "./ShopFrame";

export function OrdersPage() {
  const shopId = useRouteId("/console/shop/:shopId/orders", "shopId");
  return <ShopFrame shopId={shopId}>{(id) => <OrderQueue shopId={id} />}</ShopFrame>;
}
