import { OrderScreen } from "@/features/orders/OrderScreen";

// One placeholder file. Cloudflare rewrites /order/<any id>/ to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ id: "_" }];
}

export default function Page() {
  return <OrderScreen />;
}
