import { OrdersPage } from "@/features/console-shop/OrdersPage";

// One placeholder file; Cloudflare rewrites /console/shop/<any id>/orders to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ shopId: "_" }];
}

export default function Page() {
  return <OrdersPage />;
}
