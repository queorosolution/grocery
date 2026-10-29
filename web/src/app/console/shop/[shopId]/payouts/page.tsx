import { PayoutsScreen } from "@/features/console-shop/PayoutsScreen";

// One placeholder file; Cloudflare rewrites /console/shop/<any id>/payouts to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ shopId: "_" }];
}

export default function Page() {
  return <PayoutsScreen />;
}
